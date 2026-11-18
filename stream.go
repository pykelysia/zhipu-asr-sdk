package asr

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// 流式事件类型（payload 内的 type 字段，非 SSE event: 命名行）。
const (
	// EventTranscriptTextDelta 携带增量转录文本片段（delta 字段）。
	EventTranscriptTextDelta = "transcript.text.delta"
	// EventTranscriptTextDone 携带完整转录结果（text 字段），流结束前最后一条业务事件。
	EventTranscriptTextDone = "transcript.text.done"
)

// sseDoneLine 是流式响应的终止标记行。
const sseDoneLine = "[DONE]"

// TranscriptionEvent 是流式接口的单条 SSE 事件。
type TranscriptionEvent struct {
	ID        string `json:"id"`
	Created   int64  `json:"created"`
	RequestID string `json:"request_id"`
	Model     string `json:"model"`
	Type      string `json:"type"`
	Delta     string `json:"delta"` // 仅 transcript.text.delta 携带
	Text      string `json:"text"`  // 仅 transcript.text.done 携带
}

// TranscriptionStream 是流式转写的迭代器，用法类似 database/sql.Rows：
//
//	stream, err := client.TranscribeStream(ctx, req)
//	if err != nil { ... }
//	defer stream.Close()
//	for stream.Next() {
//	    ev := stream.Event()
//	    ...
//	}
//	if err := stream.Err(); err != nil { ... }
type TranscriptionStream struct {
	resp       *http.Response
	scanner    *bufio.Scanner
	cur        TranscriptionEvent
	err        error
	doneMarked bool // 已收到 data: [DONE]
	closed     bool // Close 已调用
}

// TranscribeStream 以流式方式调用转写接口（自动携带 stream=true），
// 通过 Event Stream 逐块返回事件，流末尾由 data: [DONE] 终止。
// 连接关闭即流结束；建议通过 ctx 控制整体读超时。
func (c *Client) TranscribeStream(ctx context.Context, req TranscriptionRequest) (*TranscriptionStream, error) {
	req.Stream = true

	resp, err := c.execute(ctx, req)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(resp.Body)
	// 单行上限 1MB：delta 片段很短，done 事件携带完整文本时可较长。
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	return &TranscriptionStream{resp: resp, scanner: scanner}, nil
}

// Next 推进到下一条业务事件；流结束、出错或收到 [DONE] 后返回 false。
// 结束后通过 Err 区分正常结束（nil）与失败。
func (s *TranscriptionStream) Next() bool {
	if s.err != nil || s.doneMarked || s.closed {
		return false
	}
	for s.scanner.Scan() {
		line := s.scanner.Text()
		// SSE 规范：空行为事件分隔符，冒号开头为注释行（可用作心跳），均跳过。
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == sseDoneLine {
			s.doneMarked = true
			return false
		}

		var ev TranscriptionEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			s.err = fmt.Errorf("asr: 解析 SSE 事件失败: %w", err)
			return false
		}
		if ev.Type == "" {
			s.err = fmt.Errorf("asr: SSE 事件缺少 type 字段: %s", data)
			return false
		}
		s.cur = ev
		return true
	}
	if err := s.scanner.Err(); err != nil {
		s.err = fmt.Errorf("asr: 读取 SSE 流失败: %w", err)
	}
	return false
}

// Event 返回最近一次 Next 成功时的事件；Next 返回 false 前不保证有效。
func (s *TranscriptionStream) Event() *TranscriptionEvent {
	return &s.cur
}

// Err 返回迭代过程中的错误；流正常结束时为 nil。
func (s *TranscriptionStream) Err() error {
	return s.err
}

// Close 关闭底层连接，必须调用（推荐 defer）。
func (s *TranscriptionStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.resp.Body.Close()
}

// FullText 消费剩余事件并返回完整转录文本：
// 收到 transcript.text.done 时取其 text（权威结果）；
// 若流异常缺失 done 事件，则退化为拼接全部 delta。
func (s *TranscriptionStream) FullText() (string, error) {
	var deltas strings.Builder
	for s.Next() {
		ev := s.Event()
		switch ev.Type {
		case EventTranscriptTextDone:
			return ev.Text, nil
		case EventTranscriptTextDelta:
			deltas.WriteString(ev.Delta)
		}
	}
	return deltas.String(), s.Err()
}

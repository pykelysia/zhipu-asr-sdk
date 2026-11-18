package asr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// ModelGLMASR2512 是当前唯一支持的转写模型编码。
const ModelGLMASR2512 = "glm-asr-2512"

// TranscriptionRequest 描述一次语音转写请求。
//
// 音频来源三选一，优先级 File > FilePath > FileBase64
// （与官方"file 与 file_base64 同时传时以 file 为准"一致）：
//   - File：内存中的音频内容，需为 .wav/.mp3，≤25MB、时长 ≤30 秒
//   - FilePath：本地音频文件路径（FileName 自动取路径基名）
//   - FileBase64：音频 Base64 编码字符串（解码后同样受大小/时长约束）
type TranscriptionRequest struct {
	File       io.Reader
	FileName   string
	FilePath   string
	FileBase64 string

	Model    string   // 必填，枚举值 glm-asr-2512
	Prompt   string   // 上下文提示，长音频分段场景传入之前的转录结果，建议 <8000 字
	Hotwords []string // 热词表，最多 100 个，multipart 中以重复字段发送
	Stream   bool     // 同步接口忽略此字段；TranscribeStream 会强制置 true
	// RequestID 请求唯一标识，6–64 字符；未传由服务端自动生成。
	RequestID string
	// UserID 终端用户唯一 ID，6–128 字符。
	UserID string
}

// TranscriptionResponse 是同步接口（stream=false）的结构化返回。
type TranscriptionResponse struct {
	ID        string `json:"id"`
	Created   int64  `json:"created"`
	RequestID string `json:"request_id"`
	Model     string `json:"model"`
	Text      string `json:"text"`
}

// Transcribe 以同步方式调用转写接口，返回完整转录文本。
// 服务端协议不含 usage/token 字段，本 SDK 同样不提供用量信息。
func (c *Client) Transcribe(ctx context.Context, req TranscriptionRequest) (*TranscriptionResponse, error) {
	req.Stream = false // 防御：同步接口不允许携带 stream=true
	body, err := req.buildMultipartBody()
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(body)
	if err != nil {
		return nil, err
	}
	httpReq = httpReq.WithContext(ctx)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("asr: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, parseAPIError(resp)
	}

	var out TranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("asr: 解析响应失败: %w", err)
	}
	return &out, nil
}

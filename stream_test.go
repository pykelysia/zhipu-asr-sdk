package asr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// startSSEServer 启动返回给定 SSE 报文的测试服务（text/event-stream），并捕获请求表单。
func startSSEServer(t *testing.T, status int, sseBody string) (*capturedForm, *httptest.Server) {
	t.Helper()
	return captureMultipart(t, status, sseBody, map[string]string{"Content-Type": "text/event-stream"})
}

func TestTranscribeStream_Events(t *testing.T) {
	body := "data: {\"id\":\"asr_8f3c\",\"created\":1760000000,\"request_id\":\"req-42\",\"model\":\"glm-asr-2512\",\"type\":\"transcript.text.delta\",\"delta\":\"会议\"}\n\n" +
		"data: {\"id\":\"asr_8f3c\",\"created\":1760000000,\"request_id\":\"req-42\",\"model\":\"glm-asr-2512\",\"type\":\"transcript.text.delta\",\"delta\":\"记录\"}\n\n" +
		"data: {\"id\":\"asr_8f3c\",\"created\":1760000000,\"request_id\":\"req-42\",\"model\":\"glm-asr-2512\",\"type\":\"transcript.text.done\",\"text\":\"会议记录\"}\n\n" +
		"data: [DONE]\n\n"
	form, srv := startSSEServer(t, http.StatusOK, body)

	client := NewClient("k", WithBaseURL(srv.URL))
	stream, err := client.TranscribeStream(context.Background(), TranscriptionRequest{
		File:      strings.NewReader("wav"),
		FileName:  "a.wav",
		Model:     ModelGLMASR2512,
		RequestID: "req-42",
	})
	if err != nil {
		t.Fatalf("TranscribeStream 返回错误: %v", err)
	}
	defer stream.Close()

	if got := form.values["stream"]; len(got) != 1 || got[0] != "true" {
		t.Errorf("流式接口应发送 stream=true: %v", got)
	}

	var types []string
	var deltas []string
	for stream.Next() {
		ev := stream.Event()
		types = append(types, ev.Type)
		if ev.Type == EventTranscriptTextDelta {
			if ev.ID != "asr_8f3c" || ev.RequestID != "req-42" || ev.Model != ModelGLMASR2512 || ev.Created != 1760000000 {
				t.Errorf("事件公共字段解析错误: %+v", ev)
			}
			deltas = append(deltas, ev.Delta)
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("流应正常结束: %v", err)
	}
	wantTypes := []string{EventTranscriptTextDelta, EventTranscriptTextDelta, EventTranscriptTextDone}
	if len(types) != 3 || types[0] != wantTypes[0] || types[1] != wantTypes[1] || types[2] != wantTypes[2] {
		t.Errorf("事件序列错误: %v", types)
	}
	if strings.Join(deltas, "") != "会议记录" {
		t.Errorf("delta 拼接错误: %v", deltas)
	}
	done := stream.Event()
	if done.Text != "会议记录" {
		t.Errorf("done 事件 text 错误: %q", done.Text)
	}
	if stream.Next() {
		t.Error("[DONE] 之后 Next 应返回 false")
	}
}

func TestTranscribeStream_FullText(t *testing.T) {
	body := "data: {\"type\":\"transcript.text.delta\",\"delta\":\"你好\"}\n\n" +
		"data: {\"type\":\"transcript.text.delta\",\"delta\":\"世界\"}\n\n" +
		"data: {\"type\":\"transcript.text.done\",\"text\":\"你好世界\"}\n\n" +
		"data: [DONE]\n\n"
	_, srv := startSSEServer(t, http.StatusOK, body)

	client := NewClient("k", WithBaseURL(srv.URL))
	stream, err := client.TranscribeStream(context.Background(), TranscriptionRequest{
		FileBase64: "aGk=",
		Model:      ModelGLMASR2512,
	})
	if err != nil {
		t.Fatalf("TranscribeStream 返回错误: %v", err)
	}
	defer stream.Close()

	text, err := stream.FullText()
	if err != nil {
		t.Fatalf("FullText 返回错误: %v", err)
	}
	if text != "你好世界" {
		t.Errorf("FullText 应取 done 事件的 text: %q", text)
	}
}

// TestTranscribeStream_FullTextFallbackDeltas 覆盖流缺失 done 事件的场景：
// 连接关闭后 FullText 退化为拼接 delta。
func TestTranscribeStream_FullTextFallbackDeltas(t *testing.T) {
	body := "data: {\"type\":\"transcript.text.delta\",\"delta\":\"片段一\"}\n\n" +
		": keepalive comment\n\n" +
		"\n" +
		"data: {\"type\":\"transcript.text.delta\",\"delta\":\"片段二\"}\n\n"
	_, srv := startSSEServer(t, http.StatusOK, body)

	client := NewClient("k", WithBaseURL(srv.URL))
	stream, err := client.TranscribeStream(context.Background(), TranscriptionRequest{
		FileBase64: "aGk=",
		Model:      ModelGLMASR2512,
	})
	if err != nil {
		t.Fatalf("TranscribeStream 返回错误: %v", err)
	}
	defer stream.Close()

	text, err := stream.FullText()
	if err != nil {
		t.Fatalf("FullText 返回错误: %v", err)
	}
	if text != "片段一片段二" {
		t.Errorf("无 done 事件时应拼接 delta: %q", text)
	}
}

func TestTranscribeStream_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"1211","message":"模型不存在"}}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient("k", WithBaseURL(srv.URL))
	stream, err := client.TranscribeStream(context.Background(), TranscriptionRequest{
		FileBase64: "aGk=",
		Model:      "glm-not-exist",
	})
	if stream != nil {
		t.Fatal("出错时不应返回流对象")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("错误类型应为 *APIError，实际 %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 || apiErr.Code != "1211" || apiErr.Message != "模型不存在" {
		t.Errorf("APIError 字段错误: %+v", apiErr)
	}
}

func TestTranscribeStream_InvalidJSON(t *testing.T) {
	_, srv := startSSEServer(t, http.StatusOK, "data: {not-json\n\n")
	client := NewClient("k", WithBaseURL(srv.URL))
	stream, err := client.TranscribeStream(context.Background(), TranscriptionRequest{
		FileBase64: "aGk=",
		Model:      ModelGLMASR2512,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	if stream.Next() {
		t.Fatal("非法 JSON 不应产出事件")
	}
	if stream.Err() == nil || !strings.Contains(stream.Err().Error(), "解析 SSE 事件失败") {
		t.Errorf("应返回解析错误: %v", stream.Err())
	}
}

// TestTranscribeStream_MissingType 覆盖 payload 缺失 type 字段的畸形事件。
func TestTranscribeStream_MissingType(t *testing.T) {
	_, srv := startSSEServer(t, http.StatusOK, "data: {\"id\":\"x\"}\n\n")
	client := NewClient("k", WithBaseURL(srv.URL))
	stream, _ := client.TranscribeStream(context.Background(), TranscriptionRequest{
		FileBase64: "aGk=",
		Model:      ModelGLMASR2512,
	})
	defer stream.Close()

	if stream.Next() {
		t.Fatal("缺失 type 的事件不应产出")
	}
	if stream.Err() == nil || !strings.Contains(stream.Err().Error(), "type") {
		t.Errorf("应报缺失 type 错误: %v", stream.Err())
	}
}

// TestTranscribeStream_DataPrefixWithoutSpace 覆盖 data:后无空格的合法写法。
func TestTranscribeStream_DataPrefixWithoutSpace(t *testing.T) {
	_, srv := startSSEServer(t, http.StatusOK, "data:{\"type\":\"transcript.text.delta\",\"delta\":\"ok\"}\n\ndata: [DONE]\n\n")
	client := NewClient("k", WithBaseURL(srv.URL))
	stream, _ := client.TranscribeStream(context.Background(), TranscriptionRequest{
		FileBase64: "aGk=",
		Model:      ModelGLMASR2512,
	})
	defer stream.Close()

	if !stream.Next() || stream.Event().Delta != "ok" {
		t.Fatalf("data:后无空格应能解析: %+v err=%v", stream.Event(), stream.Err())
	}
	if stream.Next() {
		t.Error("[DONE] 后应终止")
	}
	if stream.Err() != nil {
		t.Errorf("正常终止不应有错误: %v", stream.Err())
	}
}

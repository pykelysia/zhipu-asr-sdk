package asr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capturedForm 记录服务端收到的 multipart 表单内容，便于断言。
type capturedForm struct {
	method string
	path   string
	auth   string
	// file 为文件 part 的 (filename, content)，未发送时 ok=false
	file     string
	fileBody string
	fileOK   bool
	values   map[string][]string
}

// captureMultipart 启动一个解析 multipart 请求的测试服务，返回表单捕获与服务器。
func captureMultipart(t *testing.T, status int, respBody string, respHeader map[string]string) (*capturedForm, *httptest.Server) {
	t.Helper()
	form := &capturedForm{values: map[string][]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form.method = r.Method
		form.path = r.URL.Path
		form.auth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(maxAudioSize); err != nil {
			t.Errorf("解析 multipart 失败: %v", err)
		}
		for k, v := range r.MultipartForm.Value {
			form.values[k] = v
		}
		if r.MultipartForm.File != nil {
			if headers, ok := r.MultipartForm.File["file"]; ok && len(headers) > 0 {
				f, err := headers[0].Open()
				if err == nil {
					content, _ := io.ReadAll(f)
					f.Close()
					form.file, form.fileBody, form.fileOK = headers[0].Filename, string(content), true
				}
			}
		}
		for k, v := range respHeader {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return form, srv
}

func TestTranscribe_Sync(t *testing.T) {
	form, srv := captureMultipart(t, http.StatusOK,
		`{"id":"asr_8f3c","created":1760000000,"request_id":"req-42","model":"glm-asr-2512","text":"会议记录"}`,
		nil)

	client := NewClient("test-key", WithBaseURL(srv.URL))
	resp, err := client.Transcribe(context.Background(), TranscriptionRequest{
		File:      strings.NewReader("fake-wav-bytes"),
		FileName:  "audio.wav",
		Model:     ModelGLMASR2512,
		Prompt:    "会议记录",
		Hotwords:  []string{"张三", "李四"},
		RequestID: "req-42",
		UserID:    "user-0001",
		Stream:    true, // 同步接口应强制忽略
	})
	if err != nil {
		t.Fatalf("Transcribe 返回错误: %v", err)
	}

	// 请求侧断言
	if form.method != http.MethodPost || form.path != "/api/paas/v4/audio/transcriptions" {
		t.Errorf("请求行错误: %s %s", form.method, form.path)
	}
	if form.auth != "Bearer test-key" {
		t.Errorf("Authorization 错误: %q", form.auth)
	}
	if !form.fileOK || form.file != "audio.wav" || form.fileBody != "fake-wav-bytes" {
		t.Errorf("file part 错误: ok=%v name=%q body=%q", form.fileOK, form.file, form.fileBody)
	}
	if got := form.values["model"]; len(got) != 1 || got[0] != ModelGLMASR2512 {
		t.Errorf("model 字段错误: %v", got)
	}
	if got := form.values["prompt"]; len(got) != 1 || got[0] != "会议记录" {
		t.Errorf("prompt 字段错误: %v", got)
	}
	if got := form.values["hotwords"]; len(got) != 2 || got[0] != "张三" || got[1] != "李四" {
		t.Errorf("hotwords 应为两个重复字段: %v", got)
	}
	if got := form.values["request_id"]; len(got) != 1 || got[0] != "req-42" {
		t.Errorf("request_id 字段错误: %v", got)
	}
	if got := form.values["user_id"]; len(got) != 1 || got[0] != "user-0001" {
		t.Errorf("user_id 字段错误: %v", got)
	}
	if _, has := form.values["stream"]; has {
		t.Errorf("同步接口不应发送 stream 字段")
	}
	if _, has := form.values["file_base64"]; has {
		t.Errorf("已发送 file 时不应再发送 file_base64")
	}

	// 响应侧断言
	if resp.ID != "asr_8f3c" || resp.Created != 1760000000 ||
		resp.RequestID != "req-42" || resp.Model != ModelGLMASR2512 || resp.Text != "会议记录" {
		t.Errorf("响应解析错误: %+v", resp)
	}
}

func TestTranscribe_FilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audio.mp3")
	if err := os.WriteFile(path, []byte("fake-mp3-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	form, srv := captureMultipart(t, http.StatusOK, `{"id":"1","text":"hi"}`, nil)
	client := NewClient("k", WithBaseURL(srv.URL))

	resp, err := client.Transcribe(context.Background(), TranscriptionRequest{
		Model:    ModelGLMASR2512,
		FilePath: path,
	})
	if err != nil {
		t.Fatalf("Transcribe 返回错误: %v", err)
	}
	if !form.fileOK || form.file != "audio.mp3" || form.fileBody != "fake-mp3-bytes" {
		t.Errorf("FilePath 音频 part 错误: ok=%v name=%q body=%q", form.fileOK, form.file, form.fileBody)
	}
	if resp.Text != "hi" {
		t.Errorf("text 解析错误: %q", resp.Text)
	}
}

func TestTranscribe_FileBase64(t *testing.T) {
	form, srv := captureMultipart(t, http.StatusOK, `{"text":"ok"}`, nil)
	client := NewClient("k", WithBaseURL(srv.URL))

	if _, err := client.Transcribe(context.Background(), TranscriptionRequest{
		Model:      ModelGLMASR2512,
		FileBase64: "aGVsbG8=",
	}); err != nil {
		t.Fatalf("Transcribe 返回错误: %v", err)
	}
	if form.fileOK {
		t.Errorf("仅传 Base64 时不应发送 file part")
	}
	if got := form.values["file_base64"]; len(got) != 1 || got[0] != "aGVsbG8=" {
		t.Errorf("file_base64 字段错误: %v", got)
	}
}

func TestTranscribe_APIError(t *testing.T) {
	_, srv := captureMultipart(t, http.StatusUnauthorized,
		`{"error":{"code":"401","message":"无效的 API Key","type":"invalid_request_error"}}`,
		map[string]string{"X-Request-Id": "req-401"})

	client := NewClient("bad-key", WithBaseURL(srv.URL))
	_, err := client.Transcribe(context.Background(), TranscriptionRequest{
		File:     strings.NewReader("x"),
		FileName: "a.wav",
		Model:    ModelGLMASR2512,
	})
	if err == nil {
		t.Fatal("期望返回错误")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("错误类型应为 *APIError，实际 %T: %v", err, err)
	}
	if apiErr.StatusCode != 401 || apiErr.Code != "401" ||
		apiErr.Message != "无效的 API Key" || apiErr.Type != "invalid_request_error" {
		t.Errorf("APIError 字段错误: %+v", apiErr)
	}
	if apiErr.RequestID != "req-401" {
		t.Errorf("应从 X-Request-Id 头提取 request_id: %q", apiErr.RequestID)
	}
	if !strings.Contains(apiErr.Error(), "401") {
		t.Errorf("Error() 应包含状态码: %s", apiErr.Error())
	}
}

func TestTranscribe_UnparsableErrorBody(t *testing.T) {
	_, srv := captureMultipart(t, http.StatusBadGateway, "upstream unavailable", nil)
	client := NewClient("k", WithBaseURL(srv.URL))

	_, err := client.Transcribe(context.Background(), TranscriptionRequest{
		FileBase64: "aGk=",
		Model:      ModelGLMASR2512,
	})
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("错误类型应为 *APIError，实际 %T: %v", err, err)
	}
	if apiErr.StatusCode != 502 || apiErr.Message != "upstream unavailable" {
		t.Errorf("非 JSON 错误体应退化为原文: %+v", apiErr)
	}
}

func TestTranscribe_Validation(t *testing.T) {
	client := NewClient("k")

	if _, err := client.Transcribe(context.Background(), TranscriptionRequest{FileBase64: "aGk="}); err == nil {
		t.Error("缺少 model 应报错")
	}
	if _, err := client.Transcribe(context.Background(), TranscriptionRequest{Model: ModelGLMASR2512}); err == nil {
		t.Error("缺少音频来源应报错")
	}
	if _, err := client.Transcribe(context.Background(), TranscriptionRequest{
		Model:    ModelGLMASR2512,
		FilePath: filepath.Join(t.TempDir(), "missing.wav"),
	}); err == nil {
		t.Error("文件不存在应报错")
	}
}

func TestTranscribe_AudioTooLarge(t *testing.T) {
	big := bytes.Repeat([]byte{0}, maxAudioSize+1)
	_, err := NewClient("k").Transcribe(context.Background(), TranscriptionRequest{
		File:     bytes.NewReader(big),
		FileName: "big.wav",
		Model:    ModelGLMASR2512,
	})
	if err == nil || !strings.Contains(err.Error(), "25MB") {
		t.Errorf("超过 25MB 应报错，实际: %v", err)
	}
}

// TestTranscribe_Base64TooLarge 覆盖 Base64 解码后超过 25MB 的预检。
func TestTranscribe_Base64TooLarge(t *testing.T) {
	big := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0}, maxAudioSize+1024))
	_, err := NewClient("k").Transcribe(context.Background(), TranscriptionRequest{
		FileBase64: big,
		Model:      ModelGLMASR2512,
	})
	if err == nil || !strings.Contains(err.Error(), "25MB") {
		t.Errorf("Base64 解码后超限应报错，实际: %v", err)
	}
}

// TestTranscribe_Base64WithinLimitNotRejected 确保限内 Base64 不被预检误拒
// （预检按解码上界减去 padding 计算，限内的请求应照常发出）。
func TestTranscribe_Base64WithinLimitNotRejected(t *testing.T) {
	ok := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0}, 1024))
	_, srv := captureMultipart(t, http.StatusOK, `{"text":"ok"}`, nil)
	if _, err := NewClient("k", WithBaseURL(srv.URL)).Transcribe(context.Background(), TranscriptionRequest{
		FileBase64: ok,
		Model:      ModelGLMASR2512,
	}); err != nil {
		t.Errorf("限内 Base64 不应被预检拒绝: %v", err)
	}
}

// TestBuildMultipartBody_FilePreferred 校验音频来源优先级：File > FileBase64。
func TestBuildMultipartBody_FilePreferred(t *testing.T) {
	body, err := (&TranscriptionRequest{
		File:       strings.NewReader("raw"),
		FileName:   "a.wav",
		FileBase64: "cmF3",
		Model:      ModelGLMASR2512,
	}).buildMultipartBody()
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(body.contentType)
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(bytes.NewReader(body.buf.Bytes()), params["boundary"])
	form, err := reader.ReadForm(maxAudioSize)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := form.Value["file_base64"]; has {
		t.Error("File 与 FileBase64 同时设置时应只发送 file")
	}
	if len(form.File["file"]) != 1 {
		t.Fatalf("应发送一个 file part")
	}
}

// TestTranscribe_JSONRoundTrip 校验响应结构体 JSON 标签与协议一致。
func TestTranscribe_JSONRoundTrip(t *testing.T) {
	raw := `{"id":"asr_8f3c","created":1760000000,"request_id":"req-42","model":"glm-asr-2512","text":"转录"}`
	var resp TranscriptionResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(resp)
	want := `{"id":"asr_8f3c","created":1760000000,"request_id":"req-42","model":"glm-asr-2512","text":"转录"}`
	if string(out) != want {
		t.Errorf("JSON 序列化与协议不符:\n got %s\nwant %s", out, want)
	}
}

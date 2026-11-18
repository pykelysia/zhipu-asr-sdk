package asr

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

// maxAudioSize 是音频大小上限（25MB），与官方约束一致。
const maxAudioSize = 25 << 20

// multipartBody 封装已序列化完成的 multipart 请求体。
type multipartBody struct {
	buf         *bytes.Buffer
	contentType string
}

// buildMultipartBody 按协议将请求参数序列化为 multipart/form-data。
//
// 音频来源优先级：File > FilePath > FileBase64，
// 与官方“同时传 file 与 file_base64 时以 file 为准”的语义一致；
// hotwords 以重复的同名字段发送（不序列化成 JSON 数组字符串）。
func (r *TranscriptionRequest) buildMultipartBody() (*multipartBody, error) {
	if r.Model == "" {
		return nil, errors.New("asr: model 不能为空（当前枚举值: glm-asr-2512）")
	}

	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)

	audioSet := false
	switch {
	case r.File != nil:
		name := r.FileName
		if name == "" {
			name = "audio.wav"
		}
		n, err := writeFormFile(w, "file", name, r.File)
		if err != nil {
			return nil, fmt.Errorf("asr: 写入音频文件失败: %w", err)
		}
		if n > maxAudioSize {
			return nil, fmt.Errorf("asr: 音频大小 %d 字节超过上限 25MB", n)
		}
		audioSet = true
	case r.FilePath != "":
		f, err := os.Open(r.FilePath)
		if err != nil {
			return nil, fmt.Errorf("asr: 打开音频文件失败: %w", err)
		}
		defer f.Close()
		n, err := writeFormFile(w, "file", filepath.Base(r.FilePath), f)
		if err != nil {
			return nil, fmt.Errorf("asr: 写入音频文件失败: %w", err)
		}
		if n > maxAudioSize {
			return nil, fmt.Errorf("asr: 音频大小 %d 字节超过上限 25MB", n)
		}
		audioSet = true
	case r.FileBase64 != "":
		if err := w.WriteField("file_base64", r.FileBase64); err != nil {
			return nil, err
		}
		audioSet = true
	}
	if !audioSet {
		return nil, errors.New("asr: 必须提供音频：File（内存内容）、FilePath（本地路径）或 FileBase64 三选一")
	}

	if err := w.WriteField("model", r.Model); err != nil {
		return nil, err
	}
	if r.Prompt != "" {
		if err := w.WriteField("prompt", r.Prompt); err != nil {
			return nil, err
		}
	}
	for _, hotword := range r.Hotwords {
		if err := w.WriteField("hotwords", hotword); err != nil {
			return nil, err
		}
	}
	if r.RequestID != "" {
		if err := w.WriteField("request_id", r.RequestID); err != nil {
			return nil, err
		}
	}
	if r.UserID != "" {
		if err := w.WriteField("user_id", r.UserID); err != nil {
			return nil, err
		}
	}
	if r.Stream {
		if err := w.WriteField("stream", "true"); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return &multipartBody{buf: buf, contentType: w.FormDataContentType()}, nil
}

// writeFormFile 写入一个 binary part 并返回已写入的字节数。
func writeFormFile(w *multipart.Writer, field, filename string, src io.Reader) (int64, error) {
	dst, err := w.CreateFormFile(field, filename)
	if err != nil {
		return 0, err
	}
	return io.Copy(dst, src)
}

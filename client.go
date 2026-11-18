// Package asr 提供智谱语音转写（ASR）官方协议的轻量 Golang SDK。
//
// 协议细节见 https://open.bigmodel.cn 的语音转文本接口文档；
// 请求以 multipart/form-data 发送，支持同步 JSON 与流式 SSE 两种响应。
package asr

import (
	"net/http"
	"strings"
)

const (
	// DefaultBaseURL 是智谱官方开放平台的根地址。
	DefaultBaseURL = "https://open.bigmodel.cn"

	// transcriptionPath 是语音转写接口路径。
	transcriptionPath = "/api/paas/v4/audio/transcriptions"
)

// Client 是智谱 ASR 的 HTTP 客户端，可安全并发复用。
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// Option 用于配置 Client。
type Option func(*Client)

// WithBaseURL 覆盖默认的官方端点根地址，一般用于测试或代理网关。
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithHTTPClient 注入自定义 *http.Client（超时、代理、传输层等）。
// 流式请求建议不设整体超时，改用 context 控制生命周期。
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// NewClient 创建客户端，apiKey 为智谱 API Key（Bearer 认证）。
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    DefaultBaseURL,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// newRequest 构造带认证头的 multipart 请求。
func (c *Client) newRequest(body *multipartBody) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+transcriptionPath, body.buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", body.contentType)
	return req, nil
}

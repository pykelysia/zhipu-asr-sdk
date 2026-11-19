// Package asr 提供智谱语音转写（ASR）官方协议的轻量 Golang SDK。
//
// 协议细节见 https://open.bigmodel.cn 的语音转文本接口文档；
// 请求以 multipart/form-data 发送，支持同步 JSON 与流式 SSE 两种响应。
package asr

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

const (
	// DefaultBaseURL 是智谱官方开放平台的 API 根地址（含 /api/paas/v4 版本前缀）。
	// 自定义网关时 WithBaseURL 填到同一层级，固定路径仅有 /audio/transcriptions。
	DefaultBaseURL = "https://open.bigmodel.cn/api/paas/v4"

	// transcriptionPath 是语音转写接口的固定路径。
	transcriptionPath = "/audio/transcriptions"
)

// Client 是智谱 ASR 的 HTTP 客户端，可安全并发复用。
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// Option 用于配置 Client。
type Option func(*Client)

// WithBaseURL 覆盖默认的官方 API 根地址（一般用于测试或代理网关）。
// 填到与 DefaultBaseURL 相同的层级（即含版本前缀的根地址），
// SDK 会在其后固定拼接 /audio/transcriptions，例如：
//
//	asr.WithBaseURL("https://your-gateway.example.com/api/paas/v4")
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

// execute 发送转写请求并校验状态码，返回待消费的响应；
// 调用方负责关闭 resp.Body。
func (c *Client) execute(ctx context.Context, req TranscriptionRequest) (*http.Response, error) {
	body, err := req.buildMultipartBody()
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(httpReq.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("asr: 请求失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		return nil, parseAPIError(resp)
	}
	return resp, nil
}

package asr

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// APIError 表示服务端返回的非 2xx 错误。
type APIError struct {
	// StatusCode 为 HTTP 状态码，如 401（无效 Key）、404（模型不存在）、
	// 403（白名单外模型）、429（额度用尽）、502（上游不可达/上游 401）。
	StatusCode int    `json:"-"`
	Code       string `json:"code"`    // 服务端错误码
	Message    string `json:"message"` // 错误描述
	Type       string `json:"type"`    // 错误类型（OpenAI 风格网关可能返回）
	// RequestID 为请求唯一标识（若响应携带），便于排查回溯。
	RequestID string `json:"-"`
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "asr: api error %d", e.StatusCode)
	if e.Code != "" {
		fmt.Fprintf(&b, " (code: %s)", e.Code)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " [request_id: %s]", e.RequestID)
	}
	return b.String()
}

// errorPayload 与 errorField 对应网关侧 OpenAI 风格错误结构
// {"error": {"message", "type", "code"}}。
type errorPayload struct {
	Error errorField `json:"error"`
}

type errorField struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// parseAPIError 将非 2xx 响应体解析为 *APIError，解析失败时退化为携带原文的通用错误。
func parseAPIError(resp *http.Response) error {
	apiErr := &APIError{StatusCode: resp.StatusCode}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err == nil && len(raw) > 0 {
		var payload errorPayload
		if json.Unmarshal(raw, &payload) == nil && (payload.Error != errorField{}) {
			apiErr.Code = payload.Error.Code
			apiErr.Message = payload.Error.Message
			apiErr.Type = payload.Error.Type
		} else {
			apiErr.Message = strings.TrimSpace(string(raw))
		}
	}
	if rid := resp.Header.Get("X-Request-Id"); rid != "" {
		apiErr.RequestID = rid
	}
	return apiErr
}

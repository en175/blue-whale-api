package mediakit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/service"
)

// UpstreamError 承载非 2xx 响应的状态码与响应体。上游失败原因通常在 body 里，
// 只报状态码会让排查失去唯一线索。
type UpstreamError struct {
	StatusCode int
	Body       []byte
}

func (e *UpstreamError) Error() string {
	body := strings.TrimSpace(string(e.Body))
	if body == "" {
		return fmt.Sprintf("mediakit upstream status %d", e.StatusCode)
	}
	if len(body) > 512 {
		body = body[:512] + "..."
	}
	return fmt.Sprintf("mediakit upstream status %d: %s", e.StatusCode, body)
}

// Client 是 MediaKit 的上游 HTTP 客户端。它不认识任何具体工具——路径由调用方
// 从注册表取出后传入，客户端只负责鉴权头、代理与错误信封。
type Client struct {
	baseURL string
	apiKey  string
	proxy   string

	// HTTPDo 覆盖实际的 HTTP 往返，供测试注入桩。为 nil 时走带代理的默认客户端。
	HTTPDo func(*http.Request) (*http.Response, error)
}

func NewClient(baseURL, apiKey, proxy string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{baseURL: baseURL, apiKey: apiKey, proxy: proxy}
}

// NewRequest 构造一个带鉴权头的上游请求。path 必须是注册表给出的绝对路径
// （以 / 开头），不接受客户端字符串。
func (c *Client) NewRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("mediakit upstream path must start with /: %q", path)
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	return req, nil
}

// readBody 读取响应体。非 2xx 返回 *UpstreamError（带上游响应体），
// 调用方无需再单独判状态码。
func readBody(resp *http.Response) ([]byte, error) {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: raw}
	}
	return raw, nil
}

// Do 发送请求并返回响应体。非 2xx 返回 *UpstreamError。
func (c *Client) Do(req *http.Request) ([]byte, error) {
	do := c.HTTPDo
	if do == nil {
		httpClient, err := service.GetHttpClientWithProxy(c.proxy)
		if err != nil {
			return nil, err
		}
		do = httpClient.Do
	}
	resp, err := do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readBody(resp)
}

// Post 向绝对上游路径提交 JSON 并返回原始响应体。
func (c *Client) Post(ctx context.Context, path string, body []byte) ([]byte, error) {
	req, err := c.NewRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Get 拉取绝对上游路径并返回原始响应体。
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

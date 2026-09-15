package mediakit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubHTTP(t *testing.T, status int, body string, got *http.Request) *Client {
	t.Helper()
	client := NewClient("https://mk.example", "secret-key", "")
	client.HTTPDo = func(req *http.Request) (*http.Response, error) {
		if got != nil {
			*got = *req
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
	return client
}

func TestClientSetsAuthHeader(t *testing.T) {
	var got http.Request
	client := stubHTTP(t, http.StatusOK, `{"success":true}`, &got)

	raw, err := client.Post(context.Background(), "/api/v1/tools/enhance-image", []byte(`{}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"success":true}`, string(raw))
	assert.Equal(t, "Bearer secret-key", got.Header.Get("Authorization"))
	assert.Equal(t, "application/json", got.Header.Get("Content-Type"))
	assert.Equal(t, "https://mk.example/api/v1/tools/enhance-image", got.URL.String())
}

// TestClientPreservesUpstreamErrorBody 保护本次重构修掉的缺陷：旧实现只报
// 状态码，把上游写在 body 里的失败原因整段丢掉，排障时没有第二条线索。
func TestClientPreservesUpstreamErrorBody(t *testing.T) {
	client := stubHTTP(t, http.StatusBadRequest, `{"success":false,"message":"invalid video_url"}`, nil)

	_, err := client.Post(context.Background(), "/api/v1/tools/enhance-image", []byte(`{}`))
	require.Error(t, err)

	var upstream *UpstreamError
	require.True(t, errors.As(err, &upstream))
	assert.Equal(t, http.StatusBadRequest, upstream.StatusCode)
	assert.Contains(t, upstream.Error(), "invalid video_url")
	assert.Contains(t, upstream.Error(), "400")
}

func TestClientDefaultsBaseURL(t *testing.T) {
	client := NewClient("  ", "k", "")
	assert.Equal(t, DefaultBaseURL, client.baseURL)
}

// TestClientRejectsRelativePath 保护路径契约：上游路径只能来自注册表里的绝对
// 路径，相对路径会让请求打到域名根上。
func TestClientRejectsRelativePath(t *testing.T) {
	client := stubHTTP(t, http.StatusOK, `{}`, nil)

	_, err := client.Post(context.Background(), "api/v1/tools/enhance-image", []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must start with /")
}

// TestTaskPathIsQueryEndpoint 固定任务查询端点形状：异步工具提交走 /tools/，
// 查询走 /tasks/，两者不可互换。
func TestTaskPathIsQueryEndpoint(t *testing.T) {
	assert.Equal(t, "/api/v1/tasks/job-1", TaskPath("job-1"))
}

package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 验收标准：上游返回的 X-Trace-Id 捕获进 context（同 request_id 逻辑），
// 且不覆盖当前请求已有的 trace_id（透传优先语义）
func TestShouldCopyUpstreamHeader_TraceId(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("captures upstream trace id when absent", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		copied := ShouldCopyUpstreamHeader(c, "x-trace-id", []string{"upstream-trace"})
		assert.False(t, copied, "X-Trace-Id 不应直接复制到客户端响应")
		assert.Equal(t, "upstream-trace", c.GetString(common.TraceIdKey))
	})

	t.Run("keeps existing trace id", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(common.TraceIdKey, "local-trace")
		copied := ShouldCopyUpstreamHeader(c, common.TraceIdKey, []string{"upstream-trace"})
		assert.False(t, copied)
		assert.Equal(t, "local-trace", c.GetString(common.TraceIdKey), "已有 trace_id 不被上游覆盖")
	})

	t.Run("request id contract unchanged", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		require.False(t, ShouldCopyUpstreamHeader(c, common.RequestIdKey, []string{"up-req-id"}))
		assert.Equal(t, "up-req-id", c.GetString(common.UpstreamRequestIdKey))
		assert.True(t, ShouldCopyUpstreamHeader(c, "X-Other-Header", []string{"v"}))
	})
}

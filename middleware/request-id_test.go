package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 验收标准：收到带 X-Trace-Id 的请求，透传沿用该值，且响应头返回、写入 context
func TestRequestId_PassthroughTraceId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestId())
	var ctxTraceId, ginTraceId, requestId string
	r.GET("/ping", func(c *gin.Context) {
		ctxTraceId, _ = c.Request.Context().Value(common.TraceIdKey).(string)
		ginTraceId = c.GetString(common.TraceIdKey)
		requestId = c.GetString(common.RequestIdKey)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(common.TraceIdKey, "trace-from-upstream")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "trace-from-upstream", w.Header().Get(common.TraceIdKey), "透传优先：响应头应沿用请求的 X-Trace-Id")
	assert.Equal(t, "trace-from-upstream", ctxTraceId, "透传优先：request context 应沿用请求的 X-Trace-Id")
	assert.Equal(t, "trace-from-upstream", ginTraceId, "透传优先：gin context 应沿用请求的 X-Trace-Id")
	// request_id 契约不变：每次请求独立生成并返回响应头
	assert.NotEmpty(t, w.Header().Get(common.RequestIdKey))
	assert.Equal(t, requestId, w.Header().Get(common.RequestIdKey))
	assert.NotEqual(t, "trace-from-upstream", requestId)
}

// 验收标准：无 X-Trace-Id 的请求，本地生成 trace_id，响应头与 context 一致
func TestRequestId_GenerateTraceIdWhenAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestId())
	var ctxTraceId, ginTraceId string
	r.GET("/ping", func(c *gin.Context) {
		ctxTraceId, _ = c.Request.Context().Value(common.TraceIdKey).(string)
		ginTraceId = c.GetString(common.TraceIdKey)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	traceId := w.Header().Get(common.TraceIdKey)
	assert.NotEmpty(t, traceId, "缺省生成：无 X-Trace-Id 时应本地生成")
	assert.Equal(t, traceId, ctxTraceId)
	assert.Equal(t, traceId, ginTraceId)
	assert.NotEmpty(t, w.Header().Get(common.RequestIdKey))
	assert.NotEqual(t, w.Header().Get(common.RequestIdKey), traceId)
}

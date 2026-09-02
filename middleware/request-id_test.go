package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// 验收标准：入站 X-Trace-Id 必须匹配 ^[A-Za-z0-9_-]{8,64}$，否则另生成，原值留在 client_trace_id
// （IKB29O D6）。此前本仓完全不校验——任何字节都能进入这个每一条跨服务查询都要用的键。
func TestResolveTraceId_RejectsDirtyInput(t *testing.T) {
	for _, dirty := range []string{"has space", "abc", "quote\"inside", "../../etc/passwd", strings.Repeat("x", 65)} {
		traceId, clientTraceId := resolveTraceId(dirty, "")
		assert.NotEqual(t, dirty, traceId, "不合规入参不应被沿用：%q", dirty)
		assert.Equal(t, dirty, clientTraceId, "原值应留在 client_trace_id：%q", dirty)
		assert.Regexp(t, `^[A-Za-z0-9_-]{8,64}$`, traceId, "生成的值必须满足自己强制的规则")
	}
}

func TestResolveTraceId_PassesThroughValidInput(t *testing.T) {
	traceId, clientTraceId := resolveTraceId("  1756800000000-a1b2c3d4  ", "")
	assert.Equal(t, "1756800000000-a1b2c3d4", traceId, "两端空白 trim 后沿用")
	assert.Empty(t, clientTraceId, "沿用时不记 client_trace_id")
}

// 验收标准：没有 X-Trace-Id 时认 traceparent 的 trace-id 段（D4 的廉价保险）。
func TestResolveTraceId_FallsBackToTraceparent(t *testing.T) {
	traceId, _ := resolveTraceId("", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", traceId)

	for _, bad := range []string{"garbage", "00-short-00f067aa0ba902b7-01", ""} {
		generated, _ := resolveTraceId("", bad)
		assert.Regexp(t, `^[A-Za-z0-9_-]{8,64}$`, generated, "traceparent 不合规时应生成：%q", bad)
	}
}

// 验收标准：X-Trace-Id 发了但不合规，不落到 traceparent。两个头都发、第一个写错的调用方，
// 应该看到一个明确是新生成的 id，而不是被悄悄换到另一条链上。
func TestResolveTraceId_MalformedHeaderDoesNotFallThrough(t *testing.T) {
	traceId, clientTraceId := resolveTraceId("bad id", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	assert.NotEqual(t, "4bf92f3577b34da6a3ce929d0e0e4736", traceId)
	assert.Equal(t, "bad id", clientTraceId)
}

// 验收标准：生成格式三仓统一（IKB29O D6）——毫秒时间戳 + 8 字节随机 hex，
// 且必须满足自己强制的校验规则。
//
// 此前这里复用 common.NewRequestId()，那是网关单次请求的标识，形状和 Node 两侧完全不同：
// 看到一个 id 得先知道是谁生成的才能判断它正不正常。
func TestNewTraceId_MatchesSharedShape(t *testing.T) {
	id := newTraceId()
	assert.Regexp(t, `^\d{13}-[0-9a-f]{16}$`, id, "三仓同一形状：<毫秒>-<8字节hex>")
	assert.Regexp(t, traceIdPattern, id, "自己生成的值必须过自己的校验")
	assert.NotEqual(t, newTraceId(), newTraceId(), "每次不同")
}

package middleware

import (
	"context"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// traceIdPattern 入站 X-Trace-Id 的合法形状（IKB29O D6）。
//
// X-Trace-Id 是客户端可控输入，此前本仓完全不校验——任何字节都能进入这个每一条跨服务查询
// 都要用的键。不合规不代表丢线索：本服务另生成一个，原值写进 gin context 的 client_trace_id，
// 由日志带出去。三仓逐字相同的表达式，只改一处会重新造出它替掉的那种分歧。
var traceIdPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// traceparentPattern W3C Trace Context：00-<32位hex>-<16位hex>-<flags>，只取 trace-id 段。
var traceparentPattern = regexp.MustCompile(`^00-([0-9a-f]{32})-[0-9a-f]{16}-[0-9a-f]{2}$`)

// resolveTraceId 透传优先、缺省生成，被拒的原值一并带回。
//
// 顺序是 X-Trace-Id、traceparent、生成。X-Trace-Id 发了但不合规不会落到 traceparent：
// 两个头都发、第一个写错的调用方，应该看到一个明确是新生成的 id，而不是被悄悄换到另一条链上。
func resolveTraceId(headerValue, traceparent string) (traceId, clientTraceId string) {
	candidate := strings.TrimSpace(headerValue)
	if candidate != "" {
		if traceIdPattern.MatchString(candidate) {
			return candidate, ""
		}
		return common.NewRequestId(), candidate
	}
	if m := traceparentPattern.FindStringSubmatch(strings.TrimSpace(traceparent)); m != nil {
		return m[1], ""
	}
	return common.NewRequestId(), ""
}

func RequestId() func(c *gin.Context) {
	return func(c *gin.Context) {
		id := common.NewRequestId()
		c.Set(common.RequestIdKey, id)
		ctx := context.WithValue(c.Request.Context(), common.RequestIdKey, id)

		// 跨系统链路标识 X-Trace-Id：透传优先、校验、缺省生成（IKB29O D6）。
		traceId, clientTraceId := resolveTraceId(
			c.GetHeader(common.TraceIdKey),
			c.GetHeader(common.TraceparentKey),
		)
		c.Set(common.TraceIdKey, traceId)
		ctx = context.WithValue(ctx, common.TraceIdKey, traceId)
		if clientTraceId != "" {
			c.Set(common.ClientTraceIdKey, clientTraceId)
			ctx = context.WithValue(ctx, common.ClientTraceIdKey, clientTraceId)
		}

		c.Request = c.Request.WithContext(ctx)
		c.Header(common.RequestIdKey, id)
		c.Header(common.TraceIdKey, traceId)
		c.Next()
	}
}

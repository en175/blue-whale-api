package middleware

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func RequestId() func(c *gin.Context) {
	return func(c *gin.Context) {
		id := common.NewRequestId()
		c.Set(common.RequestIdKey, id)
		ctx := context.WithValue(c.Request.Context(), common.RequestIdKey, id)

		// 跨系统链路标识 X-Trace-Id：透传优先，缺省生成。
		// 请求头已有则沿用（保证同一链路 trace_id 唯一），否则本地生成。
		traceId := c.GetHeader(common.TraceIdKey)
		if traceId == "" {
			traceId = common.NewRequestId()
		}
		c.Set(common.TraceIdKey, traceId)
		ctx = context.WithValue(ctx, common.TraceIdKey, traceId)

		c.Request = c.Request.WithContext(ctx)
		c.Header(common.RequestIdKey, id)
		c.Header(common.TraceIdKey, traceId)
		c.Next()
	}
}

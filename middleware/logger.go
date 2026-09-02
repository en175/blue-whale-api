package middleware

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

const RouteTagKey = "route_tag"

func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(RouteTagKey, tag)
		c.Next()
	}
}

// SetUpLogger 访问日志（IKB29O）。
//
// 此前是 gin 自己的竖线分隔文本 `[GIN] ... | ... | ...`：它不是 JSON、不带 trace_id、
// 也不进火山 TLS。于是网关每个请求的状态与耗时——排查「慢在哪一跳」最直接的那份数据——
// 在跨服务查询里根本不存在，而这个仓其余的日志早就在里面了。
//
// 现在走统一入口，字段与其余日志逐字一致。formatter 返回空串：日志由 logger 自己写出，
// gin 不再另写一行，否则同一个请求会出现两条格式不同的记录。
func SetUpLogger(server *gin.Engine) {
	server.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		var clientAddress, peerAddress string
		if param.Keys != nil {
			clientAddress, _ = param.Keys[common.ClientAddressKey].(string)
			peerAddress, _ = param.Keys[common.PeerAddressKey].(string)
		}
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}
		fields := map[string]any{
			"event":                     "http.request",
			"http_request_method":       param.Method,
			"url_path":                  param.Path,
			"http_response_status_code": param.StatusCode,
			"duration_ms":               param.Latency.Milliseconds(),
			"route_tag":                 tag,
			"user_agent_original":       param.Request.UserAgent(),
		}
		if clientAddress == "" {
			// RequestId 中间件没跑到（静态资源等直挂在 engine 上的路由）时的兜底。
			clientAddress = param.ClientIP
		}
		if clientAddress != "" {
			fields[common.ClientAddressKey] = clientAddress
		}
		if peerAddress != "" {
			fields[common.PeerAddressKey] = peerAddress
		}
		level := "INFO"
		if param.StatusCode >= 500 {
			level = "WARN"
		}
		// 直接给 ctx：formatter 在请求处理完之后跑，param.Request.Context() 仍带着
		// RequestId 中间件塞进去的 request_id / trace_id / 地址。
		logger.LogWithFields(
			param.Request.Context(),
			level,
			param.Method+" "+param.Path+" "+strconv.Itoa(param.StatusCode),
			fields,
		)
		return ""
	}))
}

package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

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
		return newTraceId(), candidate
	}
	if m := traceparentPattern.FindStringSubmatch(strings.TrimSpace(traceparent)); m != nil {
		return m[1], ""
	}
	return newTraceId(), ""
}

// newTraceId 生成缺省 trace_id：毫秒时间戳 + 8 字节随机 hex（IKB29O D6）。
//
// 不复用 common.NewRequestId()：那是网关**单次请求**的标识，与贯穿多系统的 trace_id 是两个
// 概念，而且它的形状（时间串+前缀+随机）和 Node 两侧的不一样。D6 要求生成格式三仓统一——
// 看到一个 id 不必先知道是谁生成的。三处逐字相同，改一处会重新造出它替掉的那种分歧。
func newTraceId() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// 熵源不可用不该让请求失败：退回时间戳加进程号，仍满足 traceIdPattern。
		return fmt.Sprintf("%d-%08x", time.Now().UnixMilli(), os.Getpid())
	}
	return fmt.Sprintf("%d-%s", time.Now().UnixMilli(), hex.EncodeToString(buf))
}

// ipv4Pattern / ipv6Pattern 一个地址串能不能写进日志。
//
// 不合形状就丢掉，而不是原样写进去——有人读到 client_address 时会当它是事实，而
// X-Forwarded-For 是客户端可控输入。三仓同一条口径。
var (
	ipv4Pattern = regexp.MustCompile(`^\d{1,3}(?:\.\d{1,3}){3}$`)
	ipv6Pattern = regexp.MustCompile(`^[0-9a-fA-F:]{2,45}$`)
)

func sanitizeAddress(value string) string {
	text := strings.TrimSpace(value)
	if text == "" || len(text) > 45 {
		return ""
	}
	// Go 在双栈监听下把 IPv4 报成 ::ffff:10.0.0.1，剥掉前缀，和另外两个仓写出同一种值。
	text = strings.TrimPrefix(text, "::ffff:")
	if ipv4Pattern.MatchString(text) || ipv6Pattern.MatchString(text) {
		return text
	}
	return ""
}

// peerHost 从 RemoteAddr（host:port）里取出 host。IPv6 是 [::1]:1234，所以不能按冒号切。
func peerHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
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

		// 调用方地址（IKB29O）。放进 context 而不是只在访问日志那一行读 gin.Context——
		// 工单要的是「错误场景能看到是哪个 IP 出的问题」，而出错的从来不是你预先标注好的那一行。
		//
		// c.ClientIP() 按 gin 配置的可信代理解析 X-Forwarded-For；peer 直接取连接对端，
		// 两者并存：伪造的 XFF 只能添一个声称，抹不掉连接的事实。
		clientAddress := sanitizeAddress(c.ClientIP())
		peerAddress := sanitizeAddress(peerHost(c.Request.RemoteAddr))
		if clientAddress != "" {
			c.Set(common.ClientAddressKey, clientAddress)
			ctx = context.WithValue(ctx, common.ClientAddressKey, clientAddress)
		}
		if peerAddress != "" {
			c.Set(common.PeerAddressKey, peerAddress)
			ctx = context.WithValue(ctx, common.PeerAddressKey, peerAddress)
		}

		c.Request = c.Request.WithContext(ctx)
		c.Header(common.RequestIdKey, id)
		c.Header(common.TraceIdKey, traceId)
		c.Next()
	}
}

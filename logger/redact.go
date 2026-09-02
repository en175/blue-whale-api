package logger

import (
	"regexp"
	"strconv"
	"strings"
)

// 日志截断与脱敏（IKB29O 决策 D9）。
//
// 决策把这件事放在统一日志入口而不是各调用点：「不依赖调用方自觉」。每一个写日志的地方都是一个
// 有人会忘的地方，而忘掉的代价在日志直写火山 TLS 之后变了——那个 topic 是全企业可检索的，
// 而这条链上流过的正是带签名的素材地址（签名就在 query 里）和上游返回的正文片段。
//
// 与 blue-whale-agent 的 src/observability/redact.ts、blue-whale-ai 的 packages/core/src/redact.ts
// 是同一套规则，改一边就要改三边。
//
// **它不等于日志可以公开。**它删掉的是决策点名的那几类，手写的 msg 仍可以说任何话。
// 这是地板，不是证明。

const (
	// maxBodyChars body 保留的最大字符数（按 rune 计，不是字节——按字节截会把一个汉字劈成
	// 半个，写出无效 UTF-8，而这条链上的日志基本都是中文）。
	maxBodyChars = 4096

	// maxFieldChars 附加字段保留的最大字符数。
	maxFieldChars = 512
)

// urlInText 匹配字符串里任何位置的 http(s) URL。
//
// 不能只判断「整串是不是一个 URL」：它们总是引在上游报错句子中间到达，不是单独一个值。
var urlInText = regexp.MustCompile(`https?://[^\s"'<>\\]+`)

// stripURLQueries 去掉每个 URL 的 query 与 fragment，保留 path。
//
// path 是素材的身份，要留；`?` 与 `#` 之后是预签名凭证，不能留。
func stripURLQueries(text string) string {
	if !strings.Contains(text, "://") {
		return text
	}
	return urlInText.ReplaceAllStringFunc(text, func(match string) string {
		cut := strings.IndexAny(match, "?#")
		if cut < 0 {
			return match
		}
		return match[:cut] + "?…"
	})
}

// redactLogBody body 的脱敏与截断。
func redactLogBody(msg string) string {
	return truncateRunes(stripURLQueries(msg), maxBodyChars)
}

// redactLogField 一个附加字段值的脱敏与截断。键名命中凭证词表时整段抹掉。
func redactLogField(key, value string) string {
	if isSecretKey(key) {
		return "[redacted]"
	}
	return truncateRunes(stripURLQueries(value), maxFieldChars)
}

// secretKeyPatterns 值是凭证的键名，按子串、不分大小写匹配。
//
// 故意不含 "key"：它和太多正常词撞（request_id 之外还有 idempotency_key、cache_key），
// 会把人用来定位的标识抹掉。apikey / api_key / access_key / secret_key 覆盖真实情况。
var secretKeyPatterns = []string{
	"password", "passwd", "secret", "token", "credential",
	"authorization", "apikey", "api_key", "access_key", "private_key",
	"signature", "session_key",
}

func isSecretKey(key string) bool {
	lower := strings.ToLower(key)
	for _, pattern := range secretKeyPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// truncateRunes 按字符（rune）截断并记下被截掉多少——被截的字段要说自己被截了，
// 而不是看上去又完整又短。
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…[+" + strconv.Itoa(len(runes)-limit) + " chars]"
}

package logger

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 验收标准（D9）：素材 URL 只留 path。签名就在 query 里，而 TLS 是全企业可检索的。
func TestStripURLQueries_DropsSignatureKeepsPath(t *testing.T) {
	url := "https://bucket.tos-cn.volces.com/assets/a1/clip.mp4?X-Tos-Signature=deadbeef&X-Tos-Expires=900"
	assert.Equal(t, "https://bucket.tos-cn.volces.com/assets/a1/clip.mp4?…", stripURLQueries(url))
}

// URL 从来不是单独一个值到达的——它们引在上游报错句子中间。整串判断在这里没用。
func TestStripURLQueries_FindsURLInsideASentence(t *testing.T) {
	text := `下载失败 "https://cdn.example.com/x.png?sig=abc"，重试 3 次`
	assert.Equal(t, `下载失败 "https://cdn.example.com/x.png?…"，重试 3 次`, stripURLQueries(text))
}

func TestStripURLQueries_LeavesPlainTextAlone(t *testing.T) {
	assert.Equal(t, "一句没有地址的话", stripURLQueries("一句没有地址的话"))
	assert.Equal(t, "https://example.com/a/b", stripURLQueries("https://example.com/a/b"))
}

// fragment 里也可能挂东西。
func TestStripURLQueries_DropsFragment(t *testing.T) {
	assert.Equal(t, "https://example.com/a?…", stripURLQueries("https://example.com/a#token=xyz"))
}

// 按 rune 截，不按字节：按字节截会把一个汉字劈成半个，写出无效 UTF-8——而这条链上的日志
// 基本都是中文。
func TestRedactLogBody_TruncatesByRunes(t *testing.T) {
	body := strings.Repeat("中", maxBodyChars+10)
	got := redactLogBody(body)
	assert.True(t, strings.HasSuffix(got, "…[+10 chars]"))
	assert.Equal(t, maxBodyChars, len([]rune(strings.TrimSuffix(got, "…[+10 chars]"))))
	assert.True(t, strings.HasPrefix(got, "中中中"))
}

func TestRedactLogField_RedactsCredentialsByKeyName(t *testing.T) {
	assert.Equal(t, "[redacted]", redactLogField("authorization", "Bearer sk-live-1"))
	assert.Equal(t, "[redacted]", redactLogField("AccessKeySecret", "secret-value"))
	assert.Equal(t, "[redacted]", redactLogField("x_api_key", "k-1"))
}

// key 作为子串会把人用来定位的标识一起抹掉，所以词表里故意没有它。
func TestRedactLogField_KeepsIdentifiersPeopleNavigateBy(t *testing.T) {
	assert.Equal(t, "req-7", redactLogField("request_id", "req-7"))
	assert.Equal(t, "idem-1", redactLogField("idempotency_key", "idem-1"))
}

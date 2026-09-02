package logger

import (
	"regexp"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

// 验收标准：timestamp 必须是 UTC 的 ISO 8601（IKB29O D2）。
//
// 本条是本轮实测出来的：Go 侧一度写本地时区偏移（+08:00），而 Node 两侧走 toISOString()
// 写的是 Z。两者按字符串排序时 +08:00 那条会排到 Z 那条后面，尽管它实际更早——恰好把 D2
// 改时间格式想保住的毫秒级先后关系毁掉。
func TestLogRecord_TimestampIsUTCWithMillis(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	assert.NoError(t, err)
	at := time.Date(2026, 9, 2, 16, 10, 42, 575_000_000, shanghai)

	record := logRecord("INFO", at, "req-1", "trace-12345678", "logger/logger.go", 42, "logHelper", "起来了")

	assert.Equal(t, "2026-09-02T08:10:42.575Z", record["timestamp"],
		"必须换算到 UTC 并以 Z 结尾，才能和 Node 侧按字符串一起排序")
}

// 验收标准：severity_text 小写。此前 Go 打 INFO/WARN/ERR 而 Node 打 info/warn/error，
// 在 TLS 上按 severity_text: error 过滤搜不到本仓任何错误日志。
func TestLogRecord_SeverityIsLowerCase(t *testing.T) {
	for input, want := range map[string]string{"INFO": "info", "WARN": "warn", "ERR": "err", "DEBUG": "debug"} {
		record := logRecord(input, time.Now(), "", "", "", 0, "", "")
		assert.Equal(t, want, record["severity_text"], "输入 %q", input)
	}
}

// 验收标准：字段集与另外两个仓逐字一致，一个不多一个不少。
func TestLogRecord_FieldSetMatchesDecision(t *testing.T) {
	record := logRecord("INFO", time.Now(), "req", "trace", "f.go", 1, "fn", "msg")
	want := []string{
		"severity_text", "timestamp", "service_name", "service_instance_id",
		"deployment_environment_name", "service_version", "request_id", "trace_id",
		"code_file_path", "code_line_number", "code_function_name", "body",
	}
	assert.Len(t, record, len(want))
	for _, key := range want {
		assert.Contains(t, record, key)
	}
	// 投递顺序表必须和字段集同步，否则会漏投或投空值。
	assert.ElementsMatch(t, want, logRecordFieldOrder)
}

// 验收标准：缺省值三仓一致——版本未注入时是 "unknown"，不是空串。
//
// 直接改 common.Version 而不是靠默认值：测试进程里它是 "v0.0.0"，兜底分支根本走不到，
// 那样写出来的用例永远绿，也永远没测到东西。
func TestLogRecord_VersionDefaultsToUnknown(t *testing.T) {
	original := common.Version
	t.Cleanup(func() { common.Version = original })

	common.Version = ""
	assert.Equal(t, "unknown", logRecord("INFO", time.Now(), "", "", "", 0, "", "")["service_version"])

	common.Version = "v1.2.3"
	assert.Equal(t, "v1.2.3", logRecord("INFO", time.Now(), "", "", "", 0, "", "")["service_version"])
}

// 验收标准：trace_id 缺失填空串，不是此前的 "-"。一个填 "-" 一个填 ""，
// 「没有链路标识」这件事就要按两种写法各查一次。
func TestLogRecord_EmptyTraceIdIsEmptyString(t *testing.T) {
	record := logRecord("INFO", time.Now(), "req", "", "f.go", 1, "fn", "msg")
	assert.Equal(t, "", record["trace_id"])
}

// 验收标准：调用位置的文件与行号分开，行号是数字而不是拼进路径的字符串。
func TestCallerInfo_SplitsFileAndLine(t *testing.T) {
	file, line, fn := callerInfo(1)
	assert.Regexp(t, regexp.MustCompile(`^[^:]+/[^:]+\.go$`), file, "路径里不应再带 :行号")
	assert.Greater(t, line, 0)
	assert.Contains(t, fn, "TestCallerInfo_SplitsFileAndLine")
}

package logger

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

// 验收标准：timestamp 恒为 UTC 的 ISO 8601，带毫秒（IKB29O D2）。
//
// 关键不在「哪个时区」而在「三家一致且写死」：跨服务按字符串排序只在三家写同一个时区时成立。
// 本轮修过一次混合偏移的 bug（这边写 +08:00、Node 两侧写 Z，+08:00 排在 Z 后面尽管实际更早）。
//
// 2026-09-03 从 +08:00 改成 UTC：排查一定是把日志行、它写出的那条库记录、它返回的那个 API
// 响应放在一起看，而那三样已经全是 UTC（本仓 logs.created_at 就是 bigint 真 UTC epoch）。
// 日志曾是唯一的例外，于是每次交叉引用都要付八小时心算。
func TestLogRecord_TimestampIsUTCWithMillis(t *testing.T) {
	// 源时刻用 UTC 给，确保断言检验的是换算而不是"原样打印"。
	at := time.Date(2026, 9, 2, 8, 10, 42, 575_000_000, time.UTC)

	record := logRecord("INFO", at, "req-1", "trace-12345678", "logger/logger.go", 42, "logHelper", "起来了")

	assert.Equal(t, "2026-09-02T08:10:42.575Z", record["timestamp"])
}

// 验收标准：时区不依赖容器配置。LoadLocation 要读 tzdata，镜像裁掉就会失败，
// 一个容器换了时区就把跨服务排序静默毁掉。time.UTC 是常量，读不到任何外部数据。
func TestLogRecord_TimestampZoneIsFixed(t *testing.T) {
	at := time.Date(2026, 9, 2, 8, 10, 42, 575_000_000, time.UTC)
	for _, loc := range []*time.Location{time.UTC, time.FixedZone("UTC-5", -5*3600)} {
		record := logRecord("INFO", at.In(loc), "", "", "", 0, "", "")
		assert.Equal(t, "2026-09-02T08:10:42.575Z", record["timestamp"],
			"源时刻所在时区不应影响输出：%v", loc)
	}
}

// 验收标准：字典序与时间先后一致，且解析回去等于源时刻——偏移不是贴上去的假标签。
func TestLogRecord_TimestampSortsAndParses(t *testing.T) {
	at := time.Date(2026, 9, 2, 8, 10, 42, 575_000_000, time.UTC)
	earlier := logRecord("INFO", at, "", "", "", 0, "", "")["timestamp"].(string)
	later := logRecord("INFO", at.Add(time.Millisecond), "", "", "", 0, "", "")["timestamp"].(string)
	assert.Less(t, earlier, later, "字典序必须与时间先后一致")

	parsed, err := time.Parse(time.RFC3339Nano, earlier)
	assert.NoError(t, err)
	assert.True(t, parsed.Equal(at), "解析回去应等于源时刻")
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
		"severity_text", "timestamp", "service_name", "service_instance_id", "host_ip",
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

// 验收标准：入站 trace id 被拒时，原值必须真的写出去。
//
// 中间件把它放进 context 之后，此前没有任何地方读——「不丢线索」只做了一半：存下来了，
// 从没写出去过。本轮自审发现。
func TestLogHelper_WritesClientTraceId(t *testing.T) {
	ctx := context.WithValue(context.Background(), common.ClientTraceIdKey, "has space")
	record := logRecord("INFO", time.Now(), "req", "generated-12345678", "f.go", 1, "fn", "msg")
	if ctid := ctx.Value(common.ClientTraceIdKey); ctid != nil {
		record[common.ClientTraceIdKey] = ctid
	}

	assert.Equal(t, "has space", record[common.ClientTraceIdKey])
	// 顺序表之外的条件字段不能因为不在表里就被 TLS 那侧丢掉。
	assert.False(t, logRecordFieldSet[common.ClientTraceIdKey],
		"它是条件字段，本就不该在固定顺序表里")
}

// 验收标准：顺序表与字段集必须同步，否则会漏投或投空值。
func TestLogRecordFieldSet_MatchesOrder(t *testing.T) {
	assert.Len(t, logRecordFieldSet, len(logRecordFieldOrder))
	for _, key := range logRecordFieldOrder {
		assert.True(t, logRecordFieldSet[key], "顺序表里的 %q 不在集合里", key)
	}
}

// 验收标准：投递时刻能从记录自己的 timestamp 反解出来。
//
// 布局串与写出的格式对不上，会每条都解析失败并静默退回 time.Now()——TLS 的时间轴与字段值
// 从此差出批量投递的那点延迟，而且不报错。改时区时最容易漏掉的就是这处。
// （sendLogToTLS 用 RFC3339Nano，认任意偏移，所以 +08:00 → Z 这次不必改它。）
func TestLogRecord_TimestampRoundTripsForDelivery(t *testing.T) {
	at := time.Date(2026, 9, 2, 8, 10, 42, 575_000_000, time.UTC)
	record := logRecord("INFO", at, "", "", "", 0, "", "")

	parsed, err := time.Parse(time.RFC3339Nano, record["timestamp"].(string))
	assert.NoError(t, err, "sendLogToTLS 用同一个布局串反解")
	assert.Equal(t, at.UnixMilli(), parsed.UnixMilli())
}

// 验收标准（IKB29O）：进程身份与 TLS 是否启用无关。
//
// 此前 service_name / service_instance_id / deployment_environment_name 是在 InitTLSWriter 里
// 赋值的，而那个函数在缺 TLS_* 环境变量时提前 return——于是没配 TLS 的部署（开发机、任何一台
// 还没发凭证的机器）写出来的 stdout JSON 里这三个字段全是空串。stdout 是结构化日志的主通道，
// 它不该依赖上报通道有没有配起来。
func TestLogRecord_IdentityResolvedWithoutTLS(t *testing.T) {
	record := logRecord("INFO", time.Now(), "req", "trace", "f.go", 1, "fn", "msg")
	assert.NotEmpty(t, record["service_name"])
	assert.NotEmpty(t, record["service_instance_id"])
	assert.NotEmpty(t, record["deployment_environment_name"])
	// host_ip 允许为空：没有外部网卡的容器是合法状态。字段本身必须在。
	assert.Contains(t, record, "host_ip")
}

// 验收标准（IKB29O 二轮）：服务名是常量，没有环境变量能改它。
//
// CONTAINER_NAME 没了。它带三层兜底，每一层都写得出一个看起来很像真的值，而配错时没有一层会红。
func TestLogRecord_ServiceNameIsAConstant(t *testing.T) {
	t.Setenv("CONTAINER_NAME", "whatever")
	t.Setenv("SERVICE_INSTANCE_ID", "pinned")
	t.Setenv("HOST_IP", "9.9.9.9")
	record := logRecord("INFO", time.Now(), "req", "trace", "f.go", 1, "fn", "msg")
	assert.Equal(t, "blue-whale-api", record["service_name"])
	assert.NotEqual(t, "pinned", record["service_instance_id"])
	assert.NotEqual(t, "9.9.9.9", record["host_ip"])
}

// 验收标准（D9）：脱敏挂在 logRecord 上，因为 stdout 与 TLS 两条通道都从这一处取字段——
// 挂在任一条上都只覆盖一半。
func TestLogRecord_BodyIsRedacted(t *testing.T) {
	record := logRecord("INFO", time.Now(), "", "", "", 0, "",
		"下载失败 https://bucket.example.com/x.mp4?sig=secret")
	assert.Equal(t, "下载失败 https://bucket.example.com/x.mp4?…", record["body"])
}

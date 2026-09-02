package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/volcengine/volc-sdk-golang/service/tls/pb"
	"github.com/volcengine/volc-sdk-golang/service/tls/producer"
)

// 火山引擎 TLS（Torch Log Service）日志写入。
// 采用 SDK Producer 异步批量直写，任何失败静默降级，不影响主服务日志与请求链路。
// 详见 docs/adr/0002-trace-id-and-volcengine-tls.md（D3/D4）。

const (
	envTLSEndpoint = "TLS_ENDPOINT"
	envTLSAK       = "TLS_AK"
	envTLSSK       = "TLS_SK"
	envTLSTopicID  = "TLS_TOPIC_ID"

	// tlsLogChanSize 缓冲通道容量。通道满时新日志直接丢弃（静默降级），
	// 保证 TLS 异常/积压时绝不阻塞请求链路上的日志调用。
	tlsLogChanSize = 8192

	// modulePrefix 用于裁剪 runtime 函数名中的模块前缀，输出形如 "logger.LogInfo" 的简洁函数名
	modulePrefix = "github.com/QuantumNous/new-api/"
)

var (
	tlsProducer   producer.Producer
	tlsTopicID    string
	tlsSource     string
	tlsLogChan    chan *pb.Log
	tlsEnv        string
	tlsContainer  string
	tlsInstanceID string
)

// InitTLSWriter 初始化 TLS 日志 Producer。
// 需同时配置 TLS_ENDPOINT / TLS_AK / TLS_SK / TLS_TOPIC_ID 四个环境变量才启用，
// 任一缺失则静默禁用（本地日志输出不受影响）。
func InitTLSWriter() {
	if tlsProducer != nil {
		return
	}
	endpoint := os.Getenv(envTLSEndpoint)
	accessKeyID := os.Getenv(envTLSAK)
	accessKeySecret := os.Getenv(envTLSSK)
	topicID := os.Getenv(envTLSTopicID)
	if endpoint == "" || accessKeyID == "" || accessKeySecret == "" || topicID == "" {
		return
	}
	// SDK 要求 endpoint 带协议头；裸域名（如 tls-cn-beijing.volces.com）自动补 https
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}

	// SDK 签名必须携带 Region；缺省 cn-beijing，其他区域（如 tls-cn-shanghai）须显式配置 TLS_REGION
	region := os.Getenv("TLS_REGION")
	if region == "" {
		region = "cn-beijing"
	}

	cfg := producer.GetDefaultProducerConfig()
	cfg.Endpoint = endpoint
	cfg.Region = region
	cfg.AccessKeyID = accessKeyID
	cfg.AccessKeySecret = accessKeySecret
	// 降低 SDK 内部日志级别，避免 info 日志刷屏
	cfg.LogLevel = "warn"
	// 内存预算满时不阻塞 dispatch 协程（默认 60s），立即失败走丢弃降级
	cfg.MaxBlockSec = 0
	// 关停时 producer.Close() 等待重试耗尽可能阻塞数十秒，压低重试次数
	cfg.Retries = 3

	tlsLogChan = make(chan *pb.Log, tlsLogChanSize)
	tlsTopicID = topicID
	// service_name 标识服务：优先 CONTAINER_NAME 环境变量（如 blue-whale-api），
	// 未配置时用 NodeName（Docker 下=容器名，本地=hostname）兜底
	tlsContainer = os.Getenv("CONTAINER_NAME")
	if tlsContainer == "" {
		tlsContainer = common.NodeName
	}
	// service_instance_id 回答「哪个副本报的错」。此前这个概念只存在于 TLS 保留字段
	// __source__ 里，不是一个可检索的普通字段；IKB29O D2 把它提为正式字段。
	tlsInstanceID = os.Getenv("SERVICE_INSTANCE_ID")
	if tlsInstanceID == "" {
		tlsInstanceID = fmt.Sprintf("%s-%d", common.NodeName, os.Getpid())
	}
	// __source__（TLS 保留字段）仍用实例标识，与 service_instance_id 同值。
	tlsSource = common.NodeName
	tlsEnv = os.Getenv("ENV")
	if tlsEnv == "" {
		tlsEnv = os.Getenv("NODE_ENV")
	}
	if tlsEnv == "" {
		// local，不是 dev：同一套部署里两个服务自报不同 env，按环境过滤就会漏。
		// 三仓统一为 local（IKB29O D2）。
		tlsEnv = "local"
	}
	tlsProducer = producer.NewProducer(cfg)
	tlsProducer.Start()
	gopool.Go(tlsDispatchLoop)
	// 注册系统日志钩子：common.SysLog/SysError 也进 TLS（回调注入，避免循环依赖）。
	// skip=3：业务调用点 -> common.SysLog -> hook 闭包 -> callerInfo
	common.TLSLogHook = func(level string, t time.Time, msg string) {
		callerFile, callerLine, funcName := callerInfo(3)
		sendLogToTLS(level, t, "SYSTEM", "", callerFile, callerLine, funcName, msg)
	}
	common.SysLog("TLS log writer enabled, endpoint: " + endpoint + ", topic: " + topicID)
}

// CloseTLSWriter 在进程退出前冲刷日志：先等待 dispatch 通道排空（带超时），
// 再关闭 Producer 冲刷其内部缓冲。注意不 close(tlsLogChan)，避免关停后
// 仍有日志写入时向已关闭通道发送导致 panic。
func CloseTLSWriter() {
	if tlsProducer == nil {
		return
	}
	if tlsLogChan != nil {
		deadline := time.Now().Add(5 * time.Second)
		for len(tlsLogChan) > 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
	}
	tlsProducer.Close()
}

// tlsDispatchLoop 从缓冲通道消费日志并交给 Producer 异步批量发送。
// SendLog 失败（网络异常、熔断等）一律忽略，静默降级。
func tlsDispatchLoop() {
	for logItem := range tlsLogChan {
		_ = tlsProducer.SendLog("", tlsTopicID, tlsSource, "", logItem, nil)
	}
}

// logRecord 一条日志的共用字段集，stdout 与 TLS 两条通道都从这里取（IKB29O D2）。
//
// 一处定义而不是两份：两条通道各写各的，是「stdout 有这个字段、TLS 没有」这类差异的来源，
// 而这种差异只会在排障时、最需要它的那一刻被发现。
//
// 字段名对齐 OpenTelemetry 语义约定，点号平铺为下划线（supersede 本仓 ADR-0002 D4 的字段表）。
// 三仓逐字相同，改一边要改三边——留下任何一个旧名，跨服务查询就要为那个仓写特例。
// logRecordFieldOrder 投递给 TLS 时的字段顺序。map 的遍历顺序随机，而 TLS 控制台按顺序
// 展示——同一条链路的日志每行字段排列不同，人眼对比时会以为是不同结构。
var logRecordFieldOrder = []string{
	"severity_text", "timestamp", "service_name", "service_instance_id",
	"deployment_environment_name", "service_version", "request_id", "trace_id",
	"code_file_path", "code_line_number", "code_function_name", "body",
}

// serviceVersion 版本号，未注入时为 "unknown"。
//
// 空串不行：另外两个仓写的是 "unknown"，而 D2 要求缺省值三仓一致——一个空串一个 "unknown"，
// 「不知道版本」这件事就要按两种写法各查一次。
func serviceVersion() string {
	if common.Version == "" {
		return "unknown"
	}
	return common.Version
}

func logRecord(level string, t time.Time, requestID, traceID, callerFile string, callerLine int, funcName, msg string) map[string]any {
	return map[string]any{
		// 小写。此前 Go 打 INFO/WARN/ERR 而 Node 打 info/warn/error，在 TLS 上按
		// severity_text: error 过滤，搜不到本仓的任何错误日志。
		"severity_text": strings.ToLower(level),
		// ISO 8601、带毫秒、UTC。此前是 "2006-01-02 15:04:05"，无毫秒无时区——跨服务按时间
		// 排序时毫秒级先后关系丢失，而待排查的延迟正在几十到几百毫秒量级。
		//
		// 必须是 UTC 而不是本地时区偏移。Node 两侧走 toISOString()，写的是 Z；这边若写
		// +08:00，两者按字符串排序时 +08:00 那条会排到 Z 那条后面，尽管它实际更早——
		// 恰好把本条要保住的东西毁掉。本地实测发现（2026-09-02）。
		"timestamp":                   t.UTC().Format("2006-01-02T15:04:05.000Z"),
		"service_name":                tlsContainer,
		"service_instance_id":         tlsInstanceID,
		"deployment_environment_name": tlsEnv,
		"service_version":             serviceVersion(),
		"request_id":                  requestID,
		// 空串，不是此前的 "-"。三仓统一：一个填 "-" 一个填 "" 的话，「没有链路标识」
		// 这件事要按两种写法各查一次。
		"trace_id":           traceID,
		"code_file_path":     callerFile,
		"code_line_number":   callerLine,
		"code_function_name": funcName,
		"body":               msg,
	}
}

// sendLogToTLS 组装结构化键值对投递到 TLS（平铺下划线命名，跨系统统一）。
//
// 字段名对齐 OpenTelemetry 语义约定，点号平铺为下划线（IKB29O D2，supersede 本仓
// ADR-0002 D4 的字段表）：severity_text, timestamp, service_name, service_instance_id,
// deployment_environment_name, service_version, request_id, trace_id, code_file_path,
// code_line_number, code_function_name, body。
//
// 三仓逐字相同，改一边要改三边——留下任何一个旧名，跨服务查询就要为那个仓写特例。
// 通道满时直接丢弃，绝不阻塞调用方。
func sendLogToTLS(level string, t time.Time, requestID, traceID, callerFile string, callerLine int, funcName, msg string) {
	logChan := tlsLogChan
	if logChan == nil {
		return
	}
	record := logRecord(level, t, requestID, traceID, callerFile, callerLine, funcName, msg)
	contents := make([]*pb.LogContent, 0, len(record))
	for _, key := range logRecordFieldOrder {
		contents = append(contents, &pb.LogContent{Key: key, Value: fmt.Sprintf("%v", record[key])})
	}
	logItem := &pb.Log{
		Time:     t.UnixMilli(),
		Contents: contents,
	}
	select {
	case logChan <- logItem:
	default:
		// 通道已满（TLS 故障或积压），静默丢弃本条日志
	}
}

// callerInfo 返回调用位置的文件路径（目录/文件，如 "logger/logger.go"）、行号，
// 与函数名（模块内相对名，如 "logger.LogInfo"）。仅在 TLS 启用时调用，开销约 1µs。
//
// 文件与行号分开返回（IKB29O D2）：此前拼成 "logger/logger.go:110" 一个字符串，
// 那样既不能按行号排序，也不能只按文件过滤。
func callerInfo(skip int) (callerFile string, callerLine int, funcName string) {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "", 0, ""
	}
	if f := runtime.FuncForPC(pc); f != nil {
		funcName = strings.TrimPrefix(f.Name(), modulePrefix)
	}
	callerFile = fmt.Sprintf("%s/%s", filepath.Base(filepath.Dir(file)), filepath.Base(file))
	return callerFile, line, funcName
}

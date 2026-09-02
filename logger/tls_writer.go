package logger

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	tlsInstanceID string
	tlsHostIP     string

	// identityOnce 进程身份只解析一次，且**与 TLS 是否启用无关**。
	//
	// 此前这几个值是在 InitTLSWriter 里赋的，而那个函数在缺 TLS_* 环境变量时提前 return——
	// 于是没配 TLS 的部署（开发机、任何一台还没发凭证的机器）写出来的 stdout JSON 里
	// deployment_environment_name / service_instance_id 全是空串。
	// stdout 是结构化日志的主通道，它不该依赖上报通道有没有配起来。
	identityOnce sync.Once
)

// serviceName 本仓报出去的服务名。
//
// 常量，不是环境变量。曾经是 CONTAINER_NAME，缺省回退 NodeName 再回退 hostname——三层兜底，
// 每一层都写得出一个看起来很像真的值，而没有一层会在配错时红。服务名是代码的事实，不是机器上的
// 一个空格，把它交给部署去填，就是把日志格式的正确性押在有人记得填。
const serviceName = "blue-whale-api"

// resolveProcessIdentity 解析实例、环境、本机地址。幂等，随便谁先调用。
//
// **这里没有一个值要配。** 早先版本读 CONTAINER_NAME / SERVICE_INSTANCE_ID / HOST_IP，
// 每个都带兜底；漏配一个不会报错，只会安静地写出一个错值。下面每个值都从进程本来就确知的东西里
// 直接得出。唯一的例外是 deployment_environment_name：这是哪一套部署，进程内部推不出来，
// 只能由外面断言，读的是三仓同一个名字 NODE_ENV。
func resolveProcessIdentity() {
	identityOnce.Do(func() {
		// service_instance_id 回答「哪个副本报的错」。此前这个概念只存在于 TLS 保留字段
		// __source__ 里，不是一个可检索的普通字段；IKB29O D2 把它提为正式字段。
		//
		// NodeName 由 common 包的启动初始化赋值，而日志可能在那之前就被调用——那时它是空串。
		// 自己再取一次主机名：这个字段的值不该取决于另一个包的初始化顺序。容器里主机名就是容器 id。
		node := common.NodeName
		if node == "" {
			node, _ = os.Hostname()
		}
		tlsInstanceID = fmt.Sprintf("%s-%d", node, os.Getpid())
		// host_ip 回答「哪个 IP 报的错」。service_instance_id 在 k8s 里的主机名那一半是 Pod 名，
		// 有用，但跟抓包、防火墙日志对不起来。扫第一块非环回网卡得出，容器里拿到的就是容器自己的地址。
		tlsHostIP = firstNonLoopbackAddress()
		tlsEnv = os.Getenv("NODE_ENV")
		if tlsEnv == "" {
			// unknown，不是从前那个 local：生产容器里的 local 是一句读起来像事实的假话。
			// 整个集群现在正处在这个状态——只有 agent 容器设了 NODE_ENV，本服务和
			// blue-whale-backend 在同一套栈里都自报 local。写 unknown 至少看得见这个洞。
			// 三仓同一个缺省（IKB29O D2）。
			tlsEnv = "unknown"
		}
	})
}

// firstNonLoopbackAddress 本机第一块非环回网卡的地址，IPv4 优先。
//
// 取不到就是空串，那是合法状态（没有外部网卡的容器），不值得为它报一条错——而且这里报错
// 会在日志初始化阶段递归回日志。
func firstNonLoopbackAddress() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	var fallback string
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || ipNet.IP.IsUnspecified() {
			continue
		}
		if v4 := ipNet.IP.To4(); v4 != nil {
			return v4.String()
		}
		if fallback == "" {
			fallback = ipNet.IP.String()
		}
	}
	return fallback
}

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
	resolveProcessIdentity()
	// __source__（TLS 保留字段）仍用实例标识，与 service_instance_id 同值。
	tlsSource = common.NodeName
	tlsProducer = producer.NewProducer(cfg)
	tlsProducer.Start()
	gopool.Go(tlsDispatchLoop)
	// 注册系统日志钩子：common.SysLog/SysError 也进 TLS（回调注入，避免循环依赖）。
	// skip=3：业务调用点 -> common.SysLog -> hook 闭包 -> callerInfo
	common.TLSLogHook = func(level string, t time.Time, msg string) {
		callerFile, callerLine, funcName := callerInfo(3)
		sendLogToTLS(logRecord(level, t, "SYSTEM", "", callerFile, callerLine, funcName, msg))
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
// logTimeZone 日志时间戳的固定时区：+08:00，三仓共用的那一个墙上时钟。
//
// **FixedZone 而不是 LoadLocation("Asia/Shanghai")**：后者要读容器里的 tzdata，镜像没装或裁掉
// 时会失败，届时只能退回 UTC——而跨服务按字符串排序只在三家偏移**完全一致**时成立，一个容器
// 退回 UTC 就把排序静默毁掉。那正是本轮修过一次的 bug（Go 写 +08:00、Node 写 Z，+08:00 排在
// Z 后面尽管实际更早）。固定偏移不依赖任何外部数据，和原来的 UTC 一样确定。
//
// 中国不实行夏令时，固定 +08:00 全年成立。
var logTimeZone = time.FixedZone("UTC+8", 8*60*60)

var logRecordFieldSet = func() map[string]bool {
	set := make(map[string]bool, len(logRecordFieldOrder))
	for _, key := range logRecordFieldOrder {
		set[key] = true
	}
	return set
}()

var logRecordFieldOrder = []string{
	"severity_text", "timestamp", "service_name", "service_instance_id", "host_ip",
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
	// 身份与 TLS 是否启用无关：没配 TLS 的部署照样要写出完整的 stdout JSON。
	resolveProcessIdentity()
	return map[string]any{
		// 小写。此前 Go 打 INFO/WARN/ERR 而 Node 打 info/warn/error，在 TLS 上按
		// severity_text: error 过滤，搜不到本仓的任何错误日志。
		"severity_text": strings.ToLower(level),
		// ISO 8601、带毫秒、恒为 +08:00。此前是 "2006-01-02 15:04:05"，无毫秒无时区——跨服务
		// 按时间排序时毫秒级先后关系丢失，而待排查的延迟正在几十到几百毫秒量级。
		"timestamp":                   t.In(logTimeZone).Format("2006-01-02T15:04:05.000-07:00"),
		"service_name":                serviceName,
		"service_instance_id":         tlsInstanceID,
		"host_ip":                     tlsHostIP,
		"deployment_environment_name": tlsEnv,
		"service_version":             serviceVersion(),
		"request_id":                  requestID,
		// 空串，不是此前的 "-"。三仓统一：一个填 "-" 一个填 "" 的话，「没有链路标识」
		// 这件事要按两种写法各查一次。
		"trace_id":           traceID,
		"code_file_path":     callerFile,
		"code_line_number":   callerLine,
		"code_function_name": funcName,
		// 截断与脱敏（D9）在这里，因为 stdout 与 TLS 两条通道都从这一处取字段。
		"body": redactLogBody(msg),
	}
}

// sendLogToTLS 把一条已经组装好的记录投递到 TLS。
//
// 收 record 而不是自己再组装一遍：两条通道各组各的，正是「stdout 有这个字段、TLS 没有」
// 这类差异的来源，而这种差异只会在最需要它的那一刻被发现。client_trace_id 就是现成的例子——
// 它是条件字段，只在入站 trace id 被拒时出现。
//
// 通道满时直接丢弃，绝不阻塞调用方。
func sendLogToTLS(record map[string]any) {
	logChan := tlsLogChan
	if logChan == nil {
		return
	}
	contents := make([]*pb.LogContent, 0, len(record))
	for _, key := range logRecordFieldOrder {
		contents = append(contents, &pb.LogContent{Key: key, Value: fmt.Sprintf("%v", record[key])})
	}
	// 顺序表之外的条件字段（目前只有 client_trace_id）附在后面，不因为不在表里就被丢掉。
	for key, value := range record {
		if !logRecordFieldSet[key] {
			contents = append(contents, &pb.LogContent{Key: key, Value: fmt.Sprintf("%v", value)})
		}
	}
	// 投递时刻从记录自己的 timestamp 反解，两者必须同一个布局串：写 +08:00 却按 Z 解析会每条
	// 都失败并静默退回 time.Now()，于是 TLS 的时间轴与字段值差出批量投递的那点延迟，而且不报错。
	// RFC3339Nano 认任意偏移，改偏移不必再改这里。
	timestamp, _ := record["timestamp"].(string)
	at, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		at = time.Now()
	}
	logItem := &pb.Log{
		Time:     at.UnixMilli(),
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

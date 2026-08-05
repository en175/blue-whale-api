package logger

import (
	"os"
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
)

var (
	tlsProducer producer.Producer
	tlsTopicID  string
	tlsSource   string
	tlsLogChan  chan *pb.Log
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

	cfg := producer.GetDefaultProducerConfig()
	cfg.Endpoint = endpoint
	cfg.AccessKeyID = accessKeyID
	cfg.AccessKeySecret = accessKeySecret
	// 降低 SDK 内部日志级别，避免 info 日志刷屏
	cfg.LogLevel = "warn"

	tlsLogChan = make(chan *pb.Log, tlsLogChanSize)
	tlsTopicID = topicID
	tlsSource = common.NodeName
	tlsProducer = producer.NewProducer(cfg)
	tlsProducer.Start()
	gopool.Go(tlsDispatchLoop)
	common.SysLog("TLS log writer enabled, endpoint: " + endpoint + ", topic: " + topicID)
}

// CloseTLSWriter 在进程退出前冲刷 Producer 缓冲的日志。
func CloseTLSWriter() {
	if tlsProducer == nil {
		return
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

// sendLogToTLS 组装结构化键值对 {level, time, request_id, trace_id, message} 投递到 TLS。
// 通道满时直接丢弃，绝不阻塞调用方。
func sendLogToTLS(level string, t time.Time, requestID, traceID, msg string) {
	logChan := tlsLogChan
	if logChan == nil {
		return
	}
	logItem := &pb.Log{
		Time: t.Unix(),
		Contents: []*pb.LogContent{
			{Key: "level", Value: level},
			{Key: "time", Value: t.Format("2006-01-02 15:04:05")},
			{Key: "request_id", Value: requestID},
			{Key: "trace_id", Value: traceID},
			{Key: "message", Value: msg},
		},
	}
	select {
	case logChan <- logItem:
	default:
		// 通道已满（TLS 故障或积压），静默丢弃本条日志
	}
}

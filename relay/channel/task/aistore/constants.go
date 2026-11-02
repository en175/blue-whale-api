package aistore

// ChannelName 是渠道在日志/管理端展示的名字，与 constant.ChannelTypeNames 中的
// "AistoreVolc" 对应。
const ChannelName = "aistore-volc"

// LegacyChannelName 是渠道 72 的展示名：同一运河启源上游，对客走旧视频接口。
const LegacyChannelName = "aistore-volc-legacy"

// 输出分辨率档。样片（draft）只允许 480p，成片（finalize）固定 1080p——
// 这两个值同时也是档位价表的键。
const (
	Res480p  = "480p"
	Res1080p = "1080p"
)

// UserAgent 是访问运河启源的固定 UA。上游 WAF 对 GET 请求直接拒绝 Go 默认
// 的 Go-http-client/1.1（返回 HTML 403），轮询解析会一直失败、任务永远停在
// queued。POST 不受影响，所以只在轮询路径上表现。
const UserAgent = "aistore-volc/1.0"

// ModelList 是运河启源默认对客模型名，直接使用厂商 ID，渠道 model_mapping
// 默认留空。2.5 是样片模式的唯一支持者。
var ModelList = []string{
	"cdance2.0",
	"cdance2.0-fast",
	"cdance2.0-mini",
	"cdance2.5",
}

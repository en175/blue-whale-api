package aistore

// ChannelName 是渠道在日志/管理端展示的名字，与 constant.ChannelTypeNames 中的
// "AistoreVolc" 对应。
const ChannelName = "aistore-volc"

// 输出分辨率档。样片（draft）只允许 480p，成片（finalize）固定 1080p——
// 这两个值同时也是档位价表的键。
const (
	Res480p  = "480p"
	Res1080p = "1080p"
)

// ModelList 是运河启源默认对客模型名，直接使用厂商 ID，渠道 model_mapping
// 默认留空。2.5 是样片模式的唯一支持者。
var ModelList = []string{
	"cdance2.0",
	"cdance2.0-fast",
	"cdance2.0-mini",
	"cdance2.5",
}

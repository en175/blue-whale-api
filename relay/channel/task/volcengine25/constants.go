package volcengine25

// ChannelName 是渠道在日志/管理端展示的名字，与 constant.ChannelTypeNames 中的
// "VolcEngine25" 对应。
const ChannelName = "volcengine-25"

// 输出分辨率档。样片（draft）只允许 480p，成片（finalize）固定 1080p——
// 这两个值同时也是计费档位价格表（video_tier_pricing.price_table）的键。
const (
	Res480p  = "480p"
	Res1080p = "1080p"
)

// ModelList 是该渠道默认支持的模型。仅 Seedance 2.5——更早的 Seedance 版本
// 没有样片模式，且它们的降级/超分诉求由 64（VolcEngineEnhanced）承担。
var ModelList = []string{
	"doubao-seedance-2-5-260628",
}

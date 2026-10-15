package tokenhub25

// ChannelName 是渠道在日志/管理端展示的名字，与 constant.ChannelTypeNames 中的
// "TokenHub25" 对应。
const ChannelName = "tokenhub-25"

// 输出分辨率档。样片（draft）只允许 480p，成片（finalize）固定 1080p——
// 这两个值同时也是计费档位价格表（video_tier_pricing.price_table）的键。
const (
	Res480p  = "480p"
	Res1080p = "1080p"
)

// OmniReferenceTaskTypeKey 是客户端 metadata 里显式引导任务类型的键，取值
// auto / reference / edit / extend。全模态参考生视频由模型按素材与提示词判定
// 子类型，显式指定可把特殊参数限制的校验前置到提交时。
//
// 它只对 Seedance 2.5 有意义，因此不进 tokenhub 的共用 requestPayload 白名单，
// 由本渠道在 BuildRequestBody 里补写，保证 61 的 payload 逐位不变。
const OmniReferenceTaskTypeKey = "omni_reference_task_type"

// ModelList 是该渠道默认支持的模型。仅 Seedance 2.5——更早的 Seedance 版本
// 没有样片模式，它们的 2.0 三条模型（dreamina-seedance-2-0-*）继续由
// 61（TokenHub）承载。
var ModelList = []string{
	"dreamina-seedance-2-5-260628",
}

package tokenhubvolc

// ChannelName 是渠道在日志/管理端展示的名字，与 constant.ChannelTypeNames 中的
// "TokenHubVolc" 对应。
const ChannelName = "tokenhub-volc"

// 输出分辨率档。样片（draft）只能 480p，成片（finalize）固定 1080p——这两个值
// 同时是 video_tier_pricing.price_table 的键。
const (
	Res480p  = "480p"
	Res1080p = "1080p"
)

// ModelList 是该渠道的默认模型名（对客）。实际可用模型由渠道 models 配置与
// model_mapping 决定，这里只是管理端「填充模型」的默认值。
var ModelList = []string{
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
	"doubao-seedance-2-0-mini-260615",
	"doubao-seedance-2-5-260628",
}

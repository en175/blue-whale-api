package billing_setting

import (
	"github.com/QuantumNous/new-api/setting/config"
)

// VideoTierPrice 描述单个视频模型在 (输出分辨率档 × 是否含视频输入) 两个维度下的
// 单价（与 ModelRatio 同口径，如元/百万 token）。
//
// Base 为基准价（480p/720p 且不含视频输入），应等于管理员为该模型配置的
// ModelRatio；其余档位为 0 或负值时视为**未配置**（不是免费档），计费回退
// Base——当前不支持表达"某档免费"。Base 为 0/负值时整条配置无效，回退内置表。
// 运行时计费倍率 = 档位价 / Base。
type VideoTierPrice struct {
	Base                 float64 `json:"base"`                  // 480p/720p 不含视频输入（基准价，必填）
	Video                float64 `json:"video,omitempty"`       // 480p/720p 含视频输入
	Resolution1080p      float64 `json:"1080p,omitempty"`       // 1080p 不含视频输入
	Resolution1080pVideo float64 `json:"1080p_video,omitempty"` // 1080p 含视频输入
	Resolution4k         float64 `json:"4k,omitempty"`          // 4k 不含视频输入
	Resolution4kVideo    float64 `json:"4k_video,omitempty"`    // 4k 含视频输入
}

// VideoTierPricing 由 config.GlobalConfig.Register 管理。
// DB 键：video_tier_pricing.price_table，值为 {模型名: VideoTierPrice} 的 JSON。
type VideoTierPricing struct {
	PriceTable map[string]VideoTierPrice `json:"price_table"`
}

var videoTierPricing = VideoTierPricing{
	PriceTable: make(map[string]VideoTierPrice),
}

func init() {
	config.GlobalConfig.Register("video_tier_pricing", &videoTierPricing)
}

// GetVideoTierPrice 返回指定模型的档位价格表；第二个返回值表示是否已配置。
func GetVideoTierPrice(model string) (VideoTierPrice, bool) {
	price, ok := videoTierPricing.PriceTable[model]
	return price, ok
}

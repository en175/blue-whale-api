package taskcommon

import (
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// VideoPriceKey 视频档位价格表的键：输出分辨率档（Is1080p/Is4k 均为 false 即
// 480p/720p 基准档）、输入是否含视频。
type VideoPriceKey struct {
	Is1080p  bool
	Is4k     bool
	HasVideo bool
}

// builtinVideoPriceTable 豆包系模型的内置价格表（直连渠道历史配置），
// 各模型在不同 (输出分辨率档, 是否含视频输入) 下的单价（元/百万 token）。
// 零值键 {480p/720p, 不含视频} 为基准价，等于管理员应配置的 ModelRatio；
// 计费时取 实际单价/基准价 作为 OtherRatio。
// 管理员可通过 video_tier_pricing.price_table 配置覆盖或新增任意模型。
var builtinVideoPriceTable = map[string]map[VideoPriceKey]float64{
	"doubao-seedance-2-0-260128": {
		{HasVideo: false}:                46.0,
		{HasVideo: true}:                 28.0,
		{Is1080p: true, HasVideo: false}: 51.0,
		{Is1080p: true, HasVideo: true}:  31.0,
		{Is4k: true, HasVideo: false}:    26.0,
		{Is4k: true, HasVideo: true}:     16.0,
	},
	"doubao-seedance-2-0-fast-260128": {
		{HasVideo: false}: 37.0,
		{HasVideo: true}:  22.0,
	},
}

// HasVideoInputInMetadata 检查 metadata 的 content 数组是否包含 video_url 条目
// （new-api 系任务 API 契约：content 数组项 type=="video_url" 或带 video_url 键），
// 用于判定请求是否含视频输入。顶层 image/images（首帧 i2v）不算视频输入。
func HasVideoInputInMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	contentRaw, ok := metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

// ResolveVideoInputRatio 返回指定模型在给定输出分辨率/是否含视频输入下，
// 相对基准价的计费倍率；第二个返回值表示该模型是否有价格表（配置或内置）。
// 查找优先级：管理员配置 video_tier_pricing.price_table > 内置价格表。
// 倍率为 1.0 时调用方可忽略该 OtherRatio。
func ResolveVideoInputRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	if prices, ok := configuredVideoPriceTable(modelName); ok {
		return videoTierRatio(prices, resolution, hasVideo)
	}
	prices, ok := builtinVideoPriceTable[modelName]
	if !ok {
		return 0, false
	}
	return videoTierRatio(prices, resolution, hasVideo)
}

// EstimateVideoTierBilling 根据请求 metadata 的输出分辨率与是否含视频输入，
// 返回档位价相对基准价的计费 OtherRatio；无价格表或倍率为 1.0 时返回 nil。
// 价格表按上游模型名（实际发给上游的模型名）查找，回退对客名。
// newapi 与 doubao 适配器共用此实现（入参契约同源：metadata.content 的
// video_url 条目 + metadata.resolution）。
func EstimateVideoTierBilling(info *relaycommon.RelayInfo, req *relaycommon.TaskSubmitReq) map[string]float64 {
	hasVideo := HasVideoInputInMetadata(req.Metadata)
	resolution, _ := req.Metadata["resolution"].(string)
	ratio, ok := ResolveVideoInputRatio(info.UpstreamModelName, resolution, hasVideo)
	if !ok {
		ratio, ok = ResolveVideoInputRatio(info.OriginModelName, resolution, hasVideo)
	}
	if !ok || ratio == 1.0 {
		return nil
	}
	return map[string]float64{"video_input": ratio}
}

// configuredVideoPriceTable 把管理员配置的档位价格转换为统一价格表。
// Base 缺失或非正的配置视为无效（基准价是倍率计算的分母）。
func configuredVideoPriceTable(modelName string) (map[VideoPriceKey]float64, bool) {
	tier, ok := billing_setting.GetVideoTierPrice(modelName)
	if !ok || tier.Base <= 0 {
		return nil, false
	}
	prices := map[VideoPriceKey]float64{{}: tier.Base}
	add := func(key VideoPriceKey, price float64) {
		if price > 0 {
			prices[key] = price
		}
	}
	add(VideoPriceKey{HasVideo: true}, tier.Video)
	add(VideoPriceKey{Is1080p: true}, tier.Resolution1080p)
	add(VideoPriceKey{Is1080p: true, HasVideo: true}, tier.Resolution1080pVideo)
	add(VideoPriceKey{Is4k: true}, tier.Resolution4k)
	add(VideoPriceKey{Is4k: true, HasVideo: true}, tier.Resolution4kVideo)
	return prices, true
}

// videoTierRatio 在价格表内按 (分辨率档, 是否含视频输入) 查实际单价并折算为
// 相对基准价的倍率。未配置的组合（如 fast 无 1080p/4k，上游会自行报错）按基准价计费。
func videoTierRatio(prices map[VideoPriceKey]float64, resolution string, hasVideo bool) (float64, bool) {
	base := prices[VideoPriceKey{}]
	if base <= 0 {
		return 0, false
	}
	res := strings.ToLower(strings.TrimSpace(resolution))
	price, ok := prices[VideoPriceKey{Is1080p: res == "1080p", Is4k: res == "4k", HasVideo: hasVideo}]
	if !ok {
		return 1.0, true
	}
	return price / base, true
}

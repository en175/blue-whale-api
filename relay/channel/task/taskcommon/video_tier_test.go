package taskcommon

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetVideoTierConfig 恢复 video_tier_pricing 为空表，防止配置在用例间泄漏。
func resetVideoTierConfig(t *testing.T) {
	t.Helper()
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"video_tier_pricing.price_table": "{}",
	}))
	t.Cleanup(func() {
		_ = config.GlobalConfig.LoadFromDB(map[string]string{
			"video_tier_pricing.price_table": "{}",
		})
	})
}

// TestResolveVideoInputRatio_BuiltinTable 内置豆包价格表的档位倍率：
// 相对基准价（480p/720p 不含视频输入）折算。
func TestResolveVideoInputRatio_BuiltinTable(t *testing.T) {
	const model = "doubao-seedance-2-0-260128"
	tests := []struct {
		name       string
		resolution string
		hasVideo   bool
		wantRatio  float64
	}{
		{"基准档不含视频_倍率1", "720p", false, 1.0},
		{"基准档含视频", "720p", true, 28.0 / 46.0},
		{"1080p不含视频", "1080p", false, 51.0 / 46.0},
		{"1080p含视频", "1080P", true, 31.0 / 46.0}, // 大小写不敏感
		{"4k不含视频", "4k", false, 26.0 / 46.0},
		{"4k含视频", " 4k ", true, 16.0 / 46.0}, // 两侧空白被修剪
		{"空分辨率按基准档", "", false, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio, ok := ResolveVideoInputRatio(model, tt.resolution, tt.hasVideo)
			require.True(t, ok)
			assert.InDelta(t, tt.wantRatio, ratio, 1e-9)
		})
	}
}

// TestResolveVideoInputRatio_MissingCombo 未配置的组合（fast 无 1080p/4k）按基准价。
func TestResolveVideoInputRatio_MissingCombo(t *testing.T) {
	ratio, ok := ResolveVideoInputRatio("doubao-seedance-2-0-fast-260128", "1080p", false)
	require.True(t, ok)
	assert.Equal(t, 1.0, ratio)
}

// TestResolveVideoInputRatio_UnknownModel 无价格表（内置与配置均无）时明确返回 false。
func TestResolveVideoInputRatio_UnknownModel(t *testing.T) {
	ratio, ok := ResolveVideoInputRatio("no-such-model", "1080p", true)
	assert.False(t, ok)
	assert.Equal(t, 0.0, ratio)
}

// TestResolveVideoInputRatio_ConfigOverridesBuiltin 管理员配置优先于内置表，
// 且可为内置表没有的任意模型新增价格。
func TestResolveVideoInputRatio_ConfigOverridesBuiltin(t *testing.T) {
	resetVideoTierConfig(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"video_tier_pricing.price_table": `{
			"doubao-seedance-2-0-260128": {"base": 40, "video": 20, "1080p": 60},
			"my-custom-seedance": {"base": 46, "video": 28}
		}`,
	}))

	// 覆盖内置：base 40、video 20 → 倍率 0.5（内置是 28/46）
	ratio, ok := ResolveVideoInputRatio("doubao-seedance-2-0-260128", "720p", true)
	require.True(t, ok)
	assert.InDelta(t, 0.5, ratio, 1e-9)

	// 新增模型：内置表没有，配置生效
	ratio, ok = ResolveVideoInputRatio("my-custom-seedance", "720p", true)
	require.True(t, ok)
	assert.InDelta(t, 28.0/46.0, ratio, 1e-9)

	// 新增模型未配置的档位回退 base（倍率 1）
	ratio, ok = ResolveVideoInputRatio("my-custom-seedance", "1080p", false)
	require.True(t, ok)
	assert.Equal(t, 1.0, ratio)
}

// TestResolveVideoInputRatio_ConfigMissingBase 配置缺 base（或 base 非正）视为无效，
// 回退内置表；内置也没有则无价格表。
func TestResolveVideoInputRatio_ConfigMissingBase(t *testing.T) {
	resetVideoTierConfig(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"video_tier_pricing.price_table": `{
			"doubao-seedance-2-0-260128": {"video": 28},
			"base-zero-model": {"base": 0, "video": 28},
			"some-model": {"video": 28}
		}`,
	}))

	ratio, ok := ResolveVideoInputRatio("doubao-seedance-2-0-260128", "720p", true)
	require.True(t, ok, "base 缺失应回退内置表")
	assert.InDelta(t, 28.0/46.0, ratio, 1e-9)

	_, ok = ResolveVideoInputRatio("base-zero-model", "720p", true)
	assert.False(t, ok, "base 为 0 应视为配置无效，回退内置表（无内置则无价格表）")

	_, ok = ResolveVideoInputRatio("some-model", "720p", true)
	assert.False(t, ok, "无内置表且配置无效时应无价格表")
}

// TestVideoTierPriceConfigured 区分「档位有显式单价」与「漏配/无价格表」。
// 告警依据是前者：档位价恰好等于基准价（倍率 1.0）是合法配置，不能报漏配。
func TestVideoTierPriceConfigured(t *testing.T) {
	resetVideoTierConfig(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"video_tier_pricing.price_table": `{
			"doubao-seedance-2-5-260628": {"base": 46, "1080p": 46, "1080p_video": 31},
			"my-custom-seedance": {"base": 46}
		}`,
	}))

	const builtinModel = "doubao-seedance-2-0-260128"
	tests := []struct {
		name       string
		model      string
		resolution string
		hasVideo   bool
		want       bool
	}{
		{"内置表 1080p 已配", builtinModel, "1080p", false, true},
		{"内置表 4k 已配", builtinModel, "4K", true, true},
		{"内置表基准档已配", builtinModel, "720p", false, true},
		{"内置 fast 无 1080p", "doubao-seedance-2-0-fast-260128", "1080p", false, false},
		{"配置表 1080p 有价", "doubao-seedance-2-5-260628", "1080p", false, true},
		{"配置表 1080p 含视频有价", "doubao-seedance-2-5-260628", " 1080P ", true, true},
		{"配置表仅 base 无 4k", "my-custom-seedance", "4k", false, false},
		{"无价格表", "no-such-model", "1080p", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, VideoTierPriceConfigured(tt.model, tt.resolution, tt.hasVideo))
		})
	}
}

func TestHasVideoInputInMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     bool
	}{
		{"nil", nil, false},
		{"无content", map[string]interface{}{"resolution": "1080p"}, false},
		{"content非数组", map[string]interface{}{"content": "video_url"}, false},
		{"type为video_url", map[string]interface{}{
			"content": []interface{}{map[string]interface{}{"type": "video_url", "video_url": map[string]interface{}{"url": "https://v.mp4"}}},
		}, true},
		{"带video_url键", map[string]interface{}{
			"content": []interface{}{map[string]interface{}{"video_url": map[string]interface{}{"url": "https://v.mp4"}}},
		}, true},
		{"仅image_url不算视频输入", map[string]interface{}{
			"content": []interface{}{map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "https://i.png"}}},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasVideoInputInMetadata(tt.metadata))
		})
	}
}

// TestEstimateVideoTierBillingForTier 显式档位入口（Seedance 2.5 成片用）：
// hasVideo 不来自当次请求体，而是由调用方喂进来，因此必须确认同一个 1080p 档在
// 两种 hasVideo 下得到不同倍率——这是「继承样片输入维度」的全部意义。
// 同时覆盖两种返回 nil 的情形：未配置的组合（倍率 1）与完全没有价格表。
func TestEstimateVideoTierBillingForTier(t *testing.T) {
	resetVideoTierConfig(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"video_tier_pricing.price_table": `{
			"doubao-seedance-2-5-260628": {"base": 46, "video": 28, "1080p": 55, "1080p_video": 30}
		}`,
	}))

	const model = "doubao-seedance-2-5-260628"
	info := &relaycommon.RelayInfo{OriginModelName: model, ChannelMeta: &relaycommon.ChannelMeta{}}

	withVideo := EstimateVideoTierBillingForTier(info, "1080p", true)
	withoutVideo := EstimateVideoTierBillingForTier(info, "1080p", false)
	require.NotNil(t, withVideo)
	require.NotNil(t, withoutVideo)
	assert.InDelta(t, 30.0/46.0, withVideo["video_input"], 1e-9)
	assert.InDelta(t, 55.0/46.0, withoutVideo["video_input"], 1e-9)
	assert.NotEqual(t, withVideo["video_input"], withoutVideo["video_input"],
		"1080p 档含/不含视频输入的倍率必须不同")

	// 未配置的档位回退基准价 → 倍率 1 → 不产生 OtherRatio
	assert.Nil(t, EstimateVideoTierBillingForTier(info, "4k", false))
	// 没有价格表的模型同样返回 nil，而不是悄悄按 1 计费
	assert.Nil(t, EstimateVideoTierBillingForTier(
		&relaycommon.RelayInfo{OriginModelName: "no-such-model", ChannelMeta: &relaycommon.ChannelMeta{}}, "1080p", true))
}

// TestEstimateVideoTierBilling_Regression 回归：拆分出显式档位入口后，
// 原来的「从请求体读档位」入口行为不变（doubao / newapi 适配器仍在用）。
func TestEstimateVideoTierBilling_Regression(t *testing.T) {
	resetVideoTierConfig(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"video_tier_pricing.price_table": `{
			"doubao-seedance-2-5-260628": {"base": 46, "video": 28, "1080p": 55, "1080p_video": 30}
		}`,
	}))
	info := &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-5-260628", ChannelMeta: &relaycommon.ChannelMeta{}}

	// 请求体里带 video_url 的 content → hasVideo 为真，取 1080p_video 档
	ratios := EstimateVideoTierBilling(info, &relaycommon.TaskSubmitReq{
		Metadata: map[string]interface{}{
			"resolution": "1080p",
			"content": []interface{}{
				map[string]interface{}{"type": "video_url", "video_url": map[string]interface{}{"url": "https://v.mp4"}},
			},
		},
	})
	require.NotNil(t, ratios)
	assert.InDelta(t, 30.0/46.0, ratios["video_input"], 1e-9)

	// 不含视频输入 → 取 1080p 档，与上面必须不同
	ratios = EstimateVideoTierBilling(info, &relaycommon.TaskSubmitReq{
		Metadata: map[string]interface{}{"resolution": "1080p"},
	})
	require.NotNil(t, ratios)
	assert.InDelta(t, 55.0/46.0, ratios["video_input"], 1e-9)
}

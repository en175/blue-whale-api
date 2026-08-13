package taskcommon

import (
	"testing"

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

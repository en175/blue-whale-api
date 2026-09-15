package mediakit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryPathsAreConsistent 保护注册表的核心契约：每个工具的 Path 必须由
// Sync 标志唯一决定。Path 是唯一被拼进上游 URL 的字符串，写错会把请求打到错误
// 的端点（或形成路径穿越面），而门面上看不出任何异常。
func TestRegistryPathsAreConsistent(t *testing.T) {
	for _, spec := range toolRegistry {
		t.Run(spec.Name, func(t *testing.T) {
			require.NotEmpty(t, spec.Name)
			assert.False(t, strings.ContainsAny(spec.Name, "/. "), "tool name must be a bare slug")

			if spec.Sync {
				assert.Equal(t, syncPathPrefix+spec.Name, spec.Path)
			} else {
				assert.Equal(t, taskPathPrefix+spec.Name, spec.Path)
			}
		})
	}
}

// TestRegistryNamesAreUnique 防止注册表里出现重名工具：重名会让 toolIndex 静默
// 丢弃其中一个，而 GetModelList 仍返回两条同名模型。
func TestRegistryNamesAreUnique(t *testing.T) {
	seen := make(map[string]struct{}, len(toolRegistry))
	for _, spec := range toolRegistry {
		_, dup := seen[spec.Name]
		require.Falsef(t, dup, "duplicate tool name: %s", spec.Name)
		seen[spec.Name] = struct{}{}
	}
	assert.Len(t, toolIndex, len(toolRegistry))
}

// TestSyncToolsUseToolsSyncPrefix 固定「同步工具全部是图像工具」这一分类事实：
// 官方把图像工具放在 /tools-sync/，音视频工具放在 /tools/。分类填反的表现是
// 客户端拿到上游 task_id 却没有查询入口。
func TestSyncToolsUseToolsSyncPrefix(t *testing.T) {
	for _, spec := range toolRegistry {
		if spec.Sync {
			assert.True(t, strings.HasPrefix(spec.Path, "/api/v1/tools-sync/"), "sync tool %s must use tools-sync prefix", spec.Name)
			assert.NotEqual(t, BillingDuration, spec.Mode, "sync tool %s cannot bill by duration", spec.Name)
			continue
		}
		assert.True(t, strings.HasPrefix(spec.Path, "/api/v1/tools/"), "async tool %s must use tools prefix", spec.Name)
		assert.NotEqual(t, BillingVolume, spec.Mode, "async tool %s cannot bill by image volume", spec.Name)
	}
}

func TestLookupToolRejectsUnknownName(t *testing.T) {
	_, ok := LookupTool("../../etc/passwd")
	assert.False(t, ok)
	_, ok = LookupTool("not-a-real-tool")
	assert.False(t, ok)
}

// TestToolModelNameRoundTrip 保护对客模型名与工具名的双向映射。分销层拿模型名
// 查渠道，适配器再反查工具，两处不一致会导致渠道路由到却拼不出路径。
func TestToolModelNameRoundTrip(t *testing.T) {
	for _, spec := range toolRegistry {
		model := ModelNameForTool(spec.Name)
		assert.True(t, strings.HasPrefix(model, ModelPrefix))

		got, ok := ToolForModel(model)
		require.Truef(t, ok, "round trip failed for %s", spec.Name)
		assert.Equal(t, spec.Name, got.Name)
	}

	_, ok := ToolForModel("gpt-image-2-text-to-image")
	assert.False(t, ok, "non-mediakit model must not resolve")
}

// TestResultURL 覆盖结果键的不确定性：不同工具的结果键名不同，且官方未给出
// 统一 schema，故按已知键优先、再兜底扫 *_url。
func TestResultURL(t *testing.T) {
	tests := []struct {
		name   string
		result map[string]any
		want   string
	}{
		{"video", map[string]any{"video_url": "https://x/v.mp4"}, "https://x/v.mp4"},
		{"image", map[string]any{"image_url": "https://x/i.png"}, "https://x/i.png"},
		{"audio", map[string]any{"audio_url": "https://x/a.mp3"}, "https://x/a.mp3"},
		{"video wins over image", map[string]any{"image_url": "https://x/i.png", "video_url": "https://x/v.mp4"}, "https://x/v.mp4"},
		{"fallback suffix", map[string]any{"result_url": "https://x/r.mp4"}, "https://x/r.mp4"},
		{"empty value ignored", map[string]any{"video_url": ""}, ""},
		{"non-string ignored", map[string]any{"video_url": 42}, ""},
		{"no url", map[string]any{"duration": 1.5}, ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResultURL(tt.result))
		})
	}
}

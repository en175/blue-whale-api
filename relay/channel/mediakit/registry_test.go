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

// TestRegistryCoversOfficialCatalog 把条目数钉死在目录口径上：官方那三个集合
// （视频 33 / 图像 27 / 音频 4）共 64 条已全部登记，不存在「官方有、网关没接」的
// 遗留；在此之上另接入 5 条集合外的工具（extract-audio 供下游「音频提取」、
// trim-audio 供下游「音频截取」、mix-audio / mux-audio-video / adjust-audio-speed
// 供下游音视频编辑），合计 69。数字对不上就是漏了工具或凭空多了一条。
// Mode 的分项计数同时兜住计费归类，它是审计行 billing_mode 与文档计费口径的唯一来源。
func TestRegistryCoversOfficialCatalog(t *testing.T) {
	syncCount, asyncCount := 0, 0
	byMode := map[BillingMode]int{}
	for _, spec := range toolRegistry {
		if spec.Sync {
			syncCount++
		} else {
			asyncCount++
		}
		byMode[spec.Mode]++
	}

	assert.Equal(t, 69, len(toolRegistry), "69 = 官方三个集合 64 + 集合外 5（extract-audio、trim-audio、mix-audio、mux-audio-video、adjust-audio-speed）")
	assert.Equal(t, 27, syncCount, "同步 27 个，全部是图像工具")
	assert.Equal(t, 42, asyncCount, "异步 42 个 = 视频 34 + 音频 8")
	assert.Equal(t, 39, byMode[BillingDuration], "按时长：视频 32 + 音频 7（含集合外的 5 个）")
	assert.Equal(t, 17, byMode[BillingPerCall], "按次：图像 14 + 视频 2 + 音频 1")
	assert.Equal(t, 13, byMode[BillingVolume], "按量 GiB：图像基础编辑族 13 个")
	assert.Equal(t, len(toolRegistry), byMode[BillingDuration]+byMode[BillingPerCall]+byMode[BillingVolume],
		"每个工具必须落进一个已知计费档")
}

// TestAnalysisToolsDeclareNoProduct 锁定「只出分析结果、不产出文件」的工具必须声明
// ResultNone。ResultKind 是「该不该有产物」的唯一判据：声明成 ResultVideo 而实际
// 拿不到 URL，调用方会把「无产物」误判成网关取件失败。
// 实测抓到的反例是 analyze-video-highlights —— 名字带「提取」，实际只回
// highlight_info（片段起止时间），成品由 generate-highlights-* 族产出。
func TestAnalysisToolsDeclareNoProduct(t *testing.T) {
	analysisOnly := []string{
		"analyze-video-highlights",
		"analyze-video-storyline",
		"asr-subtitles",
		"assess-video-quality",
		"detect-voice-activity",
		"video-ocr",
		"image-ocr",
		"evaluate-image-quality",
		"extract-video-invisible-watermark",
		"extract-image-invisible-watermark",
		"probe-video-metadata",
		"probe-image-metadata",
		"probe-audio-metadata",
	}
	for _, name := range analysisOnly {
		t.Run(name, func(t *testing.T) {
			spec, ok := LookupTool(name)
			require.Truef(t, ok, "tool %s must be registered", name)
			assert.Equal(t, ResultNone, spec.ResultKind,
				"%s returns analysis only; declaring a product kind makes the gateway's result URL lookups fail", name)
			assert.Empty(t, spec.ResultPath, "%s has no artifact to point at", name)
		})
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

// TestResultURL 覆盖取值机制：位置由 ResultPath 或 ResultKind 决定，取不到即空串。
func TestResultURL(t *testing.T) {
	tests := []struct {
		name   string
		spec   ToolSpec
		result map[string]any
		want   string
	}{
		{"约定键 video_url", ToolSpec{ResultKind: ResultVideo}, map[string]any{"video_url": "https://x/v.mp4"}, "https://x/v.mp4"},
		{"约定键 image_url", ToolSpec{ResultKind: ResultImage}, map[string]any{"image_url": "https://x/i.png"}, "https://x/i.png"},
		{"约定键 audio_url", ToolSpec{ResultKind: ResultAudio}, map[string]any{"audio_url": "https://x/a.mp3"}, "https://x/a.mp3"},
		{"无产物类型一律取不到", ToolSpec{ResultKind: ResultNone}, map[string]any{"video_url": "https://x/v.mp4"}, ""},
		{"空值忽略", ToolSpec{ResultKind: ResultVideo}, map[string]any{"video_url": ""}, ""},
		{"非字符串忽略", ToolSpec{ResultKind: ResultVideo}, map[string]any{"video_url": 42}, ""},
		{"nil result", ToolSpec{ResultKind: ResultVideo}, nil, ""},
		{"路径未命中", ToolSpec{ResultPath: "video_urls.0"}, map[string]any{"video_urls": []any{}}, ""},
		{"路径优先于类型", ToolSpec{ResultKind: ResultVideo, ResultPath: "other_url"}, map[string]any{"video_url": "https://x/v.mp4", "other_url": "https://x/o.mp4"}, "https://x/o.mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResultURL(tt.result, tt.spec))
		})
	}
}

// TestResultURLRealToolShapes 用逐工具实测抓到的真实 result 形状锁住注册表声明。
// 上游各工具 result 结构不统一，且「取不到」与「取到输入的 URL」是两种不同的错，
// 这里都要防。
func TestResultURLRealToolShapes(t *testing.T) {
	tests := []struct {
		tool   string
		result map[string]any
		want   string
	}{
		{"enhance-video", map[string]any{"duration": 14.1, "video_url": "https://x/v.mp4"}, "https://x/v.mp4"},
		{"drama-recap", map[string]any{"duration": 37.928, "video_url": "https://x/recap.mp4"}, "https://x/recap.mp4"},
		{"transcode-audio", map[string]any{"audio_url": "https://x/a.mp3"}, "https://x/a.mp3"},
		// 音频截取：产物键名与 extract-audio 同形（顶层 audio_url），duration 是裁剪后
		// 的时长而不是原片时长，实测回包 task_type=trim-audio、duration=3。
		{"trim-audio", map[string]any{"audio_url": "https://x/clip.m4a", "duration": 3}, "https://x/clip.m4a"},
		// 成品在顶层数组里，没有单数 video_url 键。
		{"drama-recap-vertical", map[string]any{"mode": "Text", "video_urls": []any{"https://x/v.mp4"}, "input_duration": 246.63, "output_duration": 43.5}, "https://x/v.mp4"},
		// 人声与伴奏并列，必须取人声轨——旧实现按字典序会取到 background_audio_url。
		{"separate-voice", map[string]any{"voice_audio_url": "https://x/voice.mp3", "background_audio_url": "https://x/bgm.mp3"}, "https://x/voice.mp3"},
		// 多张图，url 取第一张，全量由 result.snapshots 回传。
		{"extract-frames", map[string]any{"snapshots": []any{map[string]any{"image_url": "https://x/0.jpg"}, map[string]any{"image_url": "https://x/1.jpg"}}, "snapshot_count": 2}, "https://x/0.jpg"},
		{"segment-scenes", map[string]any{"segments": []any{map[string]any{"segment_video_url": "https://x/s0.mp4"}}}, "https://x/s0.mp4"},
		// 键名不在约定之列，靠 ResultPath 声明。
		{"semantic-segment", map[string]any{"result_url": "https://x/r.mp4"}, "https://x/r.mp4"},
		{"drama-script", map[string]any{"duration": 246.656, "result_url": "https://x/d.json.gz"}, "https://x/d.json.gz"},
		// 竖版剧本还原的 result_url 指向 manifest.json 清单（再索引各产物文件），
		// 不是可播视频，故与 drama-script 同形声明 ResultNone + ResultPath。
		{"drama-script-vertical", map[string]any{"duration": 245.5, "input_duration": 245.5, "result_url": "https://x/manifest.json"}, "https://x/manifest.json"},
		{"erase-video-subtitle", map[string]any{"duration": 12.5, "video_url": "https://x/erased.mp4"}, "https://x/erased.mp4"},

		// 元信息探测只回数据段，没有 URL 可取——网关不编造 url 字段。
		{"probe-video-metadata", map[string]any{"format_meta": map[string]any{"format": "mp4"}, "video_stream_meta": map[string]any{"width": 1920}, "audio_stream_meta": map[string]any{"channels": 2}}, ""},
		{"probe-image-metadata", map[string]any{"format_meta": map[string]any{"format": "jpeg"}, "width": 1920, "height": 1080}, ""},
		{"probe-audio-metadata", map[string]any{"format_meta": map[string]any{"format": "mp3"}, "audio_stream_meta": map[string]any{"sample_rate": 44100}}, ""},

		// 「上游 success 但没有产物」——必须返回空串，调用方不得据此编造网关 URL。
		{"asr-subtitles", map[string]any{"duration": 14.12, "subtitles": []any{}}, ""},
		{"assess-video-quality", map[string]any{"duration": 14.095, "vq_score": 61.88}, ""},
		{"video-ocr", map[string]any{"duration": 14.095, "ocr_results": []any{}}, ""},
		{"analyze-video-highlights", map[string]any{"duration": 14.095, "highlight_info": []any{map[string]any{"start_time": 0, "end_time": 13.9, "score": 2}}}, ""},
		{"generate-highlights-minigame", map[string]any{"duration": 14.095, "video_urls": []any{}, "mixvideo_info": []any{map[string]any{"clips": []any{}}}}, ""},
		// 同族的非空结果：高光剪辑族的成品都在顶层 video_urls 数组里。
		{"generate-highlights-movie", map[string]any{"duration": 159.1, "video_urls": []any{"https://x/hl.mp4"}}, "https://x/hl.mp4"},
		// source_video_url 是输入素材，不是产物——不能当成结果 URL 交给客户端。
		{"analyze-video-storyline", map[string]any{"duration": 14.04, "storyline_clips": []any{}, "source_video_info": []any{map[string]any{"source_video_url": "https://input/demo.mp4"}}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			spec, ok := LookupTool(tt.tool)
			require.Truef(t, ok, "tool %s must be registered", tt.tool)
			assert.Equal(t, tt.want, ResultURL(tt.result, spec))
		})
	}
}

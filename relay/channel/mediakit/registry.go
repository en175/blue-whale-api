package mediakit

import (
	"sort"
	"strings"
)

// BillingMode 决定对客计费口径。官方计费分三档（按成功次数 / 按时长 / 按处理量 GiB），
// 对客按同一维度归口，详见 docs/channel/mediakit.md。
type BillingMode string

const (
	// BillingDuration 按输出/输入时长计费（元/分钟），提交预扣、完成结算差额。
	BillingDuration BillingMode = "duration"
	// BillingPerCall 按成功处理次数计费，每次一个固定价。
	BillingPerCall BillingMode = "per_call"
	// BillingVolume 按处理量计费（元/GiB）。官方是自然月阶梯，对客取超出档单价直算。
	BillingVolume BillingMode = "volume"
)

// ResultKind 声明工具产物类型，用于从上游 result 对象里取结果 URL。
type ResultKind string

const (
	ResultVideo ResultKind = "video"
	ResultImage ResultKind = "image"
	ResultAudio ResultKind = "audio"
	ResultNone  ResultKind = "none"
)

// ModelPrefix 是对客模型名前缀。加前缀避免与任何上游真实模型 id 冲突，
// 也让管理员能在渠道模型列表里一眼认出 MediaKit 工具。
const ModelPrefix = "mediakit-"

// ToolSpec 描述一个 MediaKit 工具。
//
// Path 是写死的完整上游路径，绝不接受客户端输入——客户端传的 tool 名只用于
// LookupTool 查表，避免字符串拼接带来的路径穿越。
type ToolSpec struct {
	Name       string      // 上游 slug，对客模型名为 ModelPrefix + Name
	Path       string      // 完整上游路径
	Sync       bool        // true=同步工具（/tools-sync/），false=异步任务（/tools/）
	Mode       BillingMode // 计费口径
	ResultKind ResultKind  // 产物类型
}

// toolRegistry 是全部已接入的工具。上游路径来自官方文档逐条核对，
// 不在表内的 tool 名一律 400，不会被转发。
var toolRegistry = []ToolSpec{
	// ── 同步：图像工具（/api/v1/tools-sync/*）────────────────────────
	{Name: "enhance-image", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "remove-image-background", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "image-ocr", Sync: true, Mode: BillingPerCall, ResultKind: ResultNone},
	{Name: "evaluate-image-quality", Sync: true, Mode: BillingPerCall, ResultKind: ResultNone},
	{Name: "erase-image", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "expand-image-canvas", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "smart-crop-image", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "slim-image", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "remove-image-elements", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "translate-image-text", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "face-blur-image", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "generate-product-scene-image", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "add-image-invisible-watermark", Sync: true, Mode: BillingPerCall, ResultKind: ResultImage},
	{Name: "extract-image-invisible-watermark", Sync: true, Mode: BillingPerCall, ResultKind: ResultNone},

	// ── 同步：图像基础编辑（官方按处理量 GiB 计费）──────────────────
	{Name: "round-corner-image", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	{Name: "flip-image", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	{Name: "rotate-image", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	{Name: "sharpen-image", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	{Name: "gaussian-blur-image", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	{Name: "mosaic-image", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	{Name: "add-image-watermark", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	{Name: "adjust-image-color", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	// 图像缩放的上游 slug 是 resize-image，不是文档里写的 scale-image——
	// 后者上游返回 NotFound，整条工具在网关上永远 400（实测发现）。
	{Name: "resize-image", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	{Name: "invert-image", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	{Name: "crop-image", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},
	{Name: "compress-image", Sync: true, Mode: BillingVolume, ResultKind: ResultImage},

	// ── 异步：视频工具（/api/v1/tools/*）────────────────────────────
	{Name: "enhance-video-generative", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "enhance-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "enhance-video-fast", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "erase-video-subtitle-pro", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "segment-scenes", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "asr-subtitles", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "video-ocr", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "analyze-video-highlights", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "analyze-video-storyline", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "generate-highlights-microdrama", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "generate-highlights-minigame", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "generate-highlights-movie", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "matte-greenscreen-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "matte-portrait-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "drama-script", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "drama-recap", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "drama-recap-vertical", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "add-video-invisible-watermark", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "extract-video-invisible-watermark", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "assess-video-quality", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "transcode-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "remux-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "martencode-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "face-blur-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "face-swap-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "semantic-segment", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "video-frame-interpolation", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "lip-sync", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "reframe-video-to-vertical", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "extract-frames", Mode: BillingPerCall, ResultKind: ResultImage},

	// ── 异步：音频工具 ──────────────────────────────────────────────
	{Name: "separate-voice", Mode: BillingDuration, ResultKind: ResultAudio},
	{Name: "transcode-audio", Mode: BillingDuration, ResultKind: ResultAudio},
	{Name: "detect-voice-activity", Mode: BillingDuration, ResultKind: ResultNone},
}

// toolIndex 由 toolRegistry 构建，键为上游 slug。在 init 中填充，保证
// toolRegistry 与 toolIndex 的 Path 始终一致。
var toolIndex map[string]ToolSpec

func init() {
	for i := range toolRegistry {
		toolRegistry[i].Path = pathFor(toolRegistry[i])
	}
	toolIndex = make(map[string]ToolSpec, len(toolRegistry))
	for _, spec := range toolRegistry {
		toolIndex[spec.Name] = spec
	}
}

func pathFor(spec ToolSpec) string {
	if spec.Sync {
		return syncPathPrefix + spec.Name
	}
	return taskPathPrefix + spec.Name
}

// LookupTool 按上游 slug 查工具。未登记返回 false，调用方必须据此拒绝请求。
func LookupTool(name string) (ToolSpec, bool) {
	spec, ok := toolIndex[name]
	return spec, ok
}

// ModelNameForTool 返回对客模型名。
func ModelNameForTool(tool string) string {
	return ModelPrefix + tool
}

// ToolForModel 从对客模型名反查工具。
func ToolForModel(model string) (ToolSpec, bool) {
	if !strings.HasPrefix(model, ModelPrefix) {
		return ToolSpec{}, false
	}
	return LookupTool(strings.TrimPrefix(model, ModelPrefix))
}

// GetModelList 返回服务端内置的模型名列表。管理员在渠道模型列表里勾选需要的工具，
// 故此处只暴露已登记工具的全集；渠道实际可用模型仍由渠道配置决定。
func GetModelList() []string {
	models := make([]string, 0, len(toolRegistry))
	for _, spec := range toolRegistry {
		models = append(models, ModelNameForTool(spec.Name))
	}
	return models
}

// resultURLKeys 是已知的产物 URL 键，按优先级排列（视频 → 图像 → 音频 → 通用）。
var resultURLKeys = []string{"video_url", "image_url", "audio_url", "url"}

// ResultURL 从上游 result 对象里取产物 URL。不同工具的结果键名不同，先按已知
// 键取，再兜底扫任意以 _url 结尾的字符串键（按键名排序保证结果确定）。
func ResultURL(result map[string]any) string {
	for _, key := range resultURLKeys {
		if value, ok := result[key].(string); ok && value != "" {
			return value
		}
	}

	fallbackKeys := make([]string, 0, len(result))
	for key, value := range result {
		if !strings.HasSuffix(key, "_url") {
			continue
		}
		if value, ok := value.(string); ok && value != "" {
			fallbackKeys = append(fallbackKeys, key)
		}
	}
	if len(fallbackKeys) == 0 {
		return ""
	}
	sort.Strings(fallbackKeys)
	return result[fallbackKeys[0]].(string)
}

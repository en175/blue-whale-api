package mediakit

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/tidwall/gjson"
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

// ResultKind 声明工具产物类型。它只描述产物「是什么」，不决定「去哪取」——
// 单产物工具可据此走约定键名，多产物或键名不规律的工具必须用 ResultPath 显式声明。
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
	// ResultPath 是相对上游 result 对象的 gjson 路径，声明产物 URL 的确切位置，
	// 例如 video_urls.0、snapshots.0.image_url。留空则按 ResultKind 走约定键名
	// （video_url / image_url / audio_url）；ResultNone 的工具留空即「无产物」。
	//
	// 逐工具实测得出：上游各工具 result 形状不统一，产物可能嵌在数组里
	// （drama-recap-vertical 的 video_urls、extract-frames 的 snapshots），
	// 也可能键名不在约定之列（semantic-segment / drama-script 的 result_url），
	// 而 multi-product 工具（separate-voice 同时给人声与伴奏）必须指定取哪一轨。
	ResultPath string
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
	// 场景切分的产物是逐个片段的视频链，放在 segments 数组里。
	{Name: "segment-scenes", Mode: BillingDuration, ResultKind: ResultNone, ResultPath: "segments.0.segment_video_url"},
	{Name: "asr-subtitles", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "video-ocr", Mode: BillingDuration, ResultKind: ResultNone},
	// 高光「提取」只回 highlight_info（各片段起止时间与评分），不产出文件；
	// 真正把高光剪成片的是 generate-highlights-* 族，产物在 video_urls 里。
	{Name: "analyze-video-highlights", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "analyze-video-storyline", Mode: BillingDuration, ResultKind: ResultNone},
	// 高光剪辑族的成品与竖版解说同形，都在顶层 video_urls 数组里（minigame 实测确认，
	// 另外两个同族工具据此推定，均走同一上游能力）。
	{Name: "generate-highlights-microdrama", Mode: BillingDuration, ResultKind: ResultVideo, ResultPath: "video_urls.0"},
	{Name: "generate-highlights-minigame", Mode: BillingDuration, ResultKind: ResultVideo, ResultPath: "video_urls.0"},
	{Name: "generate-highlights-movie", Mode: BillingDuration, ResultKind: ResultVideo, ResultPath: "video_urls.0"},
	{Name: "matte-greenscreen-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "matte-portrait-video", Mode: BillingDuration, ResultKind: ResultVideo},
	// 剧本还原的产物是 .json.gz 分析包，键名 result_url 不在约定之列。
	{Name: "drama-script", Mode: BillingDuration, ResultKind: ResultNone, ResultPath: "result_url"},
	{Name: "drama-recap", Mode: BillingDuration, ResultKind: ResultVideo},
	// 竖版解说把成品放在顶层 video_urls 数组里，没有单数 video_url 键。
	{Name: "drama-recap-vertical", Mode: BillingDuration, ResultKind: ResultVideo, ResultPath: "video_urls.0"},
	{Name: "add-video-invisible-watermark", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "extract-video-invisible-watermark", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "assess-video-quality", Mode: BillingDuration, ResultKind: ResultNone},
	{Name: "transcode-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "remux-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "martencode-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "face-blur-video", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "face-swap-video", Mode: BillingDuration, ResultKind: ResultVideo},
	// 智能语义切片的成品键名同样是 result_url，不在约定之列。
	{Name: "semantic-segment", Mode: BillingDuration, ResultKind: ResultVideo, ResultPath: "result_url"},
	{Name: "video-frame-interpolation", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "lip-sync", Mode: BillingDuration, ResultKind: ResultVideo},
	{Name: "reframe-video-to-vertical", Mode: BillingDuration, ResultKind: ResultVideo},
	// 抽帧一次产出多张图（snapshots 数组），url 只放第一张作便捷字段，全量在 result 里。
	{Name: "extract-frames", Mode: BillingPerCall, ResultKind: ResultImage, ResultPath: "snapshots.0.image_url"},

	// ── 异步：音频工具 ──────────────────────────────────────────────
	// 人声/伴奏分离同时产出两轨且都在顶层，必须指明取人声轨，否则会随键名字典序漂移。
	{Name: "separate-voice", Mode: BillingDuration, ResultKind: ResultAudio, ResultPath: "voice_audio_url"},
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

// resultKeyForKind 是各产物类型对应的约定键名。上游单产物工具基本都用这套键名，
// 凡不在其中的形状（数组、嵌套、异名键）必须由 ToolSpec.ResultPath 显式声明。
var resultKeyForKind = map[ResultKind]string{
	ResultVideo: "video_url",
	ResultImage: "image_url",
	ResultAudio: "audio_url",
}

// ResultURL 从上游 result 对象里取产物 URL。位置由 spec 决定：ResultPath 非空时
// 按 gjson 路径精确取值（可指向数组元素与嵌套字段），否则按 ResultKind 取约定键名。
//
// 取不到一律返回空串。空串意味着「本次没有产物」——历史实现在取不到时会兜底扫
// 任意 *_url 键并按字典序取第一个，这对多产物工具（separate-voice 的人声/伴奏、
// Drama 场景的 voice/music/sfx）会随键名字典序漂移到语义无关的轨道上，故移除。
func ResultURL(result map[string]any, spec ToolSpec) string {
	if len(result) == 0 {
		return ""
	}
	if spec.ResultPath == "" {
		key, ok := resultKeyForKind[spec.ResultKind]
		if !ok {
			return ""
		}
		value, _ := result[key].(string)
		return value
	}
	body, err := common.Marshal(result)
	if err != nil {
		return ""
	}
	return gjson.GetBytes(body, spec.ResultPath).String()
}

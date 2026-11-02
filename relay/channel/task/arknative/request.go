package arknative

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// NativeRequestContextKey 是请求转换中间件写入解析结果的上下文键。
// relay 层据此判定成片请求，适配器据此重建上游请求体。
const NativeRequestContextKey = "ark_native_request"

// NativeRequest 是下游按火山方舟文档发来的视频任务请求。
//
// Raw 保留原始 JSON：非成片请求要原样透传给上游（包括本结构体还没建模的
// 字段），只覆盖 model 与样片相关的两个键。结构体字段只用于判定，不用于
// 出站序列化。
type NativeRequest struct {
	Model      string              `json:"model"`
	Content    []NativeContentItem `json:"content"`
	Resolution string              `json:"resolution"`
	Draft      *bool               `json:"draft"`
	// Duration 单位秒；火山用 -1 表示「编辑/延长」由模型决定时长。
	Duration *int `json:"duration"`

	Raw map[string]any `json:"-"`
}

// NativeContentItem 是 content[] 的一项。role 之类不影响判定的字段刻意不建模，
// 它们靠 Raw 原样透传。
type NativeContentItem struct {
	Type      string        `json:"type"`
	Text      string        `json:"text"`
	ImageURL  *MediaURL     `json:"image_url"`
	VideoURL  *MediaURL     `json:"video_url"`
	AudioURL  *MediaURL     `json:"audio_url"`
	DraftTask *DraftTaskRef `json:"draft_task"`
}

type MediaURL struct {
	URL string `json:"url"`
}

type DraftTaskRef struct {
	ID string `json:"id"`
}

// Prompt 返回 content[] 中第一条文本项的提示词。火山把提示词放在 content
// 数组里而不是顶层 prompt，内部 TaskSubmitReq 仍要一个 prompt 才能通过通用
// 校验，因此在这里取值。
func (r *NativeRequest) Prompt() string {
	for _, item := range r.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			return item.Text
		}
	}
	return ""
}

// HasDraftTask 报告 content[] 里是否出现 draft_task 项，用于区分「第二步成片」
// 与「id 漏传的第二步」。
func (r *NativeRequest) HasDraftTask() bool {
	for _, item := range r.Content {
		if item.Type == "draft_task" {
			return true
		}
	}
	return false
}

// DraftTaskID 返回 content[] 中 draft_task 引用的解析后任务 ID。
// 非空即表示这是样片出成片的第二步请求。
func (r *NativeRequest) DraftTaskID() string {
	for _, item := range r.Content {
		if item.Type == "draft_task" && item.DraftTask != nil {
			return strings.TrimSpace(item.DraftTask.ID)
		}
	}
	return ""
}

// HasVideoInput 报告请求是否含视频输入，用于查视频档位价格。
// 与 taskcommon.HasVideoInputInMetadata 的判定口径一致：type 或字段存在即算。
func (r *NativeRequest) HasVideoInput() bool {
	for _, item := range r.Content {
		if item.Type == "video_url" || item.VideoURL != nil {
			return true
		}
	}
	return false
}

// IsDraft 报告是否开启样片模式。顶层 draft 是火山文档的写法，这里只认它；
// 缺省或显式 false 都按普通任务处理。
func (r *NativeRequest) IsDraft() bool {
	return r.Draft != nil && *r.Draft
}

// ParseNativeRequest 解析下游按火山文档发来的任务请求，并保留原始 JSON（Raw）
// 供出站原样透传。字段校验由调用方负责。
// ErrNativeBodyNotObject 表示请求体顶层不是 JSON 对象（数组、标量、空 body）。
// 其它解析错误说明顶层是对象但某个字段类型不对，两者对外提示不同。
var ErrNativeBodyNotObject = errors.New("request body must be a JSON object")

func ParseNativeRequest(raw []byte) (*NativeRequest, error) {
	var fields map[string]any
	if err := common.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, ErrNativeBodyNotObject
	}
	var req NativeRequest
	if err := common.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	req.Raw = fields
	return &req, nil
}

// NativeRequestFrom 取出中间件解析好的请求；未经过转换中间件时返回 false。
func NativeRequestFrom(c *gin.Context) (*NativeRequest, bool) {
	value, exists := c.Get(NativeRequestContextKey)
	if !exists {
		return nil, false
	}
	req, ok := value.(*NativeRequest)
	return req, ok
}

// StoreNativeRequest 供请求转换中间件写入解析结果。
func StoreNativeRequest(c *gin.Context, req *NativeRequest) {
	c.Set(NativeRequestContextKey, req)
}

// nativeRequestFromLegacy 把网关旧视频接口（/v1/video/generations）注册在上下文
// 里的内部请求体还原成火山形状：顶层 prompt 补成 content[] 的第一条 text，
// metadata 里的火山顶层参数（content / resolution / ratio / duration / watermark
// 等）原样提到顶层，顶层 duration / seconds / draft 补进缺失的键。
//
// 还原结果只用于校验与构造上游请求体，不写回原生请求的上下文键——relay 层仍以
// “有没有原生请求”区分两个入口，决定成片归属校验与响应形状。
func nativeRequestFromLegacy(c *gin.Context) (*NativeRequest, bool) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, false
	}
	raw := make(map[string]any, len(req.Metadata)+4)
	for key, value := range req.Metadata {
		raw[key] = value
	}
	raw["model"] = req.Model
	content, _ := raw["content"].([]any)
	// 旧接口顶层的 images 与 metadata.content 可以同时出现：把尚未出现的图片
	// 引用补进 content，避免任何一侧被静默丢弃。
	for _, imageURL := range req.Images {
		imageURL = strings.TrimSpace(imageURL)
		if imageURL == "" {
			continue
		}
		duplicate := false
		for _, item := range content {
			m, ok := item.(map[string]any)
			if !ok || m["type"] != "image_url" {
				continue
			}
			image, _ := m["image_url"].(map[string]any)
			if existing, _ := image["url"].(string); existing == imageURL {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		content = append(content, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": imageURL},
		})
	}
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		// 与 tokenhub 一致：旧接口的提示词以顶层 prompt 为准，content 里已有的
		// 文本项不再重复发送，媒体项保持原顺序，提示词排在最后。
		media := make([]any, 0, len(content)+1)
		for _, item := range content {
			if m, ok := item.(map[string]any); ok && m["type"] == "text" {
				continue
			}
			media = append(media, item)
		}
		content = append(media, map[string]any{"type": "text", "text": prompt})
	}
	if content != nil {
		raw["content"] = content
	}
	if _, exists := raw["duration"]; !exists {
		switch {
		case req.Duration > 0:
			raw["duration"] = req.Duration
		case strings.TrimSpace(req.Seconds) != "":
			if seconds, err := strconv.Atoi(strings.TrimSpace(req.Seconds)); err == nil {
				raw["duration"] = seconds
			}
		}
	}
	if taskcommon.IsDraftRequest(&req) {
		raw["draft"] = true
	}
	data, err := common.Marshal(raw)
	if err != nil {
		return nil, false
	}
	native, err := ParseNativeRequest(data)
	if err != nil {
		return nil, false
	}
	return native, true
}

// ArkRequest 返回本次请求的火山形状：原生入口由请求转换中间件写入，旧接口按
// 内部请求体还原。两条入口由此共用同一套校验、上游构造与计费。
func ArkRequest(c *gin.Context) (*NativeRequest, bool) {
	if native, ok := NativeRequestFrom(c); ok {
		return native, true
	}
	return nativeRequestFromLegacy(c)
}

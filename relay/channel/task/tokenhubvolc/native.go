package tokenhubvolc

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
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
func ParseNativeRequest(raw []byte) (*NativeRequest, error) {
	var req NativeRequest
	if err := common.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := common.Unmarshal(raw, &fields); err != nil {
		// 顶层不是对象（数组、标量）时在这里失败
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

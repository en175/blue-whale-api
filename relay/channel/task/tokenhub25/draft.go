package tokenhub25

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// finalizeRequest 是 POST /v1/videos/{video_id}/finalize 请求体中被接受的全部字段。
//
// 这是一个白名单，而不是「解析后清掉危险字段」的黑名单：成片请求必须省略
// content / prompt / duration / ratio / seed / generate_audio /
// omni_reference_task_type（自动沿用第一步的样片），因此这些字段在结构体上
// 根本不存在，客户端无论传什么都影响不了成片内容与时长。
//
// 唯一可重设的是输出规格类参数（分辨率除外——成片固定 1080p）。
type finalizeRequest struct {
	Resolution            string         `json:"resolution,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	OutputFormat          string         `json:"output_format,omitempty"`
	Watermark             *dto.BoolValue `json:"watermark,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	Priority              *dto.IntValue  `json:"priority,omitempty"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	SafetyIdentifier      string         `json:"safety_identifier,omitempty"`
}

// finalizeContentItem 是成片请求体 content 数组里唯一允许出现的项：一个指向样片
// 的 draft_task 引用。
//
// 它在本包本地定义，不复用 tokenhub 的 ContentItem：61 渠道的入站/出站行为因此
// 逐位不变（61 的 ContentItem 没有 draft_task 字段，也就无从透传客户端塞的引用）。
type finalizeContentItem struct {
	Type      string               `json:"type"`
	DraftTask finalizeDraftTaskRef `json:"draft_task"`
}

type finalizeDraftTaskRef struct {
	ID string `json:"id"`
}

// finalizePayload 是实际发给华鲲的成片请求体。与 finalizeRequest 分开定义是
// 故意的：客户端 JSON 永远不会被反序列化进这个结构体，model 与 content 只能由
// 网关填充，其中 content[0].draft_task.id 来自原任务的上游样片 ID。
type finalizePayload struct {
	Model                 string                `json:"model"`
	Content               []finalizeContentItem `json:"content"`
	Resolution            string                `json:"resolution"`
	ReturnLastFrame       *dto.BoolValue        `json:"return_last_frame,omitempty"`
	OutputFormat          string                `json:"output_format,omitempty"`
	Watermark             *dto.BoolValue        `json:"watermark,omitempty"`
	ServiceTier           string                `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue         `json:"execution_expires_after,omitempty"`
	Priority              *dto.IntValue         `json:"priority,omitempty"`
	CallbackURL           string                `json:"callback_url,omitempty"`
	SafetyIdentifier      string                `json:"safety_identifier,omitempty"`
}

// finalizeRequestContextKey 让 ValidateRequestAndSetAction 解析出的结果能被
// BuildRequestBody 读回，而不必重解析请求体。
const finalizeRequestContextKey = "tokenhub25_finalize_request"

// requestedResolution 读取 metadata.resolution 并归一化（小写、去空白）。
func requestedResolution(req *relaycommon.TaskSubmitReq) string {
	res, _ := req.Metadata["resolution"].(string)
	return strings.ToLower(strings.TrimSpace(res))
}

// validateFinalizeSubmit 校验并解析成片请求。
//
// 空请求体与 `{}` 都必须通过——客户端通常直接 POST 空体，由网关套用 1080p
// 默认值。因此这里按 body 大小先行短路，避免 JSON 解析器对空输入报语法错误。
func validateFinalizeSubmit(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req finalizeRequest

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_request_body_failed", http.StatusBadRequest)
	}
	if storage.Size() > 0 {
		if err := common.UnmarshalBodyReusable(c, &req); err != nil {
			return service.TaskErrorWrapper(err, "invalid_json", http.StatusBadRequest)
		}
	}

	if resolution := strings.ToLower(strings.TrimSpace(req.Resolution)); resolution != "" && resolution != Res1080p {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("finalize only supports resolution %s", Res1080p), "invalid_resolution", http.StatusBadRequest)
	}

	info.Action = constant.TaskActionFinalize
	c.Set(finalizeRequestContextKey, req)
	return nil
}

// finalizeParamsFromContext 取回 validateFinalizeSubmit 解析出的请求参数。
func finalizeParamsFromContext(c *gin.Context) finalizeRequest {
	if v, ok := c.Get(finalizeRequestContextKey); ok {
		if req, ok := v.(finalizeRequest); ok {
			return req
		}
	}
	return finalizeRequest{}
}

// buildFinalizePayload 用网关推导的上游样片 ID 构造成片请求体。
// draft_task.id 只可能来自 ResolveOriginTask 读取的原任务，客户端无法提供。
func buildFinalizePayload(info *relaycommon.RelayInfo, params finalizeRequest) ([]byte, error) {
	payload := finalizePayload{
		Model: info.UpstreamModelName,
		Content: []finalizeContentItem{{
			Type:      "draft_task",
			DraftTask: finalizeDraftTaskRef{ID: info.OriginUpstreamTaskID},
		}},
		Resolution:            Res1080p,
		ReturnLastFrame:       params.ReturnLastFrame,
		OutputFormat:          params.OutputFormat,
		Watermark:             params.Watermark,
		ServiceTier:           params.ServiceTier,
		ExecutionExpiresAfter: params.ExecutionExpiresAfter,
		Priority:              params.Priority,
		CallbackURL:           params.CallbackURL,
		SafetyIdentifier:      params.SafetyIdentifier,
	}
	return common.Marshal(payload)
}

// rewritePayloadFields 读出 payload 并覆盖指定字段后回写，用于样片模式强制
// draft=true 与 resolution=480p（客户端可能只传了其中一个），以及补写
// omni_reference_task_type。
//
// 覆盖的是顶层键——tokenhub 的 requestPayload 里 Draft 与 Resolution 都在顶层，
// 两处落点一致。
func rewritePayloadFields(body io.Reader, fields map[string]any) (io.Reader, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := common.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	for key, value := range fields {
		payload[key] = value
	}
	out, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(out), nil
}

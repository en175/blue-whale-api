package aistore

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskdoubao "github.com/QuantumNous/new-api/relay/channel/task/doubao"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	tasktokenhubvolc "github.com/QuantumNous/new-api/relay/channel/task/tokenhubvolc"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// TaskAdaptor 是运河启源渠道（type 71）的适配器。上游是火山方舟协议的转售封装
// （base URL 形如 https://aistore.gsiig.cn/v1），对客只开放火山原生
// /api/v3/contents/generations/tasks。
//
// 请求体与响应体都按火山原文透传：请求以中间件解析出的 native.Raw 为底，只覆盖
// model；响应保留上游全部字段，只把 id 换成网关公开 ID 并补 upstreamTaskId。
// 与 doubao 适配器的关键差别是它不经过 requestPayload 结构体，因此
// output_format、return_last_frame 等火山新增字段不会被静默丢弃。
type TaskAdaptor struct {
	taskdoubao.TaskAdaptor
	baseURL string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.TaskAdaptor.Init(info)
	a.baseURL = info.ChannelBaseUrl
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

// BuildRequestURL 拼上游地址。运河启源把版本前缀放在 base URL 里
// （.../v1），与火山的 /api/v3 不同，因此不能沿用 doubao 的实现。
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return strings.TrimSuffix(a.baseURL, "/") + "/contents/generations/tasks", nil
}

// FetchTask 轮询上游任务。baseUrl 由轮询任务从渠道记录传入。
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := strings.TrimSuffix(baseUrl, "/") + "/contents/generations/tasks/" + taskID
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// DoRequest 必须显式覆写。DoRequest 是从嵌入的 doubao 适配器提升上来的方法，
// 它把「接收者」原样传给 channel.DoTaskApiRequest；Go 的嵌入不是虚分发，所以
// 提升版内部调到的仍是 doubao 的 BuildRequestURL（{base}/api/v3/...），本类型
// 的覆写会变成死代码。这里把外层适配器交给同一个入口，URL 与鉴权头才命中本类型。
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// ValidateRequestAndSetAction 校验火山原生请求并写任务类型。
//
// duration 由适配器自己判边界：火山用 -1 表示「编辑/延长由模型决定时长」，内部
// 任务的时长校验只接受正数，因此原生 duration 不能交给通用校验。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	native, ok := tasktokenhubvolc.NativeRequestFrom(c)
	if !ok {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("native ark request is required"), "invalid_request", http.StatusBadRequest)
	}

	// 成片第二步：引用已由 ResolveOriginTask 校验归属、状态与 7 天窗口，
	// 请求体里也没有 prompt，走不了通用校验。
	if info != nil && info.Action == constant.TaskActionFinalize {
		return nil
	}

	// 通用校验读中间件改写后的内部请求体（prompt + metadata），提示词必填等
	// 既有约束原样生效。
	if taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}

	if native.Duration != nil {
		if d := *native.Duration; d != -1 && (d < 1 || d > relaycommon.MaxTaskDurationSeconds) {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("duration must be -1 or between 1 and %d", relaycommon.MaxTaskDurationSeconds),
				"invalid_duration", http.StatusBadRequest)
		}
	}

	if info != nil {
		info.HasVideoInput = native.HasVideoInput()
		info.Action = inferAction(native)
	}

	if !native.IsDraft() {
		return nil
	}

	// 样片模式只有 2.5 支持。渠道 model_mapping 在提交链路后段才生效，这里
	// 先解析一次，避免把 2.5 映射成不含 "2.5" 的别名时被误拒。
	clientModel := native.Model
	if info != nil && info.OriginModelName != "" {
		clientModel = info.OriginModelName
	}
	if !isSeedance25(clientModel, tasktokenhubvolc.MappedModelName(c, clientModel)) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("draft mode is only supported by Seedance 2.5 models"), "invalid_draft_model", http.StatusBadRequest)
	}
	if resolution := strings.ToLower(strings.TrimSpace(native.Resolution)); resolution != "" && resolution != Res480p {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("draft task only supports resolution %s", Res480p), "invalid_resolution", http.StatusBadRequest)
	}
	if info != nil {
		info.Draft = true
	}
	return nil
}

// BuildRequestBody 构造上游请求体：普通与样片请求以 native.Raw 为底，只覆盖
// model 与样片字段；成片请求从零构造，content 只能是 draft_task 引用。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	native, ok := tasktokenhubvolc.NativeRequestFrom(c)
	if !ok {
		return nil, fmt.Errorf("native ark request is missing")
	}

	modelName := native.Model
	if info != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}

	var payload map[string]any
	if info != nil && info.Action == constant.TaskActionFinalize {
		if info.OriginUpstreamTaskID == "" {
			// 正常情况下 ResolveOriginTask 已经拒绝；这里兜底避免发出无 id 的样片引用
			return nil, fmt.Errorf("origin upstream task id is missing")
		}
		payload = map[string]any{
			"model": modelName,
			"content": []any{map[string]any{
				"type":       "draft_task",
				"draft_task": map[string]any{"id": info.OriginUpstreamTaskID},
			}},
			// 上游成片档位只有 1080p：客户端传任何值都被覆盖，不报错。
			"resolution": Res1080p,
		}
		for _, key := range finalizeOptionKeys {
			if value, ok := native.Raw[key]; ok {
				payload[key] = value
			}
		}
		info.RequestedResolution = Res1080p
		info.UpstreamResolution = Res1080p
	} else {
		// 成片引用只能走 finalize 分支：其它入口把 draft_task 当普通 content 透传
		// 会绕过成片校验与两档计费。
		if native.HasDraftTask() {
			return nil, fmt.Errorf("draft_task references are only accepted on finalize requests")
		}
		payload = make(map[string]any, len(native.Raw)+1)
		for key, value := range native.Raw {
			payload[key] = value
		}
		// group 是网关的分组路由键，不是火山字段
		delete(payload, "group")
		payload["model"] = modelName
		if info != nil && info.Draft {
			// 无论客户端只传了 draft 还是只传了 resolution，上游都必须同时收到
			// draft=true 与 480p，否则样片会被当成普通任务计费。
			payload["draft"] = true
			payload["resolution"] = Res480p
			info.RequestedResolution = Res480p
			info.UpstreamResolution = Res480p
		} else if native.Resolution != "" && info != nil {
			info.RequestedResolution = native.Resolution
		}
	}

	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// finalizeOptionKeys 是成片请求里可以重设的输出规格类参数。content / prompt /
// duration / ratio / seed / generate_audio 刻意不在其中：成片沿用样片的内容与
// 时长（火山要求省略这些字段）。
var finalizeOptionKeys = []string{
	"output_format",
	"watermark",
	"return_last_frame",
	"service_tier",
	"execution_expires_after",
	"priority",
	"callback_url",
	"safety_identifier",
}

// DoResponse 把上游创建响应原样回给下游，只把 id 换成网关公开 ID。上游 cgt 号
// 以 upstreamTaskId 带出，便于下游到运河启源侧排障。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var upstream map[string]any
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(
			fmt.Errorf("body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	upstreamID, _ := upstream["id"].(string)
	if upstreamID == "" {
		message, _ := upstream["message"].(string)
		if message == "" {
			if upstreamError, ok := upstream["error"].(map[string]any); ok {
				message, _ = upstreamError["message"].(string)
			}
		}
		if message == "" {
			message = "task_id is empty"
		}
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("%s", message), "upstream_error", http.StatusInternalServerError)
	}

	upstream["id"] = info.PublicTaskID
	upstream["upstreamTaskId"] = upstreamID
	out, err := common.Marshal(upstream)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", out)
	return upstreamID, responseBody, nil
}

// ParseTaskResult 解析上游查询响应。expired 是上游的终态（任务号只保留 7 天），
// 必须映射成失败，否则任务会被一直当作进行中轮询下去。
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var res struct {
		Status  string `json:"status"`
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
		Usage struct {
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := common.Unmarshal(respBody, &res); err != nil {
		return nil, fmt.Errorf("unmarshal aistore task result failed: %w", err)
	}

	result := &relaycommon.TaskInfo{Code: 0}
	switch res.Status {
	case "pending", "queued":
		result.Status = model.TaskStatusQueued
		result.Progress = "10%"
	case "processing", "running":
		result.Status = model.TaskStatusInProgress
		result.Progress = "50%"
	case "succeeded", "success":
		result.Status = model.TaskStatusSuccess
		result.Progress = "100%"
		result.Url = res.Content.VideoURL
		result.CompletionTokens = res.Usage.CompletionTokens
		result.TotalTokens = res.Usage.TotalTokens
	case "failed":
		result.Status = model.TaskStatusFailure
		result.Progress = "100%"
		result.Reason = res.Error.Message
	case "expired":
		result.Status = model.TaskStatusFailure
		result.Progress = "100%"
		result.Reason = "upstream task expired"
		if res.Error.Message != "" {
			result.Reason = res.Error.Message
		}
	case "cancelled", "canceled":
		// 用户没有取消能力，上游主动取消（安全/审核/系统）同样是「没出成结果」，
		// 必须判终态失败，否则任务会一直轮询到全局超时才退款。
		result.Status = model.TaskStatusFailure
		result.Progress = "100%"
		result.Reason = "upstream task cancelled"
		if res.Error.Message != "" {
			result.Reason = res.Error.Message
		}
	default:
		result.Status = model.TaskStatusInProgress
		result.Progress = "30%"
	}
	return result, nil
}

// ConvertToNativeArkTask 把落库任务转成火山原生视频对象：上游原文的字段全部
// 保留（content.last_frame_url、seed、framespersecond 等），只覆盖 id 并补齐
// 网关侧一定知道的字段，保证官方字段齐全。
func (a *TaskAdaptor) ConvertToNativeArkTask(originTask *model.Task) ([]byte, error) {
	data := map[string]any{}
	if len(originTask.Data) > 0 {
		if err := common.Unmarshal(originTask.Data, &data); err != nil {
			return nil, fmt.Errorf("unmarshal aistore task data failed: %w", err)
		}
	}
	if upstreamID, _ := data["id"].(string); upstreamID != "" {
		data["upstreamTaskId"] = upstreamID
	}
	data["id"] = originTask.TaskID
	if _, ok := data["model"]; !ok {
		modelName := originTask.Properties.OriginModelName
		if modelName == "" {
			modelName = originTask.Properties.UpstreamModelName
		}
		if modelName != "" {
			data["model"] = modelName
		}
	}
	if _, ok := data["status"]; !ok {
		data["status"] = arkTaskStatus(originTask.Status)
	}
	if _, ok := data["created_at"]; !ok {
		data["created_at"] = originTask.CreatedAt
	}
	if _, ok := data["updated_at"]; !ok {
		data["updated_at"] = originTask.UpdatedAt
	}
	if originTask.Status == model.TaskStatusFailure {
		if _, ok := data["error"]; !ok {
			message := originTask.FailReason
			if message == "" {
				message = "upstream task failed"
			}
			data["error"] = map[string]any{"code": "TaskFailed", "message": message}
		}
	}
	return common.Marshal(data)
}

// EstimateBilling 返回档位价倍率：样片固定 480p 档，成片固定 1080p 档且
// 「是否含视频输入」继承第一步的样片，普通请求按本次请求体的分辨率判定。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if info != nil && info.Action == constant.TaskActionFinalize {
		return taskcommon.EstimateVideoTierBillingForTier(info, Res1080p, info.OriginHasVideoInput)
	}
	if info != nil && info.Draft {
		// 480p 与 720p 同属基准键，基准价本就是样片的正确价格，不告警。
		return taskcommon.EstimateVideoTierBillingForTier(info, Res480p, info.HasVideoInput)
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	ratios := taskcommon.EstimateVideoTierBilling(info, &req)
	// 直接提交 1080p / 4k 也按档位收费：漏配时静默按基准价结算，必须告警。
	if resolution, _ := req.Metadata["resolution"].(string); resolution != "" {
		normalized := strings.ToLower(strings.TrimSpace(resolution))
		if normalized == Res1080p || normalized == "4k" {
			taskcommon.WarnMissingTier(c, info, normalized, taskcommon.HasVideoInputInMetadata(req.Metadata), ratios)
		}
	}
	return ratios
}

// inferAction 按 content[] 的实际输入形态写任务类型：视频优先，其次图片，
// 纯文本兜底。任务日志与筛选依赖它区分「文生 / 图生 / 视频生」。
func inferAction(native *tasktokenhubvolc.NativeRequest) string {
	hasImage := false
	for _, item := range native.Content {
		if item.Type == "video_url" || item.VideoURL != nil {
			return constant.TaskActionVideoGenerate
		}
		if item.Type == "image_url" || item.ImageURL != nil {
			hasImage = true
		}
	}
	if hasImage {
		return constant.TaskActionGenerate
	}
	return constant.TaskActionTextGenerate
}

// isSeedance25 报告模型名是否为 Seedance 2.5 系（样片模式的唯一支持者）。
// 运河启源的 2.5 模型叫 cdance2.5，因此按版本号而不是厂商前缀判断。
func isSeedance25(names ...string) bool {
	for _, name := range names {
		lower := strings.ToLower(strings.TrimSpace(name))
		if lower == "" {
			continue
		}
		if strings.Contains(lower, "2.5") || strings.Contains(lower, "2-5") {
			return true
		}
	}
	return false
}

// arkTaskStatus 把网关任务状态翻译成火山原生状态词，仅在落库数据缺 status 时兜底。
func arkTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued:
		return "queued"
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	default:
		return "running"
	}
}

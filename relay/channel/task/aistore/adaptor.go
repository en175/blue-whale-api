package aistore

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/arknative"
	taskdoubao "github.com/QuantumNous/new-api/relay/channel/task/doubao"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// TaskAdaptor 是运河启源渠道的适配器：类型 71 对客只开放火山原生
// /api/v3/contents/generations/tasks；类型 72 对客只开放旧视频接口
// （/v1/video/generations 与 /v1/videos/{id}/finalize），上游完全相同
// （base URL 形如 https://aistore.gsiig.cn/v1）。
//
// 请求体与响应体都按火山原文与上游交互：原生入口以中间件解析出的 native.Raw
// 为底，只覆盖 model；旧接口入口先由 arknative.ArkRequest 还原成火山形状。
// 与 doubao 适配器的关键差别是它不经过 requestPayload 结构体，因此
// output_format、return_last_frame 等火山新增字段不会被静默丢弃。
type TaskAdaptor struct {
	taskdoubao.TaskAdaptor
	baseURL     string
	channelType int
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.TaskAdaptor.Init(info)
	a.baseURL = info.ChannelBaseUrl
	a.channelType = info.ChannelType
}

func (a *TaskAdaptor) GetChannelName() string {
	if a.channelType == constant.ChannelTypeAistoreVolcLegacy {
		return LegacyChannelName
	}
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
	// 上游 WAF 拒绝 Go 默认 UA 的 GET 请求（HTML 403），不显式设置会让轮询
	// 永远解析失败，任务停在 queued 直到全局超时。
	req.Header.Set("User-Agent", UserAgent)

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

// ValidateRequestAndSetAction 校验视频任务请求并写任务类型。
//
// 原生入口与旧接口入口共用同一套判定：先由通用校验读内部请求体，再用
// arknative.ArkRequest 还原火山形状。旧接口入口额外拒绝 draft_task 引用——
// 成片只能走 /v1/videos/{id}/finalize，否则会绕过归属校验与 1080p 档计费。
//
// duration 由适配器自己判边界：火山用 -1 表示「编辑/延长由模型决定时长」，内部
// 任务的时长校验只接受正数，因此原生 duration 不能交给通用校验。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	// 成片第二步：原生入口的引用已由 ResolveOriginTask 校验归属、状态与 7 天
	// 窗口；旧接口没有 prompt，走不了通用校验，只解析输出规格参数。
	if info != nil && info.Action == constant.TaskActionFinalize {
		return arknative.ValidateFinalizeRequest(c, Res1080p)
	}

	// 通用校验读中间件改写后的内部请求体（prompt + metadata），提示词必填等
	// 既有约束原样生效。旧接口的火山形状要在它之后才能还原。
	if taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
		return taskErr
	}

	native, ok := arknative.ArkRequest(c)
	if !ok {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("task metadata cannot be converted to a video request"), "invalid_request", http.StatusBadRequest)
	}

	// 成片引用只能走 finalize 分支：旧提交入口把 draft_task 当普通 content 透传
	// 会绕过成片校验与两档计费。
	if _, isNative := arknative.NativeRequestFrom(c); !isNative && native.HasDraftTask() {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("draft_task references are only accepted on the finalize endpoint"),
			"invalid_content", http.StatusBadRequest)
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
		info.Action = arknative.InferAction(native)
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
	if !isSeedance25(clientModel, taskcommon.MappedModelName(c, clientModel)) {
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
// model 与样片字段；成片请求从零构造，content 只能是 draft_task 引用。旧接口
// 入口与原生入口共用这一段，区别只在 ArkRequest 的还原来源与成片参数来源。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	native, _ := arknative.ArkRequest(c)

	modelName := ""
	if native != nil {
		modelName = native.Model
	}
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
		arknative.ApplyFinalizeOptions(payload, c, native)
		info.RequestedResolution = Res1080p
		info.UpstreamResolution = Res1080p
	} else {
		if native == nil {
			return nil, fmt.Errorf("ark native request is missing")
		}
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

// DoResponse 处理上游创建响应。原生入口把上游原文回给下游，只把 id 换成网关
// 公开 ID，cgt 号以 upstreamTaskId 带出；旧接口入口按既有视频接口返回 OpenAI
// 视频对象（与渠道 61/66/67/70 一致）。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var upstream map[string]any
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		// 上游原文可能含任务号等内部信息，只进后端日志，不回给下游。
		bodyForLog := string(responseBody)
		if len(bodyForLog) > 512 {
			bodyForLog = bodyForLog[:512] + "..."
		}
		logger.LogWarn(c.Request.Context(), "aistore create response is not json: "+bodyForLog)
		return "", nil, service.TaskErrorWrapper(errors.New("invalid upstream response"), "unmarshal_response_body_failed", http.StatusInternalServerError)
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

	if c.Request != nil && strings.HasPrefix(c.Request.URL.Path, constant.NativeArkVideoTaskPath) {
		upstream["id"] = info.PublicTaskID
		upstream["upstreamTaskId"] = upstreamID
		out, err := common.Marshal(upstream)
		if err != nil {
			return "", nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", out)
	} else {
		video := dto.NewOpenAIVideo()
		video.ID = info.PublicTaskID
		video.TaskID = info.PublicTaskID
		video.CreatedAt = time.Now().Unix()
		video.Model = info.OriginModelName
		c.JSON(http.StatusOK, video)
	}
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
	// PrivateData 是权威值：轮询每轮都会把 Data 换成上游查询响应，某轮缺 id 时
	// 若只靠 Data 反推，upstreamTaskId 会凭空消失。
	upstreamID := originTask.PrivateData.UpstreamTaskID
	if upstreamID == "" {
		upstreamID, _ = data["id"].(string)
	}
	if upstreamID != "" && upstreamID != originTask.TaskID {
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
		ratios := taskcommon.EstimateVideoTierBillingForTier(info, Res1080p, info.OriginHasVideoInput)
		taskcommon.WarnMissingTier(c, info, Res1080p, info.OriginHasVideoInput, ratios)
		return ratios
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

// isSeedance25 报告模型名是否为 Seedance 2.5 系（样片模式的唯一支持者）。
// 运河启源的 2.5 模型叫 cdance2.5，因此按版本号而不是厂商前缀判断。
// 版本号必须是独立片段：v12.5 这类名字不能被当成 2.5，否则样片请求会发到
// 不支持 draft 的模型上，预扣费已经发生。
func isSeedance25(names ...string) bool {
	for _, name := range names {
		lower := strings.ToLower(strings.TrimSpace(name))
		if lower == "" {
			continue
		}
		for _, token := range []string{"2.5", "2-5"} {
			for offset := 0; ; {
				pos := strings.Index(lower[offset:], token)
				if pos < 0 {
					break
				}
				start := offset + pos
				end := start + len(token)
				standaloneBefore := start == 0 || !isASCIIDigit(lower[start-1])
				standaloneAfter := end == len(lower) || !isASCIIDigit(lower[end])
				if standaloneBefore && standaloneAfter {
					return true
				}
				offset = start + 1
			}
		}
	}
	return false
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
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

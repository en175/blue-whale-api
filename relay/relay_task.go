package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	tokenhubvolc "github.com/QuantumNous/new-api/relay/channel/task/tokenhubvolc"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type TaskSubmitResult struct {
	UpstreamTaskID string
	TaskData       []byte
	Platform       constant.TaskPlatform
	Quota          int
	//PerCallPrice   types.PriceData
}

// ResolveOriginTask 处理基于已有任务的提交（remix / finalize）：
// 查找原始任务、从中提取模型名称、将渠道锁定到原始任务的渠道
// （通过 info.LockedChannel，重试时复用同一渠道并轮换 key），
// 以及提取 OtherRatios（时长、分辨率）。
// 该函数在控制器的重试循环之前调用一次，其结果通过 info 字段和上下文持久化。
func ResolveOriginTask(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	// 火山原生协议的 action 不在路径里：第二步成片由 content[] 的 draft_task
	// 引用表达，来源任务 ID 也就来自请求体。取出来后与 /v1/videos 的 finalize
	// 共用下面同一套校验（归属、状态、7 天窗口、上游 ID）。
	if native, ok := tokenhubvolc.NativeRequestFrom(c); ok {
		if draftTaskID := native.DraftTaskID(); draftTaskID != "" {
			info.Action = constant.TaskActionFinalize
			info.OriginTaskID = draftTaskID
		}
	}

	// 检测 remix / finalize action
	path := c.Request.URL.Path
	if strings.Contains(path, "/v1/videos/") {
		switch {
		case strings.HasSuffix(path, "/remix"):
			info.Action = constant.TaskActionRemix
		case strings.HasSuffix(path, "/finalize"):
			info.Action = constant.TaskActionFinalize
		}
	}

	// 提取 remix / finalize 任务的 video_id
	if info.Action == constant.TaskActionRemix || info.Action == constant.TaskActionFinalize {
		videoID := c.Param("video_id")
		if strings.TrimSpace(videoID) == "" {
			// 原生协议没有路径参数，来源任务 ID 已在上面的 draft_task 分支取到
			videoID = info.OriginTaskID
		}
		if strings.TrimSpace(videoID) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video_id is required"), "invalid_request", http.StatusBadRequest)
		}
		info.OriginTaskID = videoID
	}

	if info.OriginTaskID == "" {
		return nil
	}

	// 下面要读 info.ChannelId（ChannelMeta 的提升字段）来判断本次请求是否已经落在
	// 原任务的渠道上。ChannelMeta 由 InitChannelMeta 填充，而它在 ResolveOriginTask
	// 之后才被 RelayTaskSubmit 调用——不在这里补一次，info.ChannelId 就是解引用空指针。
	// 锁原任务的路径本来就没有选中渠道，因此这里拿到的是「无渠道」状态，比较结果
	// 正是「需要切到原任务渠道」，与既有语义一致。
	info.InitChannelMeta(c)

	// 查找原始任务
	originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
	}
	if !exist {
		return service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
	}

	// 从原始任务推导模型名称
	originModelName := originTask.Properties.OriginModelName
	if originModelName == "" {
		originModelName = originTask.Properties.UpstreamModelName
	}
	if originModelName == "" {
		var taskData map[string]interface{}
		_ = common.Unmarshal(originTask.Data, &taskData)
		if m, ok := taskData["model"].(string); ok {
			originModelName = m
		}
	}
	if info.OriginModelName == "" {
		info.OriginModelName = originModelName
	}

	// 锁定到原始任务的渠道（重试时复用同一渠道，轮换 key）
	ch, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
	}
	if ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task is disabled"), "task_channel_disable", http.StatusBadRequest)
	}
	// 出成片必须留在能承载样片的同一条通道上：/api/v3 只服务类型 70，反向不成立
	// ——类型 70 同时兼容旧视频接口，所以旧接口的样片可以在类型 70 渠道上成片。
	// 但原生协议的样片不能跨到其它渠道：那些渠道的适配器不认 draft_task 引用。
	if info.Action == constant.TaskActionFinalize {
		if _, nativeRequest := tokenhubvolc.NativeRequestFrom(c); nativeRequest && ch.Type != constant.ChannelTypeTokenHubVolc {
			return service.TaskErrorWrapperLocal(errors.New("the origin task was not created by a native channel"), "origin_task_not_native", http.StatusBadRequest)
		}
	}
	// 锁定到原任务渠道会绕过 Distribute 的路径校验，因此这里补一次：remix 只在
	// 原渠道执行，但类型 70 不支持 remix（它只兼容 /v1/video/generations 与成片），
	// 不能因为“渠道已锁定”就让 70 接下一个它不认识的动作。
	if !constant.ChannelServesRequestPath(ch.Type, c.Request.URL.Path) {
		return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task does not support this path"), "origin_task_path_mismatch", http.StatusBadRequest)
	}
	info.LockedChannel = ch

	if originTask.ChannelId != info.ChannelId {
		key, _, newAPIError := ch.GetNextEnabledKey()
		if newAPIError != nil {
			return service.TaskErrorWrapper(newAPIError, "channel_no_available_key", newAPIError.StatusCode)
		}
		common.SetContextKey(c, constant.ContextKeyChannelKey, key)
		common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
		common.SetContextKey(c, constant.ContextKeyChannelId, originTask.ChannelId)

		info.ChannelBaseUrl = ch.GetBaseURL()
		info.ChannelId = originTask.ChannelId
		info.ChannelType = ch.Type
		info.ApiKey = key
	}

	// finalize（样片出成片）：只有本人的、已成功的、7 天内的 draft 任务才能作为成片源。
	// 校验通过后把上游 draft id 与 Step 1 的 hasVideo 维度写入 info——前者是请求体的
	// 唯一来源（绝不接受客户端传入），后者是 Step 2 的 1080p 档位单价的决定因素。
	// 这里提前 return，绝不能继承 Step 1 的 OtherRatios：它的 seconds/size 会把
	// Step 2 的 token 结算放大数倍（对比下面 remix 的整份复制）。
	if info.Action == constant.TaskActionFinalize {
		// 出片模型必须由样片决定：成片请求仍带 model（协议要求），但客户端可以
		// 传一个更便宜的模型来压低预扣与结算，而实际产出是原样片的模型。无条件
		// 用原任务记录的模型覆盖，客户端值只用于日志。
		if originModelName == "" {
			// 模型名为空时上游请求会退化成无价格的 "66_finalizedraft"，导致静默少收
			return service.TaskErrorWrapperLocal(errors.New("origin model name is missing"), "origin_model_name_missing", http.StatusBadRequest)
		}
		if info.OriginModelName != "" && info.OriginModelName != originModelName {
			logger.LogWarn(c.Request.Context(), "finalize ignores mismatched client model: client="+info.OriginModelName+" origin="+originModelName)
		}
		info.OriginModelName = originModelName
		if !originTask.Properties.Draft {
			return service.TaskErrorWrapperLocal(errors.New("the origin task is not a draft"), "origin_not_draft", http.StatusBadRequest)
		}
		if originTask.Status != model.TaskStatusSuccess {
			return service.TaskErrorWrapperLocal(errors.New("the origin draft task is not finished"), "origin_task_not_finished", http.StatusConflict)
		}
		// 上游文档：样片 id 的有效期 7 天，从样片任务的 created_at 起算。这里用网关
		// 的 SubmitTime（与上游 created_at 只差一次网络往返）。不能用 FinishTime——
		// 它在样片生成完成之后，窗口比上游允许的更长，会把上游已判过期的样片放行，
		// 白跑一次预扣费。旧数据缺 SubmitTime 时退回 CreatedAt。
		draftStartedAt := originTask.SubmitTime
		if draftStartedAt <= 0 {
			draftStartedAt = originTask.CreatedAt
		}
		if draftStartedAt > 0 && time.Now().Unix()-draftStartedAt > relaycommon.DraftTaskValidSeconds {
			return service.TaskErrorWrapperLocal(errors.New("the origin draft task has expired"), "origin_draft_expired", http.StatusBadRequest)
		}
		// 必须取 PrivateData 里的上游 ID，不能用 GetUpstreamTaskID()——后者在
		// PrivateData 为空时回退到 task.TaskID，而那是网关的公开 ID（task_xxx）。
		// 把它当 draft_task.id 发出去，上游必然报错，而预扣费已经发生。
		upstreamTaskID := originTask.PrivateData.UpstreamTaskID
		if upstreamTaskID == "" {
			return service.TaskErrorWrapperLocal(errors.New("the origin task has no upstream task id"), "origin_upstream_id_missing", http.StatusBadRequest)
		}
		info.OriginUpstreamTaskID = upstreamTaskID
		info.OriginHasVideoInput = originTask.PrivateData.HasVideoInput
		return nil
	}

	// 提取 remix 参数（时长、分辨率 → OtherRatios）
	if info.Action == constant.TaskActionRemix {
		if originTask.PrivateData.BillingContext != nil {
			// 新的 remix 逻辑：直接从原始任务的 BillingContext 中提取 OtherRatios（如果存在）
			for s, f := range originTask.PrivateData.BillingContext.OtherRatios {
				info.PriceData.AddOtherRatio(s, f)
			}
		} else {
			// 旧的 remix 逻辑：直接从 task data 解析 seconds 和 size（如果存在）
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			secondsStr, _ := taskData["seconds"].(string)
			seconds, _ := strconv.Atoi(secondsStr)
			if seconds <= 0 {
				seconds = 4
			}
			// 历史任务数据可能包含未经校验的时长，作为计费乘数前必须钳制
			if seconds > relaycommon.MaxTaskDurationSeconds {
				seconds = relaycommon.MaxTaskDurationSeconds
			}
			sizeStr, _ := taskData["size"].(string)
			info.PriceData.AddOtherRatio("seconds", float64(seconds))
			info.PriceData.AddOtherRatio("size", 1)
			if sizeStr == "1792x1024" || sizeStr == "1024x1792" {
				info.PriceData.AddOtherRatio("size", 1.666667)
			}
		}
	}

	return nil
}

// IsNativeArkFinalizeRequest 报告当前请求是否是火山原生协议的第二步成片。
// 成片的上游 create 不是幂等的，调用方据此禁止重试。
func IsNativeArkFinalizeRequest(c *gin.Context) bool {
	native, ok := tokenhubvolc.NativeRequestFrom(c)
	return ok && native.DraftTaskID() != ""
}

// RelayTaskSubmit 完成 task 提交的全部流程（每次尝试调用一次）：
// 刷新渠道元数据 → 确定 platform/adaptor → 验证请求 →
// 估算计费(EstimateBilling) → 计算价格 → 预扣费（仅首次）→
// 构建/发送/解析上游请求 → 提交后计费调整(AdjustBillingOnSubmit)。
// 控制器负责 defer Refund 和成功后 Settle。
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo) (*TaskSubmitResult, *dto.TaskError) {
	info.InitChannelMeta(c)

	// 1. 确定 platform → 创建适配器 → 验证请求
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest)
	}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return nil, taskErr
	}

	// 2. 确定模型名称
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
	}

	// 2.5 应用渠道的模型映射（与同步任务对齐）
	info.OriginModelName = modelName
	info.UpstreamModelName = modelName
	// MediaKit 例外：模型名（mediakit-<tool>）是协议的一部分，决定打到哪个上游
	// 端点，不是可按渠道改写的别名——改写后 BuildRequestURL 查注册表必然失败。
	// 同步侧 MediaKitSyncHelper 出于同一理由刻意不调本函数。
	if platform != constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMediaKit)) {
		if err := helper.ModelMappedHelper(c, info, nil); err != nil {
			return nil, service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest)
		}
	}

	// 3. 预生成公开 task ID（仅首次）
	if info.PublicTaskID == "" {
		info.PublicTaskID = model.GenerateTaskID()
	}

	// 4. 价格计算：基础模型价格
	info.OriginModelName = modelName
	priceData, err := helper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest)
	}
	info.PriceData = priceData

	// 5. 计费估算：让适配器根据用户请求提供 OtherRatios（时长、分辨率等）
	//    必须在 ModelPriceHelperPerCall 之后调用（它会重建 PriceData）。
	//    ResolveOriginTask 可能已在 remix 路径中预设了 OtherRatios，此处合并。
	if estimatedRatios := adaptor.EstimateBilling(c, info); len(estimatedRatios) > 0 {
		for k, v := range estimatedRatios {
			info.PriceData.AddOtherRatio(k, v)
		}
	}

	// 6. 将 OtherRatios 应用到基础额度（饱和转换，防止溢出成负数）
	if !common.StringsContains(constant.TaskPricePatches, modelName) {
		quotaWithRatios := info.PriceData.ApplyOtherRatiosToFloat(float64(info.PriceData.Quota))
		quota, clamp := common.QuotaFromFloatChecked(quotaWithRatios)
		info.PriceData.Quota = quota
		noteTaskQuotaClamp(info, clamp)
	}

	// 7. 预扣费（仅首次 — 重试时 info.Billing 已存在，跳过）
	if info.Billing == nil && !info.PriceData.FreeModel {
		info.ForcePreConsume = true
		if apiErr := service.PreConsumeBilling(c, info.PriceData.Quota, info); apiErr != nil {
			return nil, service.TaskErrorFromAPIError(apiErr)
		}
	}

	// 8. 构建请求体
	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
	}

	// 9. 发送请求
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, service.TaskErrorWrapper(fmt.Errorf("%s", string(responseBody)), "fail_to_fetch_task", resp.StatusCode)
	}

	// 10. 返回 OtherRatios 给下游（header 必须在 DoResponse 写 body 之前设置）
	otherRatios := info.PriceData.OtherRatios()
	if otherRatios == nil {
		otherRatios = map[string]float64{}
	}
	ratiosJSON, _ := common.Marshal(otherRatios)
	c.Header("X-New-Api-Other-Ratios", string(ratiosJSON))

	// 11. 解析响应
	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}

	// 11. 提交后计费调整：让适配器根据上游实际返回调整 OtherRatios
	finalQuota := info.PriceData.Quota
	if adjustedRatios := adaptor.AdjustBillingOnSubmit(info, taskData); len(adjustedRatios) > 0 {
		if adjustedQuota, ok := recalcQuotaFromRatios(info, adjustedRatios); ok {
			// 基于调整后的 ratios 重新计算 quota
			finalQuota = adjustedQuota
			info.PriceData.ReplaceOtherRatios(adjustedRatios)
			info.PriceData.Quota = finalQuota
		}
	}

	return &TaskSubmitResult{
		UpstreamTaskID: upstreamTaskID,
		TaskData:       taskData,
		Platform:       platform,
		Quota:          finalQuota,
	}, nil
}

// recalcQuotaFromRatios 根据 adjustedRatios 重新计算 quota。
// 公式: baseQuota × ∏(ratio) — 其中 baseQuota 是不含 OtherRatios 的基础额度。
func recalcQuotaFromRatios(info *relaycommon.RelayInfo, ratios map[string]float64) (int, bool) {
	// 从 PriceData 获取不含 OtherRatios 的基础价格
	baseQuota := info.PriceData.RemoveOtherRatiosFromFloat(float64(info.PriceData.Quota))
	priceData := info.PriceData
	if !priceData.ReplaceOtherRatios(ratios) {
		return 0, false
	}
	// 应用新的 ratios
	result := priceData.ApplyOtherRatiosToFloat(baseQuota)
	quota, clamp := common.QuotaFromFloatChecked(result)
	noteTaskQuotaClamp(info, clamp)
	return quota, true
}

// noteTaskQuotaClamp records the first quota saturation event onto the task's
// RelayInfo so LogTaskConsumption can surface it on the submit log's
// admin_info. First non-nil clamp wins.
func noteTaskQuotaClamp(info *relaycommon.RelayInfo, clamp *common.QuotaClamp) {
	if clamp == nil || info == nil {
		return
	}
	if info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
}

var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeSunoFetchByID:  sunoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeSunoFetch:      sunoFetchRespBodyBuilder,
	relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeMediaKitFetch:  mediaKitFetchRespBodyBuilder,
}

// mediaKitTaskStatus 把网关任务状态翻译回 MediaKit 的词汇。客户端的提交响应
// 是 MediaKit 信封，查询也应当用同一套状态词，否则客户端要写两套判断。
var mediaKitTaskStatus = map[model.TaskStatus]string{
	model.TaskStatusNotStart:   "pending",
	model.TaskStatusSubmitted:  "queued",
	model.TaskStatusQueued:     "queued",
	model.TaskStatusInProgress: "processing",
	model.TaskStatusSuccess:    "completed",
	model.TaskStatusFailure:    "failed",
}

// mediaKitFetchRespBodyBuilder 从本库 tasks 表读任务，不代理上游 /api/v1/tasks/。
// 客户端持有的是网关公开 task_xxx，直连上游会暴露上游 task_id 并允许越权枚举
// 他人任务。
func mediaKitFetchRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_id is required"), "task_id_required", http.StatusBadRequest)
		return
	}
	task, exist, err := model.GetByTaskId(c.GetInt("id"), taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	status := mediaKitTaskStatus[task.Status]
	if status == "" {
		status = "processing"
	}
	response := gin.H{
		"success": true,
		"task_id": task.TaskID,
		"status":  status,
		"message": task.FailReason,
	}
	// 回传上游原始 result 的结构化内容。网关的下游调用方需要产物本身——可能是
	// 数组（extract-frames 的 snapshots、drama-recap-vertical 的 video_urls）、
	// 嵌套字段，或根本没有 URL 的分析结果（asr-subtitles 的字幕、画质评分）。
	// 只给一个 url 字段表达不了这些形状。
	result := gin.H{}
	if len(task.Data) > 0 {
		var envelope struct {
			Result map[string]any `json:"result"`
		}
		if err := common.Unmarshal(task.Data, &envelope); err == nil && len(envelope.Result) > 0 {
			result = envelope.Result
		}
	}
	// url 只在任务成功且确实取到上游产物时附加，三条排除理由各自独立：
	//   - 失败的task.PublicResultURL()会回落到 FailReason（见 model/task.go GetResultURL），
	//     错误文案不是地址，放进 url 会被调用方当下载链接用。错误已由上面的 message 字段承载。
	//   - 自引用代理地址来自轮询层「没有产物」时的兜底写入（taskcommon.BuildProxyURL），
	//     不是产物，回传它会让调用方拿到一个指向网关自身的地址。
	if task.Status == model.TaskStatusSuccess {
		if resultURL := task.PublicResultURL(); resultURL != "" && !taskcommon.IsProxyURL(resultURL, task.TaskID) {
			result["url"] = resultURL
		}
	}
	if len(result) > 0 {
		response["result"] = result
	}

	respBody, err = common.Marshal(response)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

func RelayTaskFetch(c *gin.Context, relayMode int) (taskResp *dto.TaskError) {
	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		taskResp = service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
	}

	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	return
}

func sunoFetchRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	userId := c.GetInt("id")
	var condition = struct {
		IDs    []any  `json:"ids"`
		Action string `json:"action"`
	}{}
	err := c.BindJSON(&condition)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
		return
	}
	var tasks []any
	if len(condition.IDs) > 0 {
		taskModels, err := model.GetByTaskIds(userId, condition.IDs)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
		for _, task := range taskModels {
			tasks = append(tasks, TaskModel2Dto(task))
		}
	} else {
		tasks = make([]any, 0)
	}
	respBody, err = common.Marshal(dto.TaskResponse[[]any]{
		Code: "success",
		Data: tasks,
	})
	return
}

func sunoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("id")
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	// 火山原生协议：按 /api/v3 的形状回填任务对象
	if strings.HasPrefix(c.Request.URL.Path, constant.NativeArkVideoTaskPath) {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
			return
		}
		converter, ok := adaptor.(channel.NativeArkTaskConverter)
		if !ok {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", originTask.Platform), "not_implemented", http.StatusNotImplemented)
			return
		}
		respBody, err = converter.ConvertToNativeArkTask(originTask)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "convert_to_native_ark_task_failed", http.StatusInternalServerError)
		}
		return
	}

	isOpenAIVideoAPI := strings.HasPrefix(c.Request.RequestURI, "/v1/videos/")

	// Gemini/Vertex 支持实时查询：用户 fetch 时直接从上游拉取最新状态
	if realtimeResp := tryRealtimeFetch(originTask, isOpenAIVideoAPI); len(realtimeResp) > 0 {
		respBody = realtimeResp
		return
	}

	// OpenAI Video API 格式: 走各 adaptor 的 ConvertToOpenAIVideo
	if isOpenAIVideoAPI {
		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
			return
		}
		if converter, ok := adaptor.(channel.OpenAIVideoConverter); ok {
			openAIVideoData, err := converter.ConvertToOpenAIVideo(originTask)
			if err != nil {
				taskResp = service.TaskErrorWrapper(err, "convert_to_openai_video_failed", http.StatusInternalServerError)
				return
			}
			respBody = openAIVideoData
			return
		}
		taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", originTask.Platform), "not_implemented", http.StatusNotImplemented)
		return
	}

	// 通用 TaskDto 格式
	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

// tryRealtimeFetch 尝试从上游实时拉取 Gemini/Vertex 任务状态。
// 仅当渠道类型为 Gemini 或 Vertex 时触发；其他渠道或出错时返回 nil。
// 当非 OpenAI Video API 时，还会构建自定义格式的响应体。
func tryRealtimeFetch(task *model.Task, isOpenAIVideoAPI bool) []byte {
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil
	}
	if channelModel.Type != constant.ChannelTypeVertexAi && channelModel.Type != constant.ChannelTypeGemini {
		return nil
	}

	baseURL := constant.ChannelBaseURLs[channelModel.Type]
	if channelModel.GetBaseURL() != "" {
		baseURL = channelModel.GetBaseURL()
	}
	proxy := channelModel.GetSetting().Proxy
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
	if adaptor == nil {
		return nil
	}

	resp, err := adaptor.FetchTask(baseURL, channelModel.Key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, proxy)
	if err != nil || resp == nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	ti, err := adaptor.ParseTaskResult(body)
	if err != nil || ti == nil {
		return nil
	}

	snap := task.Snapshot()

	// 将上游最新状态更新到 task
	if ti.Status != "" {
		task.Status = model.TaskStatus(ti.Status)
	}
	if ti.Progress != "" {
		task.Progress = ti.Progress
	}
	if strings.HasPrefix(ti.Url, "data:") {
		// data: URI — kept in Data, not ResultURL
	} else if ti.Url != "" {
		task.PrivateData.ResultURL = ti.Url
	} else if task.Status == model.TaskStatusSuccess {
		// No URL from adaptor — construct proxy URL using public task ID
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	}

	if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	// OpenAI Video API 由调用者的 ConvertToOpenAIVideo 分支处理
	if isOpenAIVideoAPI {
		return nil
	}

	// 非 OpenAI Video API: 构建自定义格式响应
	format := detectVideoFormat(body)
	out := map[string]any{
		"error":    nil,
		"format":   format,
		"metadata": nil,
		"status":   mapTaskStatusToSimple(task.Status),
		"task_id":  task.TaskID,
		"url":      task.PublicResultURL(),
	}
	respBody, _ := common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: out,
	})
	return respBody
}

// detectVideoFormat 从 Gemini/Vertex 原始响应中探测视频格式
func detectVideoFormat(rawBody []byte) string {
	var raw map[string]any
	if err := common.Unmarshal(rawBody, &raw); err != nil {
		return "mp4"
	}
	respObj, ok := raw["response"].(map[string]any)
	if !ok {
		return "mp4"
	}
	vids, ok := respObj["videos"].([]any)
	if !ok || len(vids) == 0 {
		return "mp4"
	}
	v0, ok := vids[0].(map[string]any)
	if !ok {
		return "mp4"
	}
	mt, ok := v0["mimeType"].(string)
	if !ok || mt == "" || strings.Contains(mt, "mp4") {
		return "mp4"
	}
	return mt
}

// mapTaskStatusToSimple 将内部 TaskStatus 映射为简化状态字符串
func mapTaskStatusToSimple(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	result := &dto.TaskDto{
		ID:         task.ID,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		TaskID:     task.TaskID,
		Platform:   string(task.Platform),
		UserId:     task.UserId,
		Group:      task.Group,
		ChannelId:  task.ChannelId,
		Quota:      task.Quota,
		Action:     task.Action,
		Status:     string(task.Status),
		FailReason: task.FailReason,
		ResultURL:  task.PublicResultURL(),
		SubmitTime: task.SubmitTime,
		StartTime:  task.StartTime,
		FinishTime: task.FinishTime,
		Progress:   task.Progress,
		Properties: task.Properties,
		Username:   task.Username,
		Data:       task.PublicData(),
	}
	// 真实消耗 token：按平台从 task.Data 提取（可选接口，未实现平台不返回）
	if adaptor := GetTaskAdaptor(task.Platform); adaptor != nil {
		if ue, ok := adaptor.(channel.TaskUsageExtractor); ok {
			result.Usage = ue.ExtractTaskUsage(task)
		}
	}
	return result
}

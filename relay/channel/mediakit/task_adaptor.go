package mediakit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// submitResponse 是异步工具的提交响应信封。task_id 是上游任务 ID，
// 只落库到 private_data，对客返回的是网关公开任务 ID。
type submitResponse struct {
	Success bool   `json:"success"`
	TaskID  string `json:"task_id"`
	Message string `json:"message"`
}

// queryResponse 是异步任务的查询响应。result 的键名因工具而异
// （image_url / video_url / audio_url ...），故按 map 解析后按产物类型取。
type queryResponse struct {
	Success bool           `json:"success"`
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Result  map[string]any `json:"result"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling

	// insertMediaKitCost 覆盖审计行落库，供测试注入桩；为 nil 时走 model.InsertMediaKitCost。
	insertMediaKitCost func(*model.MediaKitCost) error
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {}

// ValidateRequestAndSetAction 校验工具名与请求体。不复用
// relaycommon.ValidateBasicTaskRequest——它要求 prompt 非空，而 MediaKit
// 原生协议没有 prompt 字段（入参是 video_url / image_url 之类）。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	spec, ok := ToolForModel(info.OriginModelName)
	if !ok {
		return service.TaskErrorWrapper(fmt.Errorf("unknown mediakit tool: %s", info.OriginModelName), "unknown_tool", http.StatusBadRequest)
	}
	if spec.Sync {
		return service.TaskErrorWrapper(fmt.Errorf("mediakit tool %s is synchronous, use POST /v1/mediakit/sync/%s", spec.Name, spec.Name), "invalid_tool", http.StatusBadRequest)
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_request_body_failed", http.StatusBadRequest)
	}
	raw, err := storage.Bytes()
	if err != nil {
		return service.TaskErrorWrapper(err, "read_request_body_failed", http.StatusBadRequest)
	}
	var body map[string]any
	if err := common.Unmarshal(raw, &body); err != nil {
		return service.TaskErrorWrapper(fmt.Errorf("request body must be a JSON object: %w", err), "invalid_request_body", http.StatusBadRequest)
	}

	info.Action = spec.Name
	return nil
}

// RatioKeyMinutes 是 EstimateBilling 返回的时长倍率键。结算时要用真实分钟数
// 替换它，故两处必须用同一个键名。
const RatioKeyMinutes = "minutes"

// EstimateBilling 时长类工具按预估分钟预扣。MediaKit 多数工具的入参是 URL，
// 提交时拿不到真实时长，故用渠道配置的默认预估分钟；真实时长在任务完成时
// 由 AdjustBillingOnComplete 结算差额。按次/按量工具不需要倍率，返回 nil。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	spec, ok := ToolForModel(info.OriginModelName)
	if !ok || spec.Mode != BillingDuration {
		return nil
	}
	// 时长类的价格配在 model_price 上（元/分钟），会被判成按次计费，从而跳过
	// 完成时的差额结算。这里显式 opt-in 打开结算——是否配了默认预估分钟无关，
	// 没配也要结算，否则真实时长永远不参与计价。
	if info.TaskRelayInfo != nil {
		info.TaskRelayInfo.DurationSettlement = true
	}
	minutes := info.ChannelSetting.MediaKitDefaultMinutes
	if minutes <= 0 {
		return nil
	}
	return map[string]float64{RatioKeyMinutes: minutes}
}

// AdjustBillingOnComplete 用上游回包里的真实时长重算额度，返回正数即由
// service.RecalculateTaskQuota 结算差额（正补负退）。
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	quota, _ := a.AdjustBillingOnCompleteWithClamp(task, taskResult)
	return quota
}

// AdjustBillingOnCompleteWithClamp 是 AdjustBillingOnComplete 的带审计版本：
// 额度换算发生饱和时把 QuotaClamp 一起交回轮询层，由它写进结算日志的
// admin_info.quota_saturation。做成纯函数而非在适配器实例上暂存，是因为适配器
// 可能被并发任务共用。
//
// 只在成功的任务上结算：失败任务已由轮询层全额退预扣，再算一次会重复。
// 拿不到真实时长（字段未命中或值非法）时返回 0，保持预扣不变——宁可少结算，
// 不用猜测的时长收费。
func (a *TaskAdaptor) AdjustBillingOnCompleteWithClamp(task *model.Task, taskResult *relaycommon.TaskInfo) (int, *common.QuotaClamp) {
	if task == nil || taskResult == nil || taskResult.Status != model.TaskStatusSuccess {
		return 0, nil
	}
	bc := task.PrivateData.BillingContext
	if bc == nil || !bc.DurationSettlement || bc.ModelPrice <= 0 {
		return 0, nil
	}

	seconds := TaskDurationSeconds(task.Data)
	if seconds <= 0 {
		// 该函数签名没有 ctx，拿不到 trace_id，只能走 SysError；这行日志是
		// 核对 TaskDurationSeconds 候选字段是否命中真实上游的主要手段。
		common.SysError(fmt.Sprintf(
			"mediakit 任务 %s 未取到真实时长，保持预扣额度不变；result 键：%s",
			task.TaskID, ResultKeys(task.Data)))
		return 0, nil
	}

	// 真实分钟数替换掉预估的 minutes 倍率，其余倍率（分辨率档等）照常相乘。
	multiplier := seconds / 60
	for name, ratio := range bc.OtherRatios {
		if name == RatioKeyMinutes {
			continue
		}
		multiplier *= ratio
	}

	return common.QuotaRoundChecked(bc.ModelPrice * common.QuotaPerUnit * bc.GroupRatio * multiplier)
}

// RecordTaskCost 实现 service.TaskCostRecorder：任务到达终态时落一条审计行。
//
// 所有字段都从 task 自身推出，不需要额外透传：工具名在 Action（提交时由
// ValidateRequestAndSetAction 写死为上游 slug），预估分钟在 BillingContext 的
// minutes 倍率里，真实时长在轮询写入的 Data 里，实扣额度由轮询层算好传入。
//
// 只记不发：写失败仅告警，绝不影响已经完成的结算与退款。
func (a *TaskAdaptor) RecordTaskCost(ctx context.Context, task *model.Task, chargedQuota int) {
	if task == nil {
		return
	}
	spec, ok := LookupTool(task.Action)
	if !ok {
		// 工具名不在注册表里说明这不是本渠道的任务（或注册表已下线该工具），
		// 与其写一行来路不明的审计记录，不如不写。
		return
	}

	cost := &model.MediaKitCost{
		TaskId:        task.TaskID,
		ChannelId:     task.ChannelId,
		Tool:          spec.Name,
		BillingMode:   string(spec.Mode),
		UpstreamJobId: task.GetUpstreamTaskID(),
		Status:        string(task.Status),
		ActualSeconds: TaskDurationSeconds(task.Data),
		ChargedQuota:  chargedQuota,
	}
	if bc := task.PrivateData.BillingContext; bc != nil {
		cost.EstimatedMinutes = bc.OtherRatios[RatioKeyMinutes]
	}

	insert := a.insertMediaKitCost
	if insert == nil {
		insert = model.InsertMediaKitCost
	}
	if err := insert(cost); err != nil {
		logger.LogError(ctx, fmt.Sprintf("insert mediakit_cost failed task %s: %s", task.TaskID, err.Error()))
	}
}

// BuildRequestURL 由注册表给出上游路径，客户端工具名不参与拼接。
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	spec, ok := ToolForModel(info.UpstreamModelName)
	if !ok {
		return "", fmt.Errorf("unknown mediakit tool: %s", info.UpstreamModelName)
	}
	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return strings.TrimSuffix(baseURL, "/") + spec.Path, nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// BuildRequestBody 完全透传：MediaKit 请求体里没有 model 字段，网关一个字都不改写。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body storage: %w", err)
	}
	return common.ReaderOnly(storage), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 上游以 HTTP 200 + success=false 表达业务失败，必须在此拦下：
// relay_task.go 只拦非 200 响应码。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", nil, service.TaskErrorWrapper(readErr, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var submit submitResponse
	if err := common.Unmarshal(responseBody, &submit); err != nil {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("unmarshal_response_body_failed: %w", err), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if !submit.Success || submit.TaskID == "" {
		message := submit.Message
		if message == "" {
			message = "submit failed"
		}
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("mediakit submit failed: %s", message), "invalid_response", http.StatusBadGateway)
	}

	// 对客沿用 MediaKit 信封，但只暴露网关公开任务 ID。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"task_id": info.PublicTaskID,
		"message": "",
	})
	return submit.TaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	uri := strings.TrimSuffix(baseUrl, "/") + queryPathPrefix + taskID

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var res queryResponse
	if err := common.Unmarshal(respBody, &res); err != nil {
		return nil, fmt.Errorf("unmarshal task result failed: %w", err)
	}

	taskResult := relaycommon.TaskInfo{Code: 0}
	switch strings.ToLower(res.Status) {
	case "pending", "queued", "submitted":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "completed", "succeeded", "success":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = ResultURL(res.Result)
	case "failed", "error", "canceled", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = res.Message
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}
	return &taskResult, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return GetModelList()
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

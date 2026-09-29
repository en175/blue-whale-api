package volcengine_plus

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskdoubao "github.com/QuantumNous/new-api/relay/channel/task/doubao"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type TaskAdaptor struct {
	taskdoubao.TaskAdaptor
	randIntn          func(int) int
	mediaKit          mediaKitAPI
	loadChannel       func(int) (*model.Channel, error)
	insertEnhanceCost func(*model.EnhanceCost) error
}

var _ channel.TaskEnhancer = (*TaskAdaptor)(nil)

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.TaskAdaptor.Init(info)
	if a.loadChannel == nil {
		a.loadChannel = model.CacheGetChannel
	}
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

// ValidateRequestAndSetAction 在 doubao 的解析之上补三条本渠道专有守卫。
// 全部放在这里而不是 BuildRequestBody：这一层跑在预扣费之前，且能自定义错误码；
// BuildRequestBody 的报错会被 relay_task 一律包成 500，而且在预扣费之后。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if info != nil && info.TaskRelayInfo != nil && info.Action == constant.TaskActionRemix {
		return service.TaskErrorWrapperLocal(fmt.Errorf("remix is not supported"), "remix_not_supported", http.StatusBadRequest)
	}
	if taskErr := a.TaskAdaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}
	// 样片（metadata.draft / 顶层 draft）在 doubao 适配器上零把关透传：上游会出
	// 480p 样片，但本渠道的对客档位与扣费按请求档写死，剥皮又因未降档而跳过，
	// 结果是「按 1080p 扣费、回 1080p、交付 480p 样片」。见 ADR 0019 背景 1。
	if taskcommon.IsDraftRequest(&req) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("draft task is not supported"), "draft_not_supported", http.StatusBadRequest)
	}
	ratio, _ := req.Metadata["ratio"].(string)
	if !KnownRatio(ratio) {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported ratio: %s", strings.TrimSpace(ratio)), "invalid_ratio", http.StatusBadRequest)
	}
	// 档位超出模型能力时提前拒绝，而不是等上游报错再全额退预扣。
	modelName := ""
	if info != nil {
		modelName = info.OriginModelName
	}
	if modelName == "" {
		modelName = req.Model
	}
	resolution, _ := req.Metadata["resolution"].(string)
	resolution = NormalizeResolution(resolution)
	if !ResolutionSupported(modelName, resolution) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("model %s does not support resolution %s", modelName, resolution),
			"invalid_resolution", http.StatusBadRequest)
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	requested, _ := req.Metadata["resolution"].(string)
	requested = NormalizeResolution(requested)
	ratio, _ := req.Metadata["ratio"].(string)
	info.RequestedResolution = requested
	info.RequestedRatio = NormalizeRatio(ratio)
	if sec, convErr := strconv.Atoi(req.Seconds); convErr == nil && sec > 0 {
		info.OutputDurationMs = int64(sec) * 1000
	}

	cfg := LotteryConfig{}
	if info.ChannelMeta != nil {
		cfg = LotteryConfigFromSettings(info.ChannelSetting)
	}
	upstream := drawUpstreamResolution(requested, cfg, a.randIntn)
	if upstream == "" {
		upstream = requested
	}
	info.UpstreamResolution = upstream

	hasVideo := taskcommon.HasVideoInputInMetadata(req.Metadata)
	a.captureChannelOriginalQuota(info, requested, upstream, hasVideo)

	body, err := a.TaskAdaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, err
	}
	if !IsDowngrade(requested, upstream) {
		return body, nil
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := common.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	payload["resolution"] = upstream
	out, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(out), nil
}

func (a *TaskAdaptor) captureChannelOriginalQuota(info *relaycommon.RelayInfo, requested, upstream string, hasVideo bool) {
	if !IsDowngrade(requested, upstream) {
		return
	}
	info.ChannelCostUsesUpstreamTier = true
	reqRatio := 1.0
	if other := info.PriceData.OtherRatios(); other != nil {
		if r := other["video_input"]; r > 0 {
			reqRatio = r
		}
	}
	upRatio := reqRatio
	if r, ok := taskcommon.ResolveVideoInputRatio(info.UpstreamModelName, upstream, hasVideo); ok && r > 0 {
		upRatio = r
	} else if r, ok := taskcommon.ResolveVideoInputRatio(info.OriginModelName, upstream, hasVideo); ok && r > 0 {
		upRatio = r
	}
	info.UpstreamVideoInputRatio = upRatio
	group := info.PriceData.GroupRatioInfo.GroupRatio
	if group <= 0 {
		group = 1
	}
	if reqRatio <= 0 {
		reqRatio = 1
	}
	originalRequested := float64(info.PriceData.Quota) / group
	originalUpstream, clamp := common.QuotaFromFloatChecked(originalRequested * upRatio / reqRatio)
	if clamp != nil && info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
	info.ChannelOriginalQuota = originalUpstream
}

func (a *TaskAdaptor) ExtractTaskUsage(task *model.Task) *dto.TaskUsage {
	if task != nil && IsDowngrade(task.PrivateData.RequestedResolution, task.PrivateData.UpstreamResolution) {
		peeled := peeledTokensForTask(task)
		if peeled > 0 {
			return &dto.TaskUsage{CompletionTokens: peeled, TotalTokens: peeled}
		}
	}
	return a.TaskAdaptor.ExtractTaskUsage(task)
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	if shouldRewriteUserVisible(originTask) {
		url := ""
		done := originTask.Status == model.TaskStatusSuccess
		failed := originTask.Status == model.TaskStatusFailure
		if done {
			url = originTask.PublicResultURL()
		}
		applyUserVisibleRewrite(originTask, url, done, failed)
	}
	return a.TaskAdaptor.ConvertToOpenAIVideo(originTask)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	id, data, taskErr := a.TaskAdaptor.DoResponse(c, resp, info)
	if taskErr != nil || info == nil || info.TaskRelayInfo == nil {
		return id, data, taskErr
	}
	if IsDowngrade(info.RequestedResolution, info.UpstreamResolution) {
		data = rewriteUserVisibleTaskData(data, info.RequestedResolution, 0, 0, "", true, false)
	}
	return id, data, taskErr
}

func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	if task == nil || taskResult == nil {
		return 0
	}
	if !IsDowngrade(task.PrivateData.RequestedResolution, task.PrivateData.UpstreamResolution) {
		return 0
	}
	if task.PrivateData.UpstreamTotalTokens == 0 && taskResult.TotalTokens > 0 {
		task.PrivateData.UpstreamCompletionTokens = taskResult.CompletionTokens
		task.PrivateData.UpstreamTotalTokens = taskResult.TotalTokens
	}
	peeled := peeledTokensForTask(task)
	if peeled > 0 {
		taskResult.CompletionTokens = peeled
		taskResult.TotalTokens = peeled
	}
	a.settleChannelOriginal(task)
	return 0
}

func (a *TaskAdaptor) settleChannelOriginal(task *model.Task) {
	if task == nil || !task.PrivateData.ChannelCostUsesUpstreamTier {
		return
	}
	bc := task.PrivateData.BillingContext
	if bc == nil || bc.ModelRatio <= 0 {
		return
	}
	upstreamTokens := task.PrivateData.UpstreamTotalTokens
	if upstreamTokens <= 0 {
		return
	}
	upRatio := task.PrivateData.UpstreamVideoInputRatio
	if upRatio <= 0 {
		upRatio = 1
	}
	actual, _ := common.QuotaFromFloatChecked(float64(upstreamTokens) * bc.ModelRatio * upRatio)
	already := task.PrivateData.ChannelOriginalQuotaDeducted
	if actual > already {
		service.DeductChannelQuota(task.ChannelId, actual-already, 1)
		task.PrivateData.ChannelOriginalQuotaDeducted = actual
	}
}

package volcengine_plus

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/mediakit"
)

type mediaKitAPI interface {
	Submit(ctx context.Context, videoURL, resolution string) (jobID string, err error)
	Get(ctx context.Context, jobID string) (status, videoURL string, err error)
}

// mediaKitClient 是超分专用的薄封装：固定提交路径、固定请求体与结果字段。
// HTTP 与错误信封由 mediakit.Client 承担。
type mediaKitClient struct {
	inner      *mediakit.Client
	submitPath string
}

func newMediaKitClient(baseURL, apiKey, proxy, submitPath string) *mediaKitClient {
	submitPath = strings.TrimSpace(submitPath)
	if submitPath == "" {
		submitPath = DefaultMediaKitSubmitPath
	} else if !strings.HasPrefix(submitPath, "/") {
		submitPath = "/" + submitPath
	}
	return &mediaKitClient{
		inner:      mediakit.NewClient(baseURL, apiKey, proxy),
		submitPath: submitPath,
	}
}

func (c *mediaKitClient) Submit(ctx context.Context, videoURL, resolution string) (string, error) {
	body, err := common.Marshal(map[string]string{
		"video_url":  videoURL,
		"resolution": resolution,
	})
	if err != nil {
		return "", err
	}
	raw, err := c.inner.Post(ctx, c.submitPath, body)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Success bool   `json:"success"`
		TaskID  string `json:"task_id"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.TaskID == "" {
		return "", fmt.Errorf("mediakit submit missing task_id")
	}
	return parsed.TaskID, nil
}

func (c *mediaKitClient) Get(ctx context.Context, jobID string) (string, string, error) {
	raw, err := c.inner.Get(ctx, mediakit.TaskPath(jobID))
	if err != nil {
		return "", "", err
	}
	var parsed struct {
		Success bool   `json:"success"`
		Status  string `json:"status"`
		Result  struct {
			VideoURL string  `json:"video_url"`
			Duration float64 `json:"duration"`
		} `json:"result"`
	}
	if err := common.Unmarshal(raw, &parsed); err != nil {
		return "", "", err
	}
	return parsed.Status, parsed.Result.VideoURL, nil
}

func (a *TaskAdaptor) mediaKitForTask(task *model.Task) (mediaKitAPI, error) {
	if a.mediaKit != nil {
		return a.mediaKit, nil
	}
	ch, err := a.loadChannel(task.ChannelId)
	if err != nil {
		return nil, err
	}
	setting := ch.GetSetting()
	if strings.TrimSpace(setting.MediaKitAPIKey) == "" {
		return nil, fmt.Errorf("mediakit_api_key is required")
	}
	return newMediaKitClient(setting.MediaKitBaseURL, setting.MediaKitAPIKey, setting.Proxy, setting.MediaKitSubmitPath), nil
}

func (a *TaskAdaptor) recordEnhanceCost(ctx context.Context, task *model.Task, jobID, status string, amountFen int) {
	cost := &model.EnhanceCost{
		TaskId:              task.TaskID,
		ChannelId:           task.ChannelId,
		RequestedResolution: task.PrivateData.RequestedResolution,
		UpstreamResolution:  task.PrivateData.UpstreamResolution,
		OutputDurationMs:    task.PrivateData.OutputDurationMs,
		Fps:                 task.PrivateData.SourceFps,
		RateTier:            RateTierForRequested(task.PrivateData.RequestedResolution),
		AmountFen:           amountFen,
		MediaKitJobId:       jobID,
		Status:              status,
	}
	insert := a.insertEnhanceCost
	if insert == nil {
		insert = model.InsertEnhanceCost
	}
	if err := insert(cost); err != nil {
		logger.LogError(ctx, fmt.Sprintf("insert enhance_cost failed task %s: %s", task.TaskID, err.Error()))
	}
}

func (a *TaskAdaptor) submitEnhance(ctx context.Context, task *model.Task, client mediaKitAPI) error {
	resolution := NormalizeResolution(task.PrivateData.RequestedResolution)
	if resolution != Res720p && resolution != Res1080p {
		return fmt.Errorf("mediakit resolution must be 720p or 1080p")
	}
	sourceURL := task.PrivateData.EnhanceSourceURL
	if sourceURL == "" {
		return fmt.Errorf("missing enhance source url")
	}
	task.PrivateData.EnhanceAttempt++
	jobID, err := client.Submit(ctx, sourceURL, resolution)
	if err != nil {
		a.recordEnhanceCost(ctx, task, "", EnhanceStatusFailed, 0)
		return err
	}
	task.PrivateData.EnhanceJobID = jobID
	task.PrivateData.EnhanceStatus = EnhanceStatusPending
	return nil
}

// EnsureEnhanced runs MediaKit 超分 after Seedance succeeds at a lower tier.
// done=false means keep the task IN_PROGRESS this poll.
func (a *TaskAdaptor) EnsureEnhanced(ctx context.Context, task *model.Task, upstreamResultURL string) (string, bool, error) {
	if task == nil {
		return upstreamResultURL, true, nil
	}
	captureSeedanceMeta(task)
	requested := task.PrivateData.RequestedResolution
	upstream := task.PrivateData.UpstreamResolution
	if requested == "" || upstream == "" || !IsDowngrade(requested, upstream) {
		task.PrivateData.EnhanceStatus = EnhanceStatusSkipped
		return a.finishEnhance(task, upstreamResultURL, true, nil)
	}
	if task.PrivateData.EnhanceStatus == EnhanceStatusSucceeded && task.GetResultURL() != "" {
		return a.finishEnhance(task, task.GetResultURL(), true, nil)
	}
	if task.PrivateData.EnhanceSourceURL == "" && upstreamResultURL != "" {
		task.PrivateData.EnhanceSourceURL = upstreamResultURL
	}
	if task.PrivateData.EnhanceStatus == EnhanceStatusFailed {
		return a.finishEnhance(task, "", true, fmt.Errorf("%s", genericEnhanceFailReason))
	}

	client, err := a.mediaKitForTask(task)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("mediakit client task %s: %s", task.TaskID, err.Error()))
		task.PrivateData.EnhanceAttempt = MaxEnhanceAttempts
		task.PrivateData.EnhanceStatus = EnhanceStatusFailed
		a.recordEnhanceCost(ctx, task, "", EnhanceStatusFailed, 0)
		return a.finishEnhance(task, "", true, fmt.Errorf("%s", genericEnhanceFailReason))
	}

	if task.PrivateData.EnhanceJobID == "" {
		if task.PrivateData.EnhanceAttempt >= MaxEnhanceAttempts {
			task.PrivateData.EnhanceStatus = EnhanceStatusFailed
			a.recordEnhanceCost(ctx, task, "", EnhanceStatusFailed, 0)
			return a.finishEnhance(task, "", true, fmt.Errorf("%s", genericEnhanceFailReason))
		}
		if err := a.submitEnhance(ctx, task, client); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("mediakit submit failed task %s attempt %d: %s", task.TaskID, task.PrivateData.EnhanceAttempt, err.Error()))
			if task.PrivateData.EnhanceAttempt >= MaxEnhanceAttempts {
				task.PrivateData.EnhanceStatus = EnhanceStatusFailed
				return a.finishEnhance(task, "", true, fmt.Errorf("%s", genericEnhanceFailReason))
			}
			return a.finishEnhance(task, "", false, nil)
		}
		return a.finishEnhance(task, "", false, nil)
	}

	status, videoURL, err := client.Get(ctx, task.PrivateData.EnhanceJobID)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("mediakit get failed task %s job %s: %s", task.TaskID, task.PrivateData.EnhanceJobID, err.Error()))
		return a.finishEnhance(task, "", false, nil)
	}
	switch strings.ToLower(status) {
	case "completed", "succeeded", "success":
		if videoURL == "" {
			logger.LogWarn(ctx, fmt.Sprintf("mediakit completed without url task %s", task.TaskID))
			return a.finishOrRetry(ctx, task, client)
		}
		amount := EnhanceAmountFen(task.PrivateData.OutputDurationMs, task.PrivateData.SourceFps, RateTierForRequested(requested))
		a.recordEnhanceCost(ctx, task, task.PrivateData.EnhanceJobID, EnhanceStatusSucceeded, amount)
		task.PrivateData.EnhanceStatus = EnhanceStatusSucceeded
		return a.finishEnhance(task, videoURL, true, nil)
	case "failed", "error", "canceled", "cancelled":
		a.recordEnhanceCost(ctx, task, task.PrivateData.EnhanceJobID, EnhanceStatusFailed, 0)
		task.PrivateData.EnhanceJobID = ""
		return a.finishOrRetry(ctx, task, client)
	default:
		return a.finishEnhance(task, "", false, nil)
	}
}

func (a *TaskAdaptor) finishOrRetry(ctx context.Context, task *model.Task, client mediaKitAPI) (string, bool, error) {
	url, done, err := a.failOrRetryEnhance(ctx, task, client)
	return a.finishEnhance(task, url, done, err)
}

func (a *TaskAdaptor) finishEnhance(task *model.Task, url string, done bool, err error) (string, bool, error) {
	visibleURL := ""
	if err == nil && done && task != nil {
		visibleURL = task.PublicResultURL()
	}
	applyUserVisibleRewrite(task, visibleURL, done && err == nil, err != nil)
	return url, done, err
}

func (a *TaskAdaptor) failOrRetryEnhance(ctx context.Context, task *model.Task, client mediaKitAPI) (string, bool, error) {
	if task.PrivateData.EnhanceAttempt >= MaxEnhanceAttempts {
		task.PrivateData.EnhanceStatus = EnhanceStatusFailed
		return "", true, fmt.Errorf("%s", genericEnhanceFailReason)
	}
	if err := a.submitEnhance(ctx, task, client); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("mediakit retry submit failed task %s attempt %d: %s", task.TaskID, task.PrivateData.EnhanceAttempt, err.Error()))
		if task.PrivateData.EnhanceAttempt >= MaxEnhanceAttempts {
			task.PrivateData.EnhanceStatus = EnhanceStatusFailed
			return "", true, fmt.Errorf("%s", genericEnhanceFailReason)
		}
	}
	return "", false, nil
}

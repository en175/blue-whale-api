package volcengine_plus

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type seedanceSnapshot struct {
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	FramesPerSecond int    `json:"framespersecond"`
	Ratio           string `json:"ratio"`
	Model           string `json:"model"`
	Usage           struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func captureSeedanceMeta(task *model.Task) {
	if task == nil || len(task.Data) == 0 {
		return
	}
	var snap seedanceSnapshot
	if err := common.Unmarshal(task.Data, &snap); err != nil {
		return
	}
	if task.PrivateData.UpstreamTotalTokens == 0 && snap.Usage.TotalTokens > 0 {
		task.PrivateData.UpstreamCompletionTokens = snap.Usage.CompletionTokens
		task.PrivateData.UpstreamTotalTokens = snap.Usage.TotalTokens
	}
	if task.PrivateData.OutputDurationMs == 0 && snap.Duration > 0 {
		task.PrivateData.OutputDurationMs = int64(snap.Duration) * 1000
	}
	if task.PrivateData.SourceFps == 0 {
		if snap.FramesPerSecond > 0 {
			task.PrivateData.SourceFps = snap.FramesPerSecond
		} else {
			task.PrivateData.SourceFps = defaultEnhanceFPS
		}
	}
	if task.PrivateData.RequestedRatio == "" {
		task.PrivateData.RequestedRatio = NormalizeRatio(snap.Ratio)
	}
	if task.PrivateData.OriginModelName == "" && snap.Model != "" {
		task.PrivateData.OriginModelName = snap.Model
	}
}

func peeledTokensForTask(task *model.Task) int {
	if task == nil {
		return 0
	}
	upstream := task.PrivateData.UpstreamTotalTokens
	if upstream <= 0 {
		upstream = task.PrivateData.UpstreamCompletionTokens
	}
	if upstream <= 0 {
		return 0
	}
	if task.PrivateData.PeeledTotalTokens > 0 {
		return task.PrivateData.PeeledTotalTokens
	}
	requested := task.PrivateData.RequestedResolution
	upstreamRes := task.PrivateData.UpstreamResolution
	if requested == "" || upstreamRes == "" || !IsDowngrade(requested, upstreamRes) {
		return upstream
	}
	modelName := task.PrivateData.OriginModelName
	if modelName == "" && task.PrivateData.BillingContext != nil {
		modelName = task.PrivateData.BillingContext.OriginModelName
	}
	if modelName == "" {
		modelName = task.Properties.OriginModelName
	}
	peeled, err := PeelTokensForTiers(upstream, modelName, requested, upstreamRes, task.PrivateData.RequestedRatio)
	if err != nil {
		return 0
	}
	task.PrivateData.PeeledTotalTokens = peeled
	return peeled
}

// rewriteUserVisibleTaskData hides upstream-tier resolution, tokens, and URLs
// from the stored task JSON that user APIs serialize.
func rewriteUserVisibleTaskData(raw []byte, requested string, peeledCompletion, peeledTotal int, videoURL string, hideSucceeded, markFailed bool) []byte {
	if len(raw) == 0 {
		return raw
	}
	var m map[string]any
	if err := common.Unmarshal(raw, &m); err != nil {
		return raw
	}
	if requested != "" {
		m["resolution"] = requested
	}
	if usage, ok := m["usage"].(map[string]any); ok {
		if peeledTotal > 0 {
			usage["completion_tokens"] = peeledCompletion
			usage["total_tokens"] = peeledTotal
		}
	}
	if content, ok := m["content"].(map[string]any); ok {
		if videoURL == "" {
			delete(content, "video_url")
		} else {
			content["video_url"] = videoURL
		}
	}
	if markFailed {
		m["status"] = "failed"
	} else if hideSucceeded {
		if status, _ := m["status"].(string); status == "succeeded" {
			m["status"] = "running"
		}
	}
	out, err := common.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

func applyUserVisibleRewrite(task *model.Task, videoURL string, done, failed bool) {
	if task == nil || !IsDowngrade(task.PrivateData.RequestedResolution, task.PrivateData.UpstreamResolution) {
		return
	}
	peeled := peeledTokensForTask(task)
	task.Data = rewriteUserVisibleTaskData(task.Data, task.PrivateData.RequestedResolution, peeled, peeled, videoURL, !done && !failed, failed)
}

func (a *TaskAdaptor) RedactUserVisible(task *model.Task) {
	applyUserVisibleRewrite(task, "", false, false)
}

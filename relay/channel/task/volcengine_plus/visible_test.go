package volcengine_plus

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteUserVisibleTaskDataHidesUpstream(t *testing.T) {
	raw := []byte(`{"status":"succeeded","resolution":"720p","content":{"video_url":"https://up.example/720.mp4"},"usage":{"completion_tokens":108000,"total_tokens":108000}}`)
	got := rewriteUserVisibleTaskData(raw, Res1080p, 243000, 243000, "", true, false)

	var m map[string]any
	require.NoError(t, common.Unmarshal(got, &m))
	assert.Equal(t, Res1080p, m["resolution"])
	assert.Equal(t, "running", m["status"])
	content := m["content"].(map[string]any)
	_, hasURL := content["video_url"]
	assert.False(t, hasURL)
	usage := m["usage"].(map[string]any)
	assert.Equal(t, float64(243000), usage["total_tokens"])
	assert.NotContains(t, string(got), "720p")
	assert.NotContains(t, string(got), "108000")
	assert.NotContains(t, string(got), "https://up.example/720.mp4")
}

func TestRewriteUserVisibleTaskDataSuccessURL(t *testing.T) {
	raw := []byte(`{"status":"succeeded","resolution":"720p","content":{"video_url":"https://up.example/720.mp4"},"usage":{"completion_tokens":108000,"total_tokens":108000}}`)
	got := rewriteUserVisibleTaskData(raw, Res1080p, 243000, 243000, "https://mk.example/1080.mp4", false, false)

	var m map[string]any
	require.NoError(t, common.Unmarshal(got, &m))
	assert.Equal(t, "succeeded", m["status"])
	content := m["content"].(map[string]any)
	assert.Equal(t, "https://mk.example/1080.mp4", content["video_url"])
}

func TestPeeledTokensForTask(t *testing.T) {
	task := &model.Task{
		PrivateData: model.TaskPrivateData{
			RequestedResolution: Res1080p,
			UpstreamResolution:  Res720p,
			UpstreamTotalTokens: 108000,
			RequestedRatio:      "16:9",
			OriginModelName:     "doubao-seedance-2-0-260128",
		},
	}
	assert.Equal(t, 243000, peeledTokensForTask(task))
	assert.Equal(t, 243000, task.PrivateData.PeeledTotalTokens)
}

func TestRewriteUserVisibleTaskDataMarksFailed(t *testing.T) {
	raw := []byte(`{"status":"succeeded","resolution":"720p","content":{"video_url":"https://up.example/720.mp4"},"usage":{"completion_tokens":108000,"total_tokens":108000}}`)
	got := rewriteUserVisibleTaskData(raw, Res1080p, 243000, 243000, "", false, true)

	var m map[string]any
	require.NoError(t, common.Unmarshal(got, &m))
	assert.Equal(t, "failed", m["status"])
	assert.Equal(t, Res1080p, m["resolution"])
	content := m["content"].(map[string]any)
	_, hasURL := content["video_url"]
	assert.False(t, hasURL)
	assert.NotContains(t, string(got), "720p")
	assert.NotContains(t, string(got), "https://up.example/720.mp4")
}

func TestApplyUserVisibleRewriteSkippedUsesPublicURL(t *testing.T) {
	task := &model.Task{
		TaskID: "task_skip_url",
		PrivateData: model.TaskPrivateData{
			RequestedResolution: Res1080p,
			UpstreamResolution:  Res1080p,
			EnhanceStatus:       EnhanceStatusSkipped,
			ResultURL:           "https://ark-acg-cn-beijing.tos-cn-beijing.volces.com/clip.mp4",
		},
		Data: []byte(`{"status":"succeeded","resolution":"1080p","content":{"video_url":"https://ark-acg-cn-beijing.tos-cn-beijing.volces.com/clip.mp4"}}`),
	}
	applyUserVisibleRewrite(task, task.PublicResultURL(), true, false)

	var m map[string]any
	require.NoError(t, common.Unmarshal(task.Data, &m))
	content := m["content"].(map[string]any)
	assert.Contains(t, content["video_url"], "/v1/videos/task_skip_url/content")
	assert.NotContains(t, string(task.Data), "tos-cn-beijing")
	assert.Equal(t, "https://ark-acg-cn-beijing.tos-cn-beijing.volces.com/clip.mp4", task.GetResultURL())
}

func TestRedactUserVisibleHidesUpstreamWhileRunning(t *testing.T) {
	task := taskWithDowngrade()
	raw, err := common.Marshal(map[string]any{
		"status":     "running",
		"resolution": Res720p,
		"content":    map[string]any{"video_url": "https://up.example/720.mp4"},
	})
	require.NoError(t, err)
	task.Data = raw
	(&TaskAdaptor{}).RedactUserVisible(task)

	var m map[string]any
	require.NoError(t, common.Unmarshal(task.Data, &m))
	assert.Equal(t, Res1080p, m["resolution"])
	assert.Equal(t, "running", m["status"])
	assert.NotContains(t, string(task.Data), "720p")
	assert.NotContains(t, string(task.Data), "https://up.example/720.mp4")
}

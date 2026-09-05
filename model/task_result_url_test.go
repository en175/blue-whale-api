package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPublicResultURLUsesProxyWhenEnhanceRecorded(t *testing.T) {
	task := &Task{
		TaskID: "task_hide_enhance",
		PrivateData: TaskPrivateData{
			ResultURL:     "https://ark-acg-cn-beijing.tos-cn-beijing.volces.com/clip.mp4",
			EnhanceStatus: "skipped",
		},
	}
	got := task.PublicResultURL()
	assert.Contains(t, got, "/v1/videos/task_hide_enhance/content")
	assert.NotContains(t, got, "tos-cn-beijing")
	assert.Equal(t, "https://ark-acg-cn-beijing.tos-cn-beijing.volces.com/clip.mp4", task.GetResultURL())
}

func TestPublicDataRewritesStoredOriginURL(t *testing.T) {
	task := &Task{
		TaskID: "task_old_skip",
		Status: TaskStatusSuccess,
		PrivateData: TaskPrivateData{
			ResultURL:     "https://ark-acg-cn-beijing.tos-cn-beijing.volces.com/clip.mp4",
			EnhanceStatus: "skipped",
		},
		Data: []byte(`{"status":"succeeded","content":{"video_url":"https://ark-acg-cn-beijing.tos-cn-beijing.volces.com/clip.mp4"}}`),
	}
	got := string(task.PublicData())
	assert.Contains(t, got, "/v1/videos/task_old_skip/content")
	assert.NotContains(t, got, "tos-cn-beijing")
	assert.Contains(t, string(task.Data), "tos-cn-beijing")
}

func TestPublicResultURLPassthroughWithoutEnhanceStatus(t *testing.T) {
	task := &Task{
		TaskID: "task_plain",
		PrivateData: TaskPrivateData{
			ResultURL: "https://example.com/plain.mp4",
		},
	}
	assert.Equal(t, "https://example.com/plain.mp4", task.PublicResultURL())
}

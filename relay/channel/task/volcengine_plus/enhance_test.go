package volcengine_plus

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMediaKit struct {
	submitN         int
	getN            int
	failSubmitUntil int
	failGetUntil    int
	jobStatus       string
	videoURL        string
}

func (f *fakeMediaKit) Submit(_ context.Context, _, _ string) (string, error) {
	f.submitN++
	if f.submitN <= f.failSubmitUntil {
		return "", fmt.Errorf("submit failed")
	}
	return fmt.Sprintf("job-%d", f.submitN), nil
}

func (f *fakeMediaKit) Get(_ context.Context, _ string) (string, string, error) {
	f.getN++
	if f.getN <= f.failGetUntil {
		return "", "", fmt.Errorf("get failed")
	}
	return f.jobStatus, f.videoURL, nil
}

func taskWithDowngrade() *model.Task {
	return &model.Task{
		TaskID:    "task_test",
		ChannelId: 1,
		PrivateData: model.TaskPrivateData{
			RequestedResolution: Res1080p,
			UpstreamResolution:  Res720p,
			RequestedRatio:      "16:9",
			OriginModelName:     "doubao-seedance-2-0-260128",
			UpstreamTotalTokens: 108000,
			OutputDurationMs:    5000,
			SourceFps:           24,
			EnhanceSourceURL:    "https://up.example/720.mp4",
		},
	}
}

func TestEnsureEnhancedSkipsWhenNoDowngrade(t *testing.T) {
	task := &model.Task{
		TaskID:      "task_skip",
		PrivateData: model.TaskPrivateData{RequestedResolution: Res1080p, UpstreamResolution: Res1080p},
		Data:        []byte(`{"status":"succeeded","resolution":"1080p","content":{"video_url":"https://up.example/1080.mp4"}}`),
	}
	url, done, err := (&TaskAdaptor{}).EnsureEnhanced(context.Background(), task, "https://up.example/1080.mp4")
	require.NoError(t, err)
	assert.True(t, done)
	assert.Equal(t, "https://up.example/1080.mp4", url)
	assert.Equal(t, EnhanceStatusSkipped, task.PrivateData.EnhanceStatus)
	assert.Contains(t, string(task.Data), "/v1/videos/task_skip/content")
	assert.NotContains(t, string(task.Data), "https://up.example/1080.mp4")
}

func TestEnsureEnhancedSubmitsRequestedTier(t *testing.T) {
	var gotRes string
	kit := &recordingKit{onSubmit: func(_, resolution string) {
		gotRes = resolution
	}}
	a := &TaskAdaptor{mediaKit: kit, insertEnhanceCost: func(*model.EnhanceCost) error { return nil }}
	task := taskWithDowngrade()
	url, done, err := a.EnsureEnhanced(context.Background(), task, task.PrivateData.EnhanceSourceURL)
	require.NoError(t, err)
	assert.False(t, done)
	assert.Empty(t, url)
	assert.Equal(t, Res1080p, gotRes)
	assert.Equal(t, 1, task.PrivateData.EnhanceAttempt)
	assert.Equal(t, EnhanceStatusPending, task.PrivateData.EnhanceStatus)
}

func TestEnsureEnhancedClientInitFailureRecordsCost(t *testing.T) {
	costs := 0
	a := &TaskAdaptor{
		loadChannel: func(int) (*model.Channel, error) {
			return nil, fmt.Errorf("no channel")
		},
		insertEnhanceCost: func(*model.EnhanceCost) error {
			costs++
			return nil
		},
	}
	task := taskWithDowngrade()
	_, done, err := a.EnsureEnhanced(context.Background(), task, task.PrivateData.EnhanceSourceURL)
	require.Error(t, err)
	assert.True(t, done)
	assert.Equal(t, EnhanceStatusFailed, task.PrivateData.EnhanceStatus)
	assert.Equal(t, 1, costs)
}

func TestEnsureEnhancedRetriesUntilThreeFailures(t *testing.T) {
	kit := &fakeMediaKit{failSubmitUntil: 3}
	costs := 0
	a := &TaskAdaptor{mediaKit: kit, insertEnhanceCost: func(*model.EnhanceCost) error {
		costs++
		return nil
	}}
	task := taskWithDowngrade()
	var done bool
	var err error
	for i := 0; i < 5 && !done; i++ {
		_, done, err = a.EnsureEnhanced(context.Background(), task, task.PrivateData.EnhanceSourceURL)
	}
	require.Error(t, err)
	assert.True(t, done)
	assert.Equal(t, MaxEnhanceAttempts, task.PrivateData.EnhanceAttempt)
	assert.Equal(t, EnhanceStatusFailed, task.PrivateData.EnhanceStatus)
	assert.Equal(t, 3, costs)
	assert.Equal(t, 3, kit.submitN)
}

func TestEnsureEnhancedSuccessReturnsURL(t *testing.T) {
	kit := &fakeMediaKit{jobStatus: "completed", videoURL: "https://mk.example/1080.mp4"}
	var last *model.EnhanceCost
	a := &TaskAdaptor{mediaKit: kit, insertEnhanceCost: func(c *model.EnhanceCost) error {
		last = c
		return nil
	}}
	task := taskWithDowngrade()
	task.Data = []byte(`{"status":"succeeded","resolution":"720p","content":{"video_url":"https://up.example/720.mp4"}}`)
	_, done, err := a.EnsureEnhanced(context.Background(), task, task.PrivateData.EnhanceSourceURL)
	require.NoError(t, err)
	require.False(t, done)

	url, done, err := a.EnsureEnhanced(context.Background(), task, task.PrivateData.EnhanceSourceURL)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Equal(t, "https://mk.example/1080.mp4", url)
	require.NotNil(t, last)
	assert.Equal(t, EnhanceStatusSucceeded, last.Status)
	assert.Equal(t, 42, last.AmountFen)
	assert.Contains(t, string(task.Data), "/v1/videos/task_test/content")
	assert.NotContains(t, string(task.Data), "https://mk.example/1080.mp4")
	assert.NotContains(t, string(task.Data), "720p")
}

type recordingKit struct {
	onSubmit func(videoURL, resolution string)
	n        int
}

func (r *recordingKit) Submit(_ context.Context, videoURL, resolution string) (string, error) {
	r.n++
	if r.onSubmit != nil {
		r.onSubmit(videoURL, resolution)
	}
	return "job-1", nil
}

func (r *recordingKit) Get(context.Context, string) (string, string, error) {
	return "running", "", nil
}

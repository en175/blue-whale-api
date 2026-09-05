package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertEnhanceCostRow(t *testing.T, cost *EnhanceCost) {
	t.Helper()
	require.NoError(t, InsertEnhanceCost(cost))
}

func TestTaskGetAllTasksEnhancedOnlyUsesEnhanceCosts(t *testing.T) {
	truncateTables(t)

	enhanced := &Task{TaskID: "task_enhanced", UserId: 1, Status: TaskStatusSuccess}
	plain := &Task{TaskID: "task_plain", UserId: 1, Status: TaskStatusSuccess}
	insertTask(t, enhanced)
	insertTask(t, plain)
	insertEnhanceCostRow(t, &EnhanceCost{
		TaskId:              enhanced.TaskID,
		ChannelId:           29,
		RequestedResolution: "1080p",
		UpstreamResolution:  "720p",
		AmountFen:           42,
		Status:              "succeeded",
	})

	got := TaskGetAllTasks(0, 20, SyncTaskQueryParams{EnhancedOnly: true})
	require.Len(t, got, 1)
	assert.Equal(t, "task_enhanced", got[0].TaskID)
	assert.Equal(t, int64(1), TaskCountAllTasks(SyncTaskQueryParams{EnhancedOnly: true}))
}

func TestTaskGetAllUserTaskIgnoresEnhancedOnly(t *testing.T) {
	truncateTables(t)

	plain := &Task{TaskID: "task_user_plain", UserId: 7, Status: TaskStatusSuccess}
	insertTask(t, plain)

	got := TaskGetAllUserTask(7, 0, 20, SyncTaskQueryParams{EnhancedOnly: true})
	require.Len(t, got, 1)
	assert.Equal(t, "task_user_plain", got[0].TaskID)
}

func TestSummarizeEnhanceCostsByTaskIDs(t *testing.T) {
	truncateTables(t)

	insertEnhanceCostRow(t, &EnhanceCost{
		TaskId:              "task_ok",
		RequestedResolution: "1080p",
		UpstreamResolution:  "720p",
		AmountFen:           0,
		Status:              "failed",
	})
	insertEnhanceCostRow(t, &EnhanceCost{
		TaskId:              "task_ok",
		RequestedResolution: "1080p",
		UpstreamResolution:  "720p",
		AmountFen:           42,
		Status:              "succeeded",
	})
	insertEnhanceCostRow(t, &EnhanceCost{
		TaskId:              "task_fail",
		RequestedResolution: "1080p",
		UpstreamResolution:  "480p",
		AmountFen:           0,
		Status:              "failed",
	})

	got := SummarizeEnhanceCostsByTaskIDs([]string{"task_ok", "task_fail", "task_none"})
	require.Len(t, got, 2)

	ok := got["task_ok"]
	assert.Equal(t, "succeeded", ok.Status)
	assert.Equal(t, "1080p", ok.RequestedResolution)
	assert.Equal(t, "720p", ok.UpstreamResolution)
	assert.Equal(t, 42, ok.AmountFen)
	assert.Equal(t, 2, ok.AttemptCount)

	fail := got["task_fail"]
	assert.Equal(t, "failed", fail.Status)
	assert.Equal(t, 0, fail.AmountFen)
	assert.Equal(t, 1, fail.AttemptCount)
	assert.NotContains(t, got, "task_none")
}

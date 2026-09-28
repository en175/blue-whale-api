package mediakit

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureTaskCost 注入审计行落库桩，返回捕获到的行。
func captureTaskCost(t *testing.T) (*TaskAdaptor, **model.MediaKitCost) {
	t.Helper()
	var captured *model.MediaKitCost
	adaptor := &TaskAdaptor{
		insertMediaKitCost: func(cost *model.MediaKitCost) error {
			captured = cost
			return nil
		},
	}
	return adaptor, &captured
}

// TestRecordTaskCostDerivesFromTask 保护审计行的取值来源：所有字段都必须从任务
// 自身推出，不依赖提交时额外透传。工具名来自 Action、预估分钟来自 BillingContext
// 的 minutes 倍率、真实时长来自轮询写入的 Data、实扣额度由轮询层传入
// （退款过就是 0，不能读 task.Quota——退款不回写该字段）。
func TestRecordTaskCostDerivesFromTask(t *testing.T) {
	adaptor, captured := captureTaskCost(t)

	task := &model.Task{
		TaskID:    "task_public_1",
		ChannelId: 7,
		Action:    "enhance-video-generative",
		Status:    model.TaskStatusSuccess,
		Quota:     12345, // 预扣值，不应出现在审计行里
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream_job_9",
			BillingContext: &model.TaskBillingContext{
				ModelPrice:  2.5,
				OtherRatios: map[string]float64{RatioKeyMinutes: 1},
			},
		},
	}
	task.SetData(map[string]any{
		"success": true,
		"status":  "completed",
		"result":  map[string]any{"video_url": "https://x/a.mp4", "duration": 90},
	})

	adaptor.RecordTaskCost(context.Background(), task, 0) // 失败退款后传 0

	require.NotNil(t, *captured)
	row := *captured
	assert.Equal(t, "task_public_1", row.TaskId)
	assert.Equal(t, 7, row.ChannelId)
	assert.Equal(t, "enhance-video-generative", row.Tool)
	assert.Equal(t, string(BillingDuration), row.BillingMode)
	assert.Equal(t, "upstream_job_9", row.UpstreamJobId)
	assert.Equal(t, string(model.TaskStatusSuccess), row.Status)
	assert.Equal(t, float64(1), row.EstimatedMinutes)
	assert.Equal(t, float64(90), row.ActualSeconds)
	assert.Equal(t, 0, row.ChargedQuota, "实扣额度取轮询层传入值，不读 task.Quota")
}

// TestRecordTaskCostSkipsUnknownTool 工具名不在注册表里说明这不是本渠道的任务，
// 写一行来路不明的审计记录比不写更糟。
func TestRecordTaskCostSkipsUnknownTool(t *testing.T) {
	adaptor, captured := captureTaskCost(t)

	adaptor.RecordTaskCost(context.Background(), &model.Task{
		TaskID: "task_public_2",
		Action: "song", // 其他平台的 action
		Status: model.TaskStatusSuccess,
	}, 100)

	assert.Nil(t, *captured)
}

// TestRecordTaskCostToleratesNilTask 轮询层在超时清扫路径上同样会调用本方法，
// 那里 task 一定非 nil，但方法本身不该因为 nil 而炸掉整轮轮询。
func TestRecordTaskCostToleratesNilTask(t *testing.T) {
	adaptor, captured := captureTaskCost(t)
	adaptor.RecordTaskCost(context.Background(), nil, 0)
	assert.Nil(t, *captured)
}

// TestParseTaskResultKeepsFailureReason 是真实上游踩出来的回归：上游把失败详情
// 放在顶层 error 对象里，顶层 message 是空的。只读 message 会让任务在库里落成
// 一条没有原因的失败记录，用户和运维都看不出为什么挂了。
func TestParseTaskResultKeepsFailureReason(t *testing.T) {
	body := []byte(`{"error":{"code":"DownloadFileError","type":"BadRequest",` +
		`"message":"file upload by url failed, code: DownloadFailed"},"status":"failed",` +
		`"success":true,"task_id":"amk-tool-x-1","task_type":"enhance-video-generative"}`)

	var adaptor TaskAdaptor
	result, err := adaptor.ParseTaskResult(body)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.TaskStatusFailure, result.Status)
	assert.Equal(t, "DownloadFileError: file upload by url failed, code: DownloadFailed", result.Reason)
}

// TestParseTaskResultFallsBackToTopLevelMessage 旧形状（错误只在顶层 message）
// 仍要能取到原因，不能为了新形状把老形状丢掉。
func TestParseTaskResultFallsBackToTopLevelMessage(t *testing.T) {
	var adaptor TaskAdaptor
	result, err := adaptor.ParseTaskResult([]byte(`{"success":false,"status":"failed","message":"boom"}`))
	require.NoError(t, err)
	assert.Equal(t, "boom", result.Reason)
}

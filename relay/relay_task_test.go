package relay

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTaskModel2DtoPopulatesUsage newapi 平台（返回 token 的上游）→ usage 非空。
func TestTaskModel2DtoPopulatesUsage(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_usage",
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPI)),
		Status:   model.TaskStatusSuccess,
		Data: json.RawMessage(`{"code":"success","data":{"task_id":"task_usage","status":"SUCCESS","usage_completion_tokens":108900,"usage_total_tokens":108900}}`),
	}
	dto_ := TaskModel2Dto(task)
	require.NotNil(t, dto_.Usage)
	assert.Equal(t, 108900, dto_.Usage.CompletionTokens)
	assert.Equal(t, 108900, dto_.Usage.TotalTokens)
}

// TestTaskModel2DtoNoUsageForKling 未实现 TaskUsageExtractor 的平台（kling）→ usage 为 nil。
func TestTaskModel2DtoNoUsageForKling(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_kling",
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeKling)),
		Status:   model.TaskStatusSuccess,
		Data:     json.RawMessage(`{"data":{"task_status":"succeed","final_unit_deduction":"10"}}`),
	}
	dto_ := TaskModel2Dto(task)
	assert.Nil(t, dto_.Usage)
}

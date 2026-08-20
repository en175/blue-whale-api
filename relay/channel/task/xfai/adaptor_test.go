package xfai

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractTaskUsage 从 usage.completion_tokens/total_tokens 提取真实 token。
func TestExtractTaskUsage(t *testing.T) {
	task := &model.Task{
		Data: json.RawMessage(`{"id":"task_1","model":"xfai-video","status":"succeeded","content":{"video_url":"https://x.mp4"},"usage":{"completion_tokens":108900,"total_tokens":108900}}`),
	}
	usage := (&TaskAdaptor{}).ExtractTaskUsage(task)
	require.NotNil(t, usage)
	assert.Equal(t, 108900, usage.CompletionTokens)
	assert.Equal(t, 108900, usage.TotalTokens)
}

// TestExtractTaskUsage_NoTokens 无 usage / 解析失败 → nil。
func TestExtractTaskUsage_NoTokens(t *testing.T) {
	empty := &model.Task{Data: json.RawMessage(`{"id":"task_2","status":"processing"}`)}
	assert.Nil(t, (&TaskAdaptor{}).ExtractTaskUsage(empty))
	bad := &model.Task{Data: json.RawMessage(`not-json`)}
	assert.Nil(t, (&TaskAdaptor{}).ExtractTaskUsage(bad))
}

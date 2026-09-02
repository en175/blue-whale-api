package doubao

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRequestAndSetActionInfersSubmitShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		body   string
		action string
	}{
		{
			name:   "prompt only is 文生视频",
			body:   `{"model":"doubao-seedance-1-0-lite-t2v","prompt":"a cat walking"}`,
			action: "textGenerate",
		},
		{
			name:   "top-level image is 图生视频",
			body:   `{"model":"doubao-seedance-1-0-lite-i2v","prompt":"animate","image":"https://example.com/first.png"}`,
			action: "generate",
		},
		{
			name:   "metadata image_url is 图生视频",
			body:   `{"model":"doubao-seedance-1-0-lite-i2v","prompt":"animate","metadata":{"content":[{"type":"image_url","image_url":{"url":"https://example.com/ref.png"}}]}}`,
			action: "generate",
		},
		{
			name:   "metadata video_url is 视频生视频",
			body:   `{"model":"doubao-seedance-2-0-260128","prompt":"restyle","metadata":{"content":[{"type":"video_url","video_url":{"url":"https://example.com/clip.mp4"}}]}}`,
			action: "videoGenerate",
		},
		{
			name:   "video_url takes priority over image",
			body:   `{"model":"doubao-seedance-2-0-260128","prompt":"restyle","image":"https://example.com/first.png","metadata":{"content":[{"type":"video_url","video_url":{"url":"https://example.com/clip.mp4"}}]}}`,
			action: "videoGenerate",
		},
		{
			name:   "audio_url alone stays 文生视频",
			body:   `{"model":"doubao-seedance-2-0-260128","prompt":"score","metadata":{"content":[{"type":"audio_url","audio_url":{"url":"https://example.com/a.mp3"}}]}}`,
			action: "textGenerate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}

			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)

			require.Nil(t, taskErr)
			require.Equal(t, tt.action, info.Action)
		})
	}
}

// TestExtractTaskUsage 从 usage.completion_tokens/total_tokens 提取真实 token。
func TestExtractTaskUsage(t *testing.T) {
	task := &model.Task{
		Data: json.RawMessage(`{"id":"task_1","model":"doubao-seedance-1-0","status":"succeeded","content":{"video_url":"https://x.mp4"},"usage":{"completion_tokens":108900,"total_tokens":108900}}`),
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

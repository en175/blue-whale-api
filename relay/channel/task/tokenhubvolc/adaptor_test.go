package tokenhubvolc

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTaskContext 复现请求转换中间件的产物：解析原生请求、写入上下文，并把
// 内部请求体（model + prompt + metadata）放回请求，供通用校验读取。
func newTaskContext(t *testing.T, nativeJSON string) (*gin.Context, *relaycommon.RelayInfo, *NativeRequest) {
	t.Helper()

	native, err := ParseNativeRequest([]byte(nativeJSON))
	require.NoError(t, err)

	internal := map[string]any{"model": native.Model, "metadata": native.Raw}
	if prompt := native.Prompt(); prompt != "" {
		internal["prompt"] = prompt
	}
	body, err := common.Marshal(internal)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, constant.NativeArkVideoTaskPath, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	StoreNativeRequest(c, native)

	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta:   &relaycommon.ChannelMeta{},
	}
	return c, info, native
}

func TestValidateRequestAndSetAction_draftGuards(t *testing.T) {
	tests := []struct {
		name         string
		nativeJSON   string
		wantCode     string
		wantDraft    bool
		wantVideoIn  bool
		wantNoTaskEr bool
	}{
		{
			name:         "2.5 样片 480p 通过",
			nativeJSON:   `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"a cat"}],"draft":true,"resolution":"480p"}`,
			wantDraft:    true,
			wantNoTaskEr: true,
		},
		{
			name:       "2.0 开启样片被拒",
			nativeJSON: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"a cat"}],"draft":true}`,
			wantCode:   "invalid_draft_model",
		},
		{
			name:       "样片请求 720p 被拒",
			nativeJSON: `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"a cat"}],"draft":true,"resolution":"720p"}`,
			wantCode:   "invalid_resolution",
		},
		{
			name:       "时长越界被拒",
			nativeJSON: `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"a cat"}],"duration":999999}`,
			wantCode:   "invalid_duration",
		},
		{
			name:         "含视频输入被识别",
			nativeJSON:   `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"a cat"},{"type":"video_url","video_url":{"url":"https://x/v.mp4"}}]}`,
			wantVideoIn:  true,
			wantNoTaskEr: true,
		},
		{
			name:         "编辑场景 duration=-1 允许",
			nativeJSON:   `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"edit"}],"duration":-1}`,
			wantNoTaskEr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, info, _ := newTaskContext(t, tt.nativeJSON)
			adaptor := &TaskAdaptor{}

			taskErr := adaptor.ValidateRequestAndSetAction(c, info)

			if tt.wantCode != "" {
				require.NotNil(t, taskErr)
				assert.Equal(t, tt.wantCode, taskErr.Code)
				assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
				return
			}
			require.Nil(t, taskErr)
			assert.Equal(t, tt.wantDraft, info.Draft)
			assert.Equal(t, tt.wantVideoIn, info.HasVideoInput)
		})
	}
}

func TestBuildRequestBody_forcesDraftSpec(t *testing.T) {
	nativeJSON := `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"a cat"},{"type":"image_url","image_url":{"url":"asset://a"},"role":"first_frame"}],"draft":true}`
	c, info, _ := newTaskContext(t, nativeJSON)
	info.UpstreamModelName = "dreamina-seedance-2-5-260628"
	info.Draft = true

	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, reader)

	assert.Equal(t, "dreamina-seedance-2-5-260628", payload["model"])
	assert.Equal(t, true, payload["draft"])
	assert.Equal(t, Res480p, payload["resolution"])
	assert.Equal(t, Res480p, info.UpstreamResolution)
	// content 原样透传，包含 role 与素材引用
	assert.Equal(t, []any{
		map[string]any{"type": "text", "text": "a cat"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "asset://a"}, "role": "first_frame"},
	}, payload["content"])
}

func TestBuildRequestBody_keepsUnknownTopLevelFields(t *testing.T) {
	nativeJSON := `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"a cat"}],"group":"vip","tools":[{"type":"web_search"}],"seed":7}`
	c, info, _ := newTaskContext(t, nativeJSON)
	info.UpstreamModelName = "dreamina-seedance-2-0-260128"

	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, reader)

	// 网关路由键不发给上游；没建模的火山字段原样透传
	assert.NotContains(t, payload, "group")
	assert.Equal(t, float64(7), payload["seed"])
	assert.Equal(t, []any{map[string]any{"type": "web_search"}}, payload["tools"])
}

func TestBuildRequestBody_finalizeOverridesResolution(t *testing.T) {
	// 客户端在成片请求里传 720p / 时长 / 提示词，全部不得改变成片规格
	nativeJSON := `{"model":"doubao-seedance-2-5-260628","content":[{"type":"draft_task","draft_task":{"id":"task_abc"}}],"resolution":"720p","duration":10,"seed":3,"prompt":"ignored","output_format":"mov"}`
	c, info, _ := newTaskContext(t, nativeJSON)
	info.UpstreamModelName = "dreamina-seedance-2-5-260628"
	info.Action = constant.TaskActionFinalize
	info.OriginUpstreamTaskID = "cgt-upstream-1"

	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))

	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, reader)

	assert.Equal(t, Res1080p, payload["resolution"])
	assert.Equal(t, Res1080p, info.UpstreamResolution)
	assert.Equal(t, "mov", payload["output_format"])
	assert.Equal(t, []any{map[string]any{
		"type":       "draft_task",
		"draft_task": map[string]any{"id": "cgt-upstream-1"},
	}}, payload["content"])
	for _, key := range []string{"duration", "seed", "prompt", "draft"} {
		assert.NotContains(t, payload, key)
	}
}

func TestConvertToNativeArkTask_statusAndPayload(t *testing.T) {
	adaptor := &TaskAdaptor{}

	t.Run("成功任务回填视频地址与用量", func(t *testing.T) {
		task := &model.Task{
			TaskID:    "task_ok",
			Status:    model.TaskStatusSuccess,
			CreatedAt: 100,
			UpdatedAt: 200,
		}
		task.Properties.OriginModelName = "doubao-seedance-2-5-260628"
		task.Data = []byte(`{"resultSummary":{"content":{"video_url":"https://cdn/v.mp4"},"resolution":"1080p","duration":"5","usage":{"completion_tokens":120,"total_tokens":150}}}`)

		raw, err := adaptor.ConvertToNativeArkTask(task)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, common.Unmarshal(raw, &out))
		assert.Equal(t, "task_ok", out["id"])
		assert.Equal(t, "succeeded", out["status"])
		assert.Equal(t, "doubao-seedance-2-5-260628", out["model"])
		assert.Equal(t, map[string]any{"video_url": "https://cdn/v.mp4"}, out["content"])
		assert.Equal(t, map[string]any{"completion_tokens": float64(120), "total_tokens": float64(150)}, out["usage"])
		assert.Equal(t, "1080p", out["resolution"])
		assert.Equal(t, float64(5), out["duration"])
	})

	t.Run("运行中任务不返回产物", func(t *testing.T) {
		task := &model.Task{TaskID: "task_run", Status: model.TaskStatusInProgress}
		task.Data = []byte(`{"resultSummary":{"content":{"video_url":"https://cdn/v.mp4"}}}`)

		raw, err := adaptor.ConvertToNativeArkTask(task)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, common.Unmarshal(raw, &out))
		assert.Equal(t, "running", out["status"])
		assert.NotContains(t, out, "content")
		assert.NotContains(t, out, "usage")
	})

	t.Run("失败任务返回错误对象", func(t *testing.T) {
		task := &model.Task{TaskID: "task_bad", Status: model.TaskStatusFailure, FailReason: "content blocked"}
		task.Data = []byte(`{}`)

		raw, err := adaptor.ConvertToNativeArkTask(task)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, common.Unmarshal(raw, &out))
		assert.Equal(t, "failed", out["status"])
		assert.Equal(t, map[string]any{"code": "TaskFailed", "message": "content blocked"}, out["error"])
	})
}

func readPayload(t *testing.T, reader io.Reader) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(raw, &payload))
	return payload
}

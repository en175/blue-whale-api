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
	"github.com/QuantumNous/new-api/relay/channel/task/arknative"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTaskContext 复现请求转换中间件的产物：解析原生请求、写入上下文，并把
// 内部请求体（model + prompt + metadata）放回请求，供通用校验读取。
func newTaskContext(t *testing.T, nativeJSON string) (*gin.Context, *relaycommon.RelayInfo, *arknative.NativeRequest) {
	t.Helper()

	native, err := arknative.ParseNativeRequest([]byte(nativeJSON))
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
	arknative.StoreNativeRequest(c, native)

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
		task.Data = []byte(`{"id":"01a17b4f-083d-71f4-bcab-c5128ba2f00c","upstreamTaskId":"cgt-20261027110219-vvggz","status":"succeeded","resultSummary":{"content":{"video_url":"https://cdn/v.mp4","last_frame_url":"https://cdn/f.png"},"resolution":"1080p","duration":"5","usage":{"completion_tokens":120,"total_tokens":150}}}`)

		raw, err := adaptor.ConvertToNativeArkTask(task)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, common.Unmarshal(raw, &out))
		assert.Equal(t, "task_ok", out["id"])
		assert.Equal(t, "succeeded", out["status"])
		assert.Equal(t, "doubao-seedance-2-5-260628", out["model"])
		// 华鲲的 UUID 是轮询键，火山 cgt 号才是到火山侧排障的凭据，两者都要能让下游看到
		assert.Equal(t, "cgt-20261027110219-vvggz", out["upstreamTaskId"])
		assert.Equal(t, map[string]any{"video_url": "https://cdn/v.mp4", "last_frame_url": "https://cdn/f.png"}, out["content"])
		assert.Equal(t, map[string]any{"completion_tokens": float64(120), "total_tokens": float64(150)}, out["usage"])
		assert.Equal(t, "1080p", out["resolution"])
		assert.Equal(t, float64(5), out["duration"])
	})

	t.Run("只回尾帧时仍返回 content", func(t *testing.T) {
		task := &model.Task{TaskID: "task_frame", Status: model.TaskStatusSuccess}
		task.Data = []byte(`{"resultSummary":{"content":{"last_frame_url":"https://cdn/f.png"}}}`)

		raw, err := adaptor.ConvertToNativeArkTask(task)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, common.Unmarshal(raw, &out))
		assert.Equal(t, map[string]any{"last_frame_url": "https://cdn/f.png"}, out["content"])
	})

	t.Run("透出联网搜索用量与实际调用的工具", func(t *testing.T) {
		task := &model.Task{TaskID: "task_tools", Status: model.TaskStatusSuccess}
		task.Data = []byte(`{"tools":[{"type":"web_search"}],"resultSummary":{"content":{"video_url":"https://cdn/v.mp4"},"usage":{"completion_tokens":10,"total_tokens":10,"tool_usage":{"web_search":2}}}}`)

		raw, err := adaptor.ConvertToNativeArkTask(task)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, common.Unmarshal(raw, &out))
		assert.Equal(t, map[string]any{
			"completion_tokens": float64(10),
			"total_tokens":      float64(10),
			"tool_usage":        map[string]any{"web_search": float64(2)},
		}, out["usage"])
		assert.Equal(t, []any{map[string]any{"type": "web_search"}}, out["tools"])
	})

	t.Run("只有 tool_usage 时仍输出 usage", func(t *testing.T) {
		task := &model.Task{TaskID: "task_tool_only", Status: model.TaskStatusSuccess}
		task.Data = []byte(`{"resultSummary":{"usage":{"tool_usage":{"web_search":3}}}}`)

		raw, err := adaptor.ConvertToNativeArkTask(task)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, common.Unmarshal(raw, &out))
		assert.Equal(t, map[string]any{
			"completion_tokens": float64(0),
			"total_tokens":      float64(0),
			"tool_usage":        map[string]any{"web_search": float64(3)},
		}, out["usage"])
	})

	t.Run("辅助字段形态漂移不影响主响应", func(t *testing.T) {
		task := &model.Task{TaskID: "task_drift", Status: model.TaskStatusSuccess}
		task.Data = []byte(`{"tools":{"type":"web_search"},"resultSummary":{"content":{"video_url":"https://cdn/v.mp4"},"usage":{"completion_tokens":1,"total_tokens":2,"tool_usage":{"web_search":"2"}}}}`)

		raw, err := adaptor.ConvertToNativeArkTask(task)
		require.NoError(t, err)

		var out map[string]any
		require.NoError(t, common.Unmarshal(raw, &out))
		assert.Equal(t, "succeeded", out["status"])
		assert.Equal(t, map[string]any{"video_url": "https://cdn/v.mp4"}, out["content"])
		assert.NotContains(t, out, "tools")
		assert.Equal(t, map[string]any{
			"completion_tokens": float64(1),
			"total_tokens":      float64(2),
		}, out["usage"])
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
		// 上游没给出 cgt 号时省略字段，不返回空串占位
		assert.NotContains(t, out, "upstreamTaskId")
	})

	t.Run("失败任务透出上游错误码", func(t *testing.T) {
		t.Run("优先结果信封内的 error.code", func(t *testing.T) {
			task := &model.Task{TaskID: "task_code", Status: model.TaskStatusFailure}
			task.Data = []byte(`{"error":{"code":"TopLevel","message":"outer"},"resultSummary":{"error":{"code":"SensitiveContent","message":"content blocked"}}}`)

			raw, err := adaptor.ConvertToNativeArkTask(task)
			require.NoError(t, err)

			var out map[string]any
			require.NoError(t, common.Unmarshal(raw, &out))
			assert.Equal(t, map[string]any{"code": "SensitiveContent", "message": "content blocked"}, out["error"])
		})

		t.Run("顶层 error.code 支持数字形态", func(t *testing.T) {
			task := &model.Task{TaskID: "task_code_num", Status: model.TaskStatusFailure}
			task.Data = []byte(`{"error":{"code":500,"message":"upstream failed"}}`)

			raw, err := adaptor.ConvertToNativeArkTask(task)
			require.NoError(t, err)

			var out map[string]any
			require.NoError(t, common.Unmarshal(raw, &out))
			assert.Equal(t, map[string]any{"code": "500", "message": "upstream failed"}, out["error"])
		})
	})
}

// TestValidateRequestAndSetAction_infersTaskAction 验证任务日志的 action 按输入
// 形态写入：参考视频 → videoGenerate、图片 → generate、纯文本 → textGenerate。
// 之前固定成 generate，导致纯文本和视频生视频都显示成「图生视频」。
func TestValidateRequestAndSetAction_infersTaskAction(t *testing.T) {
	tests := []struct {
		name       string
		nativeJSON string
		wantAction string
	}{
		{
			name:       "纯文本是文生视频",
			nativeJSON: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"a cat"}]}`,
			wantAction: constant.TaskActionTextGenerate,
		},
		{
			name:       "image_url 是图生视频",
			nativeJSON: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"a cat"},{"type":"image_url","image_url":{"url":"asset://a"},"role":"first_frame"}]}`,
			wantAction: constant.TaskActionGenerate,
		},
		{
			name:       "只带 image_url 字段也识别为图生视频",
			nativeJSON: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"a cat"},{"image_url":{"url":"https://x/a.png"}}]}`,
			wantAction: constant.TaskActionGenerate,
		},
		{
			name:       "参考视频是视频生视频",
			nativeJSON: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"continue"},{"type":"video_url","video_url":{"url":"https://x/v.mp4"},"role":"reference_video"}]}`,
			wantAction: constant.TaskActionVideoGenerate,
		},
		{
			name:       "视频优先于图片",
			nativeJSON: `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"continue"},{"type":"image_url","image_url":{"url":"https://x/a.png"}},{"type":"video_url","video_url":{"url":"https://x/v.mp4"}}]}`,
			wantAction: constant.TaskActionVideoGenerate,
		},
		{
			name:       "纯音频不单独成类",
			nativeJSON: `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"score"},{"type":"audio_url","audio_url":{"url":"https://x/a.mp3"}}]}`,
			wantAction: constant.TaskActionTextGenerate,
		},
		{
			name:       "样片同样按输入推断",
			nativeJSON: `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"continue"},{"type":"video_url","video_url":{"url":"https://x/v.mp4"},"role":"reference_video"}],"draft":true}`,
			wantAction: constant.TaskActionVideoGenerate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, info, _ := newTaskContext(t, tt.nativeJSON)

			require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
			assert.Equal(t, tt.wantAction, info.Action)
		})
	}

	t.Run("成片保持 finalizeDraft", func(t *testing.T) {
		c, info, _ := newTaskContext(t, `{"model":"doubao-seedance-2-5-260628","content":[{"type":"draft_task","draft_task":{"id":"task_abc"}}]}`)
		info.Action = constant.TaskActionFinalize

		require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
		assert.Equal(t, constant.TaskActionFinalize, info.Action)
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

// TestValidateRequestAndSetAction_draftModelFollowsMapping 验证 2.5 判定看映射后的
// 上游模型名：把对客别名映射到 2.5 时样片不能被误拒，映射到 2.0 时仍要拒绝。
func TestValidateRequestAndSetAction_draftModelFollowsMapping(t *testing.T) {
	newDraftContext := func(t *testing.T, model string) (*gin.Context, *relaycommon.RelayInfo) {
		c, info, _ := newTaskContext(t,
			`{"model":"`+model+`","content":[{"type":"text","text":"a cat"}],"draft":true,"resolution":"480p"}`)
		return c, info
	}

	t.Run("别名映射到 2.5 时放行", func(t *testing.T) {
		c, info := newDraftContext(t, "seedance-pro")
		c.Set("model_mapping", `{"seedance-pro":"doubao-seedance-2-5-260628"}`)

		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)

		require.Nil(t, taskErr)
		assert.True(t, info.Draft)
	})

	t.Run("别名映射到 2.0 时拒绝", func(t *testing.T) {
		c, info := newDraftContext(t, "seedance-pro")
		c.Set("model_mapping", `{"seedance-pro":"doubao-seedance-2-0-260128"}`)

		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)

		require.NotNil(t, taskErr)
		assert.Equal(t, "invalid_draft_model", taskErr.Code)
	})
}

// newLegacyContext 构造旧视频接口（/v1/video/generations、/v1/videos/{id}/finalize）
// 的 gin 上下文：请求体是网关内部任务请求（prompt + seconds + metadata），同时写入
// BodyStorage，供通用校验与成片参数解析各自读取一次。
func newLegacyContext(t *testing.T, path, body string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	storage, err := common.CreateBodyStorage([]byte(body))
	require.NoError(t, err)
	c.Set(common.KeyBodyStorage, storage)

	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta:   &relaycommon.ChannelMeta{},
	}
	return c, info
}

// TestValidateRequestAndSetAction_legacyEntry 验证旧接口的请求体经 prompt +
// metadata 还原成火山形状后，与原生入口共用同一套样片守卫与输入判定。
func TestValidateRequestAndSetAction_legacyEntry(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCode    string
		wantDraft   bool
		wantVideoIn bool
		wantAction  string
	}{
		{
			name:       "图片参考按图生视频",
			body:       `{"model":"doubao-seedance-2-0-260128","prompt":"a cat","seconds":"5","metadata":{"resolution":"720p","content":[{"type":"image_url","image_url":{"url":"asset://Asset-1"},"role":"reference_image"}]}}`,
			wantAction: constant.TaskActionGenerate,
		},
		{
			name:        "参考视频被识别为视频输入",
			body:        `{"model":"doubao-seedance-2-0-260128","prompt":"continue","metadata":{"content":[{"type":"video_url","video_url":{"url":"https://x/v.mp4"}}]}}`,
			wantVideoIn: true,
			wantAction:  constant.TaskActionVideoGenerate,
		},
		{
			name:       "2.5 样片 480p 通过",
			body:       `{"model":"doubao-seedance-2-5-260628","prompt":"a cat","metadata":{"draft":true,"resolution":"480p"}}`,
			wantDraft:  true,
			wantAction: constant.TaskActionTextGenerate,
		},
		{
			name:     "2.0 开启样片被拒",
			body:     `{"model":"doubao-seedance-2-0-260128","prompt":"a cat","metadata":{"draft":true}}`,
			wantCode: "invalid_draft_model",
		},
		{
			name:     "样片请求 720p 被拒",
			body:     `{"model":"doubao-seedance-2-5-260628","prompt":"a cat","metadata":{"draft":true,"resolution":"720p"}}`,
			wantCode: "invalid_resolution",
		},
		{
			name:     "时长越界被拒",
			body:     `{"model":"doubao-seedance-2-5-260628","prompt":"a cat","metadata":{"duration":999999}}`,
			wantCode: "invalid_duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, info := newLegacyContext(t, "/v1/video/generations", tt.body)

			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)

			if tt.wantCode != "" {
				require.NotNil(t, taskErr)
				assert.Equal(t, tt.wantCode, taskErr.Code)
				assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
				return
			}
			require.Nil(t, taskErr)
			assert.Equal(t, tt.wantDraft, info.Draft)
			assert.Equal(t, tt.wantVideoIn, info.HasVideoInput)
			assert.Equal(t, tt.wantAction, info.Action)
		})
	}
}

// TestBuildRequestBody_legacyEntry 验证旧接口的出站请求体：提示词补成 content[]
// 的文本项、metadata.content 的媒体项原样保留、seconds 变成顶层 duration。
func TestBuildRequestBody_legacyEntry(t *testing.T) {
	body := `{"model":"doubao-seedance-2-0-260128","prompt":"a cat","seconds":"5","metadata":{"resolution":"720p","generate_audio":false,"content":[{"type":"image_url","image_url":{"url":"asset://Asset-1"},"role":"reference_image"}]}}`
	c, info := newLegacyContext(t, "/v1/video/generations", body)
	info.UpstreamModelName = "dreamina-seedance-2-0-260128"

	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, reader)

	assert.Equal(t, "dreamina-seedance-2-0-260128", payload["model"])
	assert.Equal(t, "720p", payload["resolution"])
	assert.Equal(t, float64(5), payload["duration"])
	assert.Equal(t, false, payload["generate_audio"], "显式 false 必须原样发给上游")
	assert.Equal(t, []any{
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "asset://Asset-1"}, "role": "reference_image"},
		map[string]any{"type": "text", "text": "a cat"},
	}, payload["content"])
}

// TestBuildRequestBody_legacyDraft 验证旧接口的样片同样强制 draft=true + 480p。
func TestBuildRequestBody_legacyDraft(t *testing.T) {
	body := `{"model":"doubao-seedance-2-5-260628","prompt":"a cat","metadata":{"draft":true,"resolution":"480p"}}`
	c, info := newLegacyContext(t, "/v1/video/generations", body)
	info.UpstreamModelName = "dreamina-seedance-2-5-260628"

	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	require.True(t, info.Draft)

	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, reader)

	assert.Equal(t, true, payload["draft"])
	assert.Equal(t, Res480p, payload["resolution"])
	assert.Equal(t, Res480p, info.UpstreamResolution)
}

// TestValidateRequestAndSetAction_legacyFinalize 验证旧接口的成片第二步：空体与
// 空对象都放行，非 1080p 分辨率被拒，输出规格类参数落到上下文供后续构造使用。
func TestValidateRequestAndSetAction_legacyFinalize(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode string
	}{
		{name: "空请求体", body: ""},
		{name: "空对象", body: `{}`},
		{name: "1080p 放行", body: `{"resolution":"1080p"}`},
		{name: "720p 被拒", body: `{"resolution":"720p"}`, wantCode: "invalid_resolution"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, info := newLegacyContext(t, "/v1/videos/task_draft/finalize", tt.body)
			info.Action = constant.TaskActionFinalize

			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)

			if tt.wantCode != "" {
				require.NotNil(t, taskErr)
				assert.Equal(t, tt.wantCode, taskErr.Code)
				return
			}
			require.Nil(t, taskErr)
		})
	}
}

// TestBuildRequestBody_legacyFinalize 验证旧接口成片：content 只能是网关换过 ID
// 的 draft_task 引用，分辨率固定 1080p，白名单内的输出规格原样带上（含 false）。
func TestBuildRequestBody_legacyFinalize(t *testing.T) {
	body := `{"output_format":"mov","watermark":false,"duration":10,"seed":3}`
	c, info := newLegacyContext(t, "/v1/videos/task_draft/finalize", body)
	info.UpstreamModelName = "dreamina-seedance-2-5-260628"
	info.Action = constant.TaskActionFinalize
	info.OriginUpstreamTaskID = "cgt-upstream-1"

	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, reader)

	assert.Equal(t, Res1080p, payload["resolution"])
	assert.Equal(t, "mov", payload["output_format"])
	assert.Equal(t, false, payload["watermark"], "白名单内的显式 false 必须保留")
	assert.Equal(t, []any{map[string]any{
		"type":       "draft_task",
		"draft_task": map[string]any{"id": "cgt-upstream-1"},
	}}, payload["content"])
	for _, key := range []string{"duration", "seed", "prompt", "draft"} {
		assert.NotContains(t, payload, key)
	}
}

// TestDoResponse_shapeByEntry 验证创建响应按入口分流：原生协议只回 {"id"}，
// 旧视频接口回 OpenAI 视频对象（与 61/66/67 一致）。
func TestDoResponse_shapeByEntry(t *testing.T) {
	newResponse := func() *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"cgt-upstream-9"}`))),
		}
	}
	newInfo := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
			ChannelMeta:     &relaycommon.ChannelMeta{},
			OriginModelName: "doubao-seedance-2-5-260628",
		}
	}

	t.Run("原生协议只回 id", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, constant.NativeArkVideoTaskPath, nil)

		taskID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, newResponse(), newInfo())
		require.Nil(t, taskErr)
		assert.Equal(t, "cgt-upstream-9", taskID)
		assert.JSONEq(t, `{"id":"task_public"}`, w.Body.String())
	})

	t.Run("旧接口回 OpenAI 视频对象", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", nil)

		taskID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, newResponse(), newInfo())
		require.Nil(t, taskErr)
		assert.Equal(t, "cgt-upstream-9", taskID)

		var video map[string]any
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &video))
		assert.Equal(t, "task_public", video["id"])
		assert.Equal(t, "task_public", video["task_id"])
		assert.Equal(t, "video", video["object"])
		assert.Equal(t, "queued", video["status"])
		assert.Equal(t, "doubao-seedance-2-5-260628", video["model"])
	})
}

// TestValidateRequestAndSetAction_legacyRejectsDraftTask 旧提交入口不能携带
// draft_task 引用：成片必须走 /v1/videos/{id}/finalize，由 ResolveOriginTask
// 校验归属、样片状态与 7 天窗口，并按成片档位计费。放行会在网关侧绕过这些
// 边界（引用被当作普通 content 发给上游）。
func TestValidateRequestAndSetAction_legacyRejectsDraftTask(t *testing.T) {
	body := `{"model":"doubao-seedance-2-5-260628","prompt":"a cat","metadata":{"content":[{"type":"draft_task","draft_task":{"id":"cgt-upstream-1"}}]}}`
	c, info := newLegacyContext(t, "/v1/video/generations", body)

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)

	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_content", taskErr.Code)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

// TestBuildRequestBody_legacyPromptWins 旧接口的提示词以顶层 prompt 为准：
// metadata.content 里已有的文本项不重复发送，媒体项原样保留。
func TestBuildRequestBody_legacyPromptWins(t *testing.T) {
	body := `{"model":"doubao-seedance-2-0-260128","prompt":"outer prompt","metadata":{"content":[{"type":"text","text":"inner prompt"},{"type":"image_url","image_url":{"url":"asset://Asset-1"}}]}}`
	c, info := newLegacyContext(t, "/v1/video/generations", body)
	info.UpstreamModelName = "dreamina-seedance-2-0-260128"

	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, reader)

	assert.Equal(t, []any{
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "asset://Asset-1"}},
		map[string]any{"type": "text", "text": "outer prompt"},
	}, payload["content"])
}

// TestBuildRequestBody_legacyMergesTopLevelImages 旧接口的 images 与
// metadata.content 可以同时出现：未出现过的图片补进 content，重复引用不重复发送。
func TestBuildRequestBody_legacyMergesTopLevelImages(t *testing.T) {
	body := `{"model":"doubao-seedance-2-0-260128","prompt":"a cat","images":["asset://Asset-A","asset://Asset-B"],"metadata":{"content":[{"type":"image_url","image_url":{"url":"asset://Asset-B"}}]}}`
	c, info := newLegacyContext(t, "/v1/video/generations", body)
	info.UpstreamModelName = "dreamina-seedance-2-0-260128"

	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	reader, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, reader)

	assert.Equal(t, []any{
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "asset://Asset-B"}},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "asset://Asset-A"}},
		map[string]any{"type": "text", "text": "a cat"},
	}, payload["content"], "images 只能补齐缺失项，不能覆盖或重复 metadata.content")
}

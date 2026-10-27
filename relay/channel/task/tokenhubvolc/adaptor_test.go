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

// TestEstimateBilling_tierByAction 锁定三步各自的档位来源：样片固定 480p、
// 成片固定 1080p 且视频输入维度继承样片、普通请求按本次请求体的分辨率与
// content[]。前两步不能用当次请求体——样片请求体只有 prompt，成片请求体里
// 没有 content，取错维度会静默按基准价结算。
func TestEstimateBilling_tierByAction(t *testing.T) {
	// doubao-seedance-2-0-260128 内置价格表（元/百万 token）：
	// 基准 46、基准含视频 28、1080p 51、1080p 含视频 31。
	const model = "doubao-seedance-2-0-260128"
	tests := []struct {
		name string
		info relaycommon.TaskRelayInfo
		req  relaycommon.TaskSubmitReq
		want float64
	}{
		{
			name: "成片固定1080p并继承样片的视频输入",
			info: relaycommon.TaskRelayInfo{Action: constant.TaskActionFinalize, OriginHasVideoInput: true},
			want: 31.0 / 46.0,
		},
		{
			// HasVideoInput 故意置为 true：成片必须只看 OriginHasVideoInput，
			// 本次请求体里的视频项不参与档位判定。
			name: "成片继承样片不含视频，忽略当次请求",
			info: relaycommon.TaskRelayInfo{Action: constant.TaskActionFinalize, HasVideoInput: true},
			want: 51.0 / 46.0,
		},
		{
			// metadata 故意写 4k：样片一律按 480p 档，不能跟随请求体分辨率。
			name: "样片固定480p档",
			info: relaycommon.TaskRelayInfo{Draft: true, HasVideoInput: true},
			req:  relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"resolution": "4k"}},
			want: 28.0 / 46.0,
		},
		{
			name: "普通请求按metadata分辨率与视频输入",
			req: relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{
				"resolution": "1080p",
				"content":    []interface{}{map[string]interface{}{"type": "video_url"}},
			}},
			want: 31.0 / 46.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_request", tt.req)
			info := &relaycommon.RelayInfo{
				TaskRelayInfo: &tt.info,
				ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: model},
			}

			got := (&TaskAdaptor{}).EstimateBilling(c, info)

			require.Len(t, got, 1)
			assert.InDelta(t, tt.want, got["video_input"], 1e-9)
		})
	}
}

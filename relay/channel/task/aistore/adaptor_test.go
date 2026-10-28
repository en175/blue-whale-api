package aistore

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	tasktokenhubvolc "github.com/QuantumNous/new-api/relay/channel/task/tokenhubvolc"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newNativeContext 复现 ArkNativeRequestConvert 的产物：解析原生请求写入上下文，
// 并把内部请求体（model + prompt + metadata）放回请求，供通用校验读取。
func newNativeContext(t *testing.T, nativeJSON string) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	t.Helper()

	native, err := tasktokenhubvolc.ParseNativeRequest([]byte(nativeJSON))
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
	tasktokenhubvolc.StoreNativeRequest(c, native)

	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta:   &relaycommon.ChannelMeta{},
	}
	info.OriginModelName = native.Model
	return c, w, info
}

func readPayload(t *testing.T, body io.Reader) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(raw, &payload))
	return payload
}

func TestBuildRequestURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelBaseUrl: "https://aistore.gsiig.cn/v1/"},
	}
	adaptor.Init(info)

	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://aistore.gsiig.cn/v1/contents/generations/tasks", url)
}

func TestValidateRequestAndSetAction(t *testing.T) {
	tests := []struct {
		name        string
		nativeJSON  string
		wantCode    string
		wantDraft   bool
		wantVideoIn bool
	}{
		{
			name:       "2.5 样片 480p 通过",
			nativeJSON: `{"model":"cdance2.5","content":[{"type":"text","text":"a cat"}],"draft":true,"resolution":"480p"}`,
			wantDraft:  true,
		},
		{
			name:       "2.0 开启样片被拒",
			nativeJSON: `{"model":"cdance2.0","content":[{"type":"text","text":"a cat"}],"draft":true}`,
			wantCode:   "invalid_draft_model",
		},
		{
			name:       "样片请求 720p 被拒",
			nativeJSON: `{"model":"cdance2.5","content":[{"type":"text","text":"a cat"}],"draft":true,"resolution":"720p"}`,
			wantCode:   "invalid_resolution",
		},
		{
			name:       "时长越界被拒",
			nativeJSON: `{"model":"cdance2.0","content":[{"type":"text","text":"a cat"}],"duration":999999}`,
			wantCode:   "invalid_duration",
		},
		{
			name:       "时长 0 被拒",
			nativeJSON: `{"model":"cdance2.0","content":[{"type":"text","text":"a cat"}],"duration":0}`,
			wantCode:   "invalid_duration",
		},
		{
			name:       "时长 -2 被拒",
			nativeJSON: `{"model":"cdance2.0","content":[{"type":"text","text":"a cat"}],"duration":-2}`,
			wantCode:   "invalid_duration",
		},
		{
			name:       "duration=-1 由模型决定时长时放行",
			nativeJSON: `{"model":"cdance2.0","content":[{"type":"text","text":"edit"}],"duration":-1}`,
		},
		{
			name:        "含视频输入被识别",
			nativeJSON:  `{"model":"cdance2.0","content":[{"type":"text","text":"a cat"},{"type":"video_url","video_url":{"url":"https://x/v.mp4"}}]}`,
			wantVideoIn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _, info := newNativeContext(t, tt.nativeJSON)
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

// TestBuildRequestBody_passthrough 保护「原样透传」这条契约：火山新增字段
// （output_format / return_last_frame / priority）必须到达上游，不能像渠道 54
// 那样被结构体静默丢弃；网关自己的 group 键不能发出去。
func TestBuildRequestBody_passthrough(t *testing.T) {
	nativeJSON := `{
		"model":"cdance2.0",
		"content":[
			{"type":"text","text":"a cat"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://x/a.jpg"}}
		],
		"resolution":"720p",
		"duration":5,
		"output_format":"mov",
		"return_last_frame":true,
		"priority":3,
		"vendor_field":"keep-me",
		"group":"vip"
	}`
	c, _, info := newNativeContext(t, nativeJSON)
	info.UpstreamModelName = "cdance2.0"

	adaptor := &TaskAdaptor{}
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, body)

	assert.Equal(t, "cdance2.0", payload["model"])
	assert.Equal(t, "mov", payload["output_format"])
	assert.Equal(t, true, payload["return_last_frame"])
	assert.Equal(t, float64(3), payload["priority"])
	assert.Equal(t, "keep-me", payload["vendor_field"])
	assert.NotContains(t, payload, "group")

	content, ok := payload["content"].([]any)
	require.True(t, ok)
	assert.Len(t, content, 2)
	assert.Equal(t, "720p", info.RequestedResolution)
}

func TestBuildRequestBody_draftForces480p(t *testing.T) {
	nativeJSON := `{"model":"cdance2.5","content":[{"type":"text","text":"a cat"}],"draft":true}`
	c, _, info := newNativeContext(t, nativeJSON)
	info.UpstreamModelName = "cdance2.5"
	info.Draft = true

	adaptor := &TaskAdaptor{}
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, body)

	assert.Equal(t, true, payload["draft"])
	assert.Equal(t, "480p", payload["resolution"])
	assert.Equal(t, "480p", info.RequestedResolution)
	assert.Equal(t, "480p", info.UpstreamResolution)
}

// TestBuildRequestBody_finalize 成片请求只能引用原样片的上游任务号，且必须
// 省略 prompt / duration / ratio，避免客户端改掉成片内容与时长。
func TestBuildRequestBody_finalize(t *testing.T) {
	nativeJSON := `{
		"model":"cdance2.5",
		"content":[{"type":"draft_task","draft_task":{"id":"task_public"}}],
		"duration":5,
		"output_format":"mov"
	}`
	c, _, info := newNativeContext(t, nativeJSON)
	info.UpstreamModelName = "cdance2.5"
	info.Action = constant.TaskActionFinalize
	info.OriginUpstreamTaskID = "cgt-origin"

	adaptor := &TaskAdaptor{}
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, body)

	assert.Equal(t, "cdance2.5", payload["model"])
	assert.Equal(t, "1080p", payload["resolution"])
	assert.Equal(t, "mov", payload["output_format"])
	assert.NotContains(t, payload, "duration")
	assert.NotContains(t, payload, "prompt")

	content, ok := payload["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	item, ok := content[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "draft_task", item["type"])
	draftTask, ok := item["draft_task"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "cgt-origin", draftTask["id"])
	assert.Equal(t, "1080p", info.RequestedResolution)
}

// TestDoResponse 创建响应只换 id：下游拿网关号轮询，cgt 号以 upstreamTaskId 带出。
func TestDoResponse(t *testing.T) {
	c, w, info := newNativeContext(t, `{"model":"cdance2.0","content":[{"type":"text","text":"a cat"}]}`)
	info.PublicTaskID = "task_public"

	adaptor := &TaskAdaptor{}
	resp := &http.Response{Body: io.NopCloser(bytes.NewReader([]byte(`{"id":"cgt-123","extra":"kept"}`)))}
	taskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "cgt-123", taskID)
	assert.JSONEq(t, `{"id":"cgt-123","extra":"kept"}`, string(taskData))

	var out map[string]any
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "task_public", out["id"])
	assert.Equal(t, "cgt-123", out["upstreamTaskId"])
	assert.Equal(t, "kept", out["extra"])
}

func TestParseTaskResult(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus model.TaskStatus
		wantURL    string
		wantReason string
	}{
		{
			name:       "排队",
			body:       `{"id":"cgt-1","status":"queued"}`,
			wantStatus: model.TaskStatusQueued,
		},
		{
			name:       "运行中",
			body:       `{"id":"cgt-1","status":"running"}`,
			wantStatus: model.TaskStatusInProgress,
		},
		{
			name:       "成功带地址",
			body:       `{"id":"cgt-1","status":"succeeded","content":{"video_url":"https://x/v.mp4"}}`,
			wantStatus: model.TaskStatusSuccess,
			wantURL:    "https://x/v.mp4",
		},
		{
			name:       "失败带原因",
			body:       `{"id":"cgt-1","status":"failed","error":{"message":"boom"}}`,
			wantStatus: model.TaskStatusFailure,
			wantReason: "boom",
		},
		{
			name:       "上游过期是终态",
			body:       `{"id":"cgt-1","status":"expired"}`,
			wantStatus: model.TaskStatusFailure,
			wantReason: "upstream task expired",
		},
		{
			name:       "上游取消是终态",
			body:       `{"id":"cgt-1","status":"cancelled"}`,
			wantStatus: model.TaskStatusFailure,
			wantReason: "upstream task cancelled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adaptor := &TaskAdaptor{}
			result, err := adaptor.ParseTaskResult([]byte(tt.body))
			require.NoError(t, err)
			assert.Equal(t, string(tt.wantStatus), string(result.Status))
			assert.Equal(t, tt.wantURL, result.Url)
			if tt.wantReason != "" {
				assert.Equal(t, tt.wantReason, result.Reason)
			}
		})
	}
}

// TestConvertToNativeArkTask 响应体以上游原文为准：官方字段与厂商额外字段都
// 原样保留，只把 id 换成网关公开 ID。
func TestConvertToNativeArkTask(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		CreatedAt: 1,
		UpdatedAt: 2,
		Data: []byte(`{
			"id":"cgt-9",
			"model":"cdance2.5",
			"status":"succeeded",
			"content":{"video_url":"https://x/v.mp4","last_frame_url":"https://x/f.jpg"},
			"usage":{"completion_tokens":10,"total_tokens":10},
			"seed":7,
			"framespersecond":24,
			"vendor_extra":"keep-me"
		}`),
	}

	adaptor := &TaskAdaptor{}
	out, err := adaptor.ConvertToNativeArkTask(task)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(out, &payload))
	assert.Equal(t, "task_public", payload["id"])
	assert.Equal(t, "cgt-9", payload["upstreamTaskId"])
	assert.Equal(t, "keep-me", payload["vendor_extra"])
	assert.Equal(t, float64(24), payload["framespersecond"])

	content, ok := payload["content"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://x/v.mp4", content["video_url"])
	assert.Equal(t, "https://x/f.jpg", content["last_frame_url"])
}

// TestConvertToNativeArkTask_fillsOfficialFields 落库数据只有创建响应（仅 id）时，
// 官方字段必须由任务表补齐，不能只回一个 id。
func TestConvertToNativeArkTask_fillsOfficialFields(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusQueued,
		CreatedAt: 1700000000,
		UpdatedAt: 1700000001,
		Data:      []byte(`{"id":"cgt-9"}`),
	}

	adaptor := &TaskAdaptor{}
	out, err := adaptor.ConvertToNativeArkTask(task)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(out, &payload))
	assert.Equal(t, "task_public", payload["id"])
	assert.Equal(t, "cgt-9", payload["upstreamTaskId"])
	assert.Equal(t, "queued", payload["status"])
	assert.Equal(t, float64(1700000000), payload["created_at"])
	assert.Equal(t, float64(1700000001), payload["updated_at"])
}

func TestEstimateBilling(t *testing.T) {
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"video_tier_pricing.price_table": `{"cdance2.5":{"base":100,"1080p":120,"1080p_video":150}}`,
	}))
	t.Cleanup(func() {
		_ = config.GlobalConfig.LoadFromDB(map[string]string{
			"video_tier_pricing.price_table": "{}",
		})
	})

	adaptor := &TaskAdaptor{}

	t.Run("样片按 480p 基准档", func(t *testing.T) {
		c, _, info := newNativeContext(t, `{"model":"cdance2.5","content":[{"type":"text","text":"a cat"}],"draft":true}`)
		info.UpstreamModelName = "cdance2.5"
		info.Draft = true
		assert.Nil(t, adaptor.EstimateBilling(c, info))
	})

	t.Run("成片按 1080p 档", func(t *testing.T) {
		c, _, info := newNativeContext(t, `{"model":"cdance2.5","content":[{"type":"draft_task","draft_task":{"id":"task_public"}}]}`)
		info.UpstreamModelName = "cdance2.5"
		info.Action = constant.TaskActionFinalize
		ratios := adaptor.EstimateBilling(c, info)
		require.Contains(t, ratios, "video_input")
		assert.InDelta(t, 1.2, ratios["video_input"], 1e-9)
	})

	t.Run("含视频输入的成片按 1080p_video 档", func(t *testing.T) {
		c, _, info := newNativeContext(t, `{"model":"cdance2.5","content":[{"type":"draft_task","draft_task":{"id":"task_public"}}]}`)
		info.UpstreamModelName = "cdance2.5"
		info.Action = constant.TaskActionFinalize
		info.OriginHasVideoInput = true
		ratios := adaptor.EstimateBilling(c, info)
		require.Contains(t, ratios, "video_input")
		assert.InDelta(t, 1.5, ratios["video_input"], 1e-9)
	})
}

// TestDoRequestHitsAistorePath 回归 Critical：DoRequest 不能沿用嵌入 doubao 适配器
// 的提升版——那条链路把内层适配器当接收者，BuildRequestURL 会落回 doubao 的
// {base}/api/v3/contents/generations/tasks，而运河启源的版本前缀在 base 里
// （.../v1），提交会 404。这里走真实 DoRequest，由测试服务器记录路径。
func TestDoRequestHitsAistorePath(t *testing.T) {
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cgt-1"}`))
	}))
	defer server.Close()

	c, _, info := newNativeContext(t, `{"model":"cdance2.0","content":[{"type":"text","text":"a cat"}]}`)
	info.ChannelBaseUrl = server.URL + "/v1"
	info.ApiKey = "sk-test"

	service.InitHttpClient() // DoRequest 走的是服务级出站客户端，测试里没有启动流程

	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	resp, err := adaptor.DoRequest(c, info, bytes.NewReader([]byte(`{"model":"cdance2.0"}`)))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, "/v1/contents/generations/tasks", gotPath)
	assert.Equal(t, "Bearer sk-test", gotAuth)
}

// TestBuildRequestBody_rejectsDraftTaskOutsideFinalize 样片引用只能走成片入口：
// 普通提交里带 draft_task 必须拒绝，不能当普通 content 透传（那会绕过成片校验
// 与 1080p 档计费）。
func TestBuildRequestBody_rejectsDraftTaskOutsideFinalize(t *testing.T) {
	c, _, info := newNativeContext(t, `{"model":"cdance2.5","content":[{"type":"draft_task","draft_task":{"id":"task_public"}}]}`)
	info.UpstreamModelName = "cdance2.5"

	adaptor := &TaskAdaptor{}
	_, err := adaptor.BuildRequestBody(c, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only accepted on finalize")
}

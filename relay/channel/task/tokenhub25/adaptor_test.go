package tokenhub25

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSubmitContext(t *testing.T, path, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	return c
}

func newInfo(action string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: action},
		ChannelMeta:   &relaycommon.ChannelMeta{},
	}
}

// readPayload 把 BuildRequestBody 的产物读回成 map，便于断言「上游实际收到什么」。
func readPayload(t *testing.T, body io.Reader) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(raw, &payload))
	return payload
}

// TestDraftRequestForcesDraftAnd480p 样片请求的两条入口（顶层 draft 与
// metadata.draft）都必须让上游收到 draft=true 与 resolution=480p，即使客户端
// 完全没有传 resolution。
func TestDraftRequestForcesDraftAnd480p(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "顶层 draft 参数",
			body: `{"model":"dreamina-seedance-2-5-260628","prompt":"a cat","draft":true}`,
		},
		{
			name: "metadata.draft 且显式 480p",
			body: `{"model":"dreamina-seedance-2-5-260628","prompt":"a cat","metadata":{"draft":true,"resolution":"480p"}}`,
		},
		{
			name: "metadata.draft 字符串 true",
			body: `{"model":"dreamina-seedance-2-5-260628","prompt":"a cat","metadata":{"draft":"true"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newSubmitContext(t, "/v1/videos", tt.body)
			info := newInfo("")

			adaptor := &TaskAdaptor{}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			require.True(t, info.Draft, "样片标记必须落到 info")
			// tokenhub 的 ValidateBasicTaskRequest 写死 generate，本渠道不得覆盖它。
			assert.Equal(t, constant.TaskActionGenerate, info.Action)

			body, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			payload := readPayload(t, body)

			assert.Equal(t, true, payload["draft"])
			assert.Equal(t, Res480p, payload["resolution"])
			assert.Equal(t, Res480p, info.RequestedResolution)
			assert.Equal(t, Res480p, info.UpstreamResolution)
		})
	}
}

// TestDraftRequestRejectsNon480pResolution 样片只允许 480p：其余分辨率明确 400，
// 而不是静默改写成 480p 让客户端以为自己拿到了 1080p。拒绝发生在预扣费之前。
func TestDraftRequestRejectsNon480pResolution(t *testing.T) {
	for _, resolution := range []string{"720p", "1080p", "4k"} {
		t.Run(resolution, func(t *testing.T) {
			c := newSubmitContext(t, "/v1/videos",
				`{"model":"dreamina-seedance-2-5-260628","prompt":"a cat","metadata":{"draft":true,"resolution":"`+resolution+`"}}`)
			info := newInfo("")

			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, "invalid_resolution", taskErr.Code)
			assert.False(t, info.Draft)
		})
	}
}

// TestNonDraftRequestKeepsResolution 非样片提交不受影响：分辨率原样透传，
// 不注入 draft 字段。
func TestNonDraftRequestKeepsResolution(t *testing.T) {
	c := newSubmitContext(t, "/v1/videos",
		`{"model":"dreamina-seedance-2-5-260628","prompt":"a cat","metadata":{"resolution":"1080p"}}`)
	info := newInfo("")

	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.False(t, info.Draft)

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, body)

	assert.Equal(t, "1080p", payload["resolution"])
	_, hasDraft := payload["draft"]
	assert.False(t, hasDraft, "非样片请求不应出现 draft 字段")
}

// TestHasVideoInputCapturedForDraft 样片提交要把「是否含视频输入」记录下来，
// 供第二步成片继承档位。
func TestHasVideoInputCapturedForDraft(t *testing.T) {
	c := newSubmitContext(t, "/v1/videos",
		`{"model":"dreamina-seedance-2-5-260628","prompt":"restyle","draft":true,`+
			`"metadata":{"content":[{"type":"video_url","video_url":{"url":"https://v.mp4"}}]}}`)
	info := newInfo("")

	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	assert.True(t, info.Draft)
	assert.True(t, info.HasVideoInput)
}

// TestFinalizeSubmitAcceptsEmptyBody 成片请求体允许为空或 `{}`——客户端通常
// 直接 POST 空体；只有显式要求非 1080p 才拒绝。
func TestFinalizeSubmitAcceptsEmptyBody(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "空 body", body: "", wantStatus: 0},
		{name: "空对象", body: `{}`, wantStatus: 0},
		{name: "显式 1080p", body: `{"resolution":"1080p"}`, wantStatus: 0},
		{name: "显式 720p 被拒", body: `{"resolution":"720p"}`, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newSubmitContext(t, "/v1/videos/task_x/finalize", tt.body)
			info := newInfo(constant.TaskActionFinalize)

			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			if tt.wantStatus == 0 {
				require.Nil(t, taskErr)
				assert.Equal(t, constant.TaskActionFinalize, info.Action)
				return
			}
			require.NotNil(t, taskErr)
			assert.Equal(t, tt.wantStatus, taskErr.StatusCode)
			assert.Equal(t, "invalid_resolution", taskErr.Code)
		})
	}
}

// TestFinalizePayloadIsWhitelisted 成片请求体的结构本身不表达上游禁止的字段。
// 这条用例同时是「反劫持」断言：客户端塞进来的 content / draft_task / prompt /
// duration / ratio 都不会出现在上游 payload 里，draft_task.id 只来自原任务。
func TestFinalizePayloadIsWhitelisted(t *testing.T) {
	injected := `{"resolution":"1080p","prompt":"hijacked","duration":30,"ratio":"16:9",` +
		`"seed":123,"generate_audio":false,"draft":true,"omni_reference_task_type":"x",` +
		`"content":[{"type":"draft_task","draft_task":{"id":"cgt-attacker"}},{"type":"text","text":"hijacked"}],` +
		`"draft_task":{"id":"cgt-attacker"},"watermark":true}`
	c := newSubmitContext(t, "/v1/videos/task_x/finalize", injected)
	info := newInfo(constant.TaskActionFinalize)
	info.OriginUpstreamTaskID = "cgt-origin"
	info.UpstreamModelName = "dreamina-seedance-2-5-260628"

	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.Equal(t, constant.TaskActionFinalize, info.Action,
		"tokenhub 写死的 generate 不得覆盖 finalize")

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, body)

	assert.Equal(t, "dreamina-seedance-2-5-260628", payload["model"])
	assert.Equal(t, Res1080p, payload["resolution"])
	// 唯一允许被重设的输出规格参数仍然生效
	assert.Equal(t, true, payload["watermark"])

	content, ok := payload["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1, "成片 payload 只能有一个 draft_task 引用项")
	item, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Len(t, item, 2, "content 项只能是 draft_task 引用，不得带出其它键")
	assert.Equal(t, "draft_task", item["type"])

	draftTask, ok := item["draft_task"].(map[string]any)
	require.True(t, ok)
	require.Len(t, draftTask, 1)
	assert.Equal(t, "cgt-origin", draftTask["id"], "draft_task.id 必须来自原任务，客户端无法覆盖")

	for _, forbidden := range []string{
		"prompt", "duration", "ratio", "seed", "generate_audio", "omni_reference_task_type", "draft",
	} {
		_, exists := payload[forbidden]
		assert.False(t, exists, "成片 payload 不得包含 %s", forbidden)
	}
	assert.Equal(t, Res1080p, info.RequestedResolution)
	assert.Equal(t, Res1080p, info.UpstreamResolution)
}

// TestFinalizeInheritsHasVideoInputForBilling 成片请求体里没有 content，
// 计费档位必须继承第一步样片的输入维度，而不是从（空的）当次请求推断。
func TestFinalizeInheritsHasVideoInputForBilling(t *testing.T) {
	c := newSubmitContext(t, "/v1/videos/task_x/finalize", "{}")
	info := newInfo(constant.TaskActionFinalize)
	info.OriginHasVideoInput = true

	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	// 未配置档位价格表时返回 nil；这里只断言「继承值确实被读走」——
	// 具体倍率由 taskcommon 的价格表用例覆盖。
	assert.True(t, info.OriginHasVideoInput)
	assert.Nil(t, adaptor.EstimateBilling(c, info))
}

// TestRequestURLUsesTokenHubV2Path 华鲲走 /api/v2（doubao 是 /api/v3），
// base_url 只配主机。两步打同一个端点。
func TestRequestURLUsesTokenHubV2Path(t *testing.T) {
	info := newInfo(constant.TaskActionFinalize)
	info.ChannelMeta.ChannelBaseUrl = "https://ai-tokenhub.ai"

	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://ai-tokenhub.ai/api/v2/contents/generations/tasks", url)
}

// TestFinalizeWithoutOriginUpstreamIDFails 兜底：没有上游样片 ID 时不得发出
// 一个无 id 的 draft_task 引用（那会白跑一次预扣费）。
func TestFinalizeWithoutOriginUpstreamIDFails(t *testing.T) {
	c := newSubmitContext(t, "/v1/videos/task_x/finalize", "{}")
	info := newInfo(constant.TaskActionFinalize)

	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	_, err := adaptor.BuildRequestBody(c, info)
	require.Error(t, err)
}

// TestModelListIsSeedance25Only 该渠道只承载 2.5；2.0 三条继续由 61 承载。
func TestModelListIsSeedance25Only(t *testing.T) {
	assert.Equal(t, []string{"dreamina-seedance-2-5-260628"}, (&TaskAdaptor{}).GetModelList())
	assert.Equal(t, "tokenhub-25", (&TaskAdaptor{}).GetChannelName())
}

// TestOmniReferenceTaskTypeIsForwarded 该字段不在 tokenhub 的共用白名单里（只对
// 2.5 有意义），必须由本渠道补写到上游 payload；补写不得影响同一请求的其它字段。
func TestOmniReferenceTaskTypeIsForwarded(t *testing.T) {
	c := newSubmitContext(t, "/v1/videos",
		`{"model":"dreamina-seedance-2-5-260628","prompt":"a cat","seconds":"12",`+
			`"metadata":{"resolution":"720p","ratio":"9:16","omni_reference_task_type":"reference"}}`)
	info := newInfo("")

	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, body)

	assert.Equal(t, "reference", payload[OmniReferenceTaskTypeKey])
	assert.Equal(t, float64(12), payload["duration"], "补写不得损坏 duration")
	assert.Equal(t, "720p", payload["resolution"])
	assert.Equal(t, "9:16", payload["ratio"])
}

// TestOmniReferenceTaskTypeAbsentWhenNotSubmitted 客户端没传时不得凭空构造该键，
// 否则上游行为与 61 不再一致。
func TestOmniReferenceTaskTypeAbsentWhenNotSubmitted(t *testing.T) {
	c := newSubmitContext(t, "/v1/videos",
		`{"model":"dreamina-seedance-2-5-260628","prompt":"a cat","metadata":{"resolution":"720p"}}`)
	info := newInfo("")

	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, body)

	_, exists := payload[OmniReferenceTaskTypeKey]
	assert.False(t, exists, "未提交时 payload 不得出现 %s", OmniReferenceTaskTypeKey)
}

// TestOmniReferenceTaskTypeRejectsNonString 该字段曾经被静默丢弃、客户端无从得知
// 意图没生效；类型不对时必须明确 400，且发生在预扣费之前。
// 取值本身（auto / reference / edit / extend 及各自约束）交给上游判定。
func TestOmniReferenceTaskTypeRejectsNonString(t *testing.T) {
	c := newSubmitContext(t, "/v1/videos",
		`{"model":"dreamina-seedance-2-5-260628","prompt":"a cat",`+
			`"metadata":{"omni_reference_task_type":123}}`)
	info := newInfo("")

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "invalid_omni_reference_task_type", taskErr.Code)
}

// TestOmniReferenceTaskTypeForwardedForDraft 样片提交同样要带上该字段——任务类型
// 判定对样片与成片两条路径一致，样片也可能被判为编辑 / 延长任务。
func TestOmniReferenceTaskTypeForwardedForDraft(t *testing.T) {
	c := newSubmitContext(t, "/v1/videos",
		`{"model":"dreamina-seedance-2-5-260628","prompt":"a cat","draft":true,`+
			`"metadata":{"omni_reference_task_type":"edit"}}`)
	info := newInfo("")

	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.True(t, info.Draft)

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload := readPayload(t, body)

	assert.Equal(t, "edit", payload[OmniReferenceTaskTypeKey])
	assert.Equal(t, true, payload["draft"])
	assert.Equal(t, Res480p, payload["resolution"])
}

package volcengine_plus

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	taskdoubao "github.com/QuantumNous/new-api/relay/channel/task/doubao"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSubmitFixture builds the gin context and RelayInfo a task submit validation
// sees: body carrying the request, and the client-requested model on RelayInfo.
func newSubmitFixture(t *testing.T, body, originModel string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: originModel,
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}
	return c, info
}

func TestValidateRequestRejectsUnsupportedResolutionPerModel(t *testing.T) {
	tests := []struct {
		model      string
		resolution string
		wantCode   string
	}{
		{model: "doubao-seedance-2-0-260128", resolution: Res4k},
		{model: "doubao-seedance-2-5-260628", resolution: Res1080p},
		{model: "doubao-seedance-2-0-fast-260128", resolution: Res720p},

		{model: "doubao-seedance-2-5-260628", resolution: Res4k, wantCode: "invalid_resolution"},
		{model: "doubao-seedance-2-0-fast-260128", resolution: Res4k, wantCode: "invalid_resolution"},
		{model: "doubao-seedance-2-0-mini-260615", resolution: Res4k, wantCode: "invalid_resolution"},
		{model: "doubao-seedance-2-0-fast-260128", resolution: Res1080p, wantCode: "invalid_resolution"},
		{model: "doubao-seedance-2-0-mini-260615", resolution: Res1080p, wantCode: "invalid_resolution"},
		{model: "doubao-seedance-1-0-pro-250528", resolution: Res720p, wantCode: "invalid_resolution"},
		{model: "doubao-seedance-1-5-pro-251215", resolution: Res1080p, wantCode: "invalid_resolution"},
	}
	for _, tt := range tests {
		t.Run(tt.model+"/"+tt.resolution, func(t *testing.T) {
			body := fmt.Sprintf(
				`{"model":%q,"prompt":"a cat","metadata":{"resolution":%q,"ratio":"16:9"}}`,
				tt.model, tt.resolution)
			c, info := newSubmitFixture(t, body, tt.model)
			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
			if tt.wantCode == "" {
				require.Nil(t, taskErr)
				return
			}
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, tt.wantCode, taskErr.Code)
		})
	}
}

func TestValidateRequestRejectsDraft(t *testing.T) {
	bodies := []string{
		`{"model":"doubao-seedance-2-5-260628","prompt":"a cat","draft":true,"metadata":{"resolution":"480p","ratio":"16:9"}}`,
		`{"model":"doubao-seedance-2-5-260628","prompt":"a cat","metadata":{"draft":true,"resolution":"480p","ratio":"16:9"}}`,
		`{"model":"doubao-seedance-2-5-260628","prompt":"a cat","metadata":{"draft":"true","resolution":"480p","ratio":"16:9"}}`,
	}
	for _, body := range bodies {
		c, info := newSubmitFixture(t, body, "doubao-seedance-2-5-260628")
		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
		require.NotNil(t, taskErr, body)
		assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		assert.Equal(t, "draft_not_supported", taskErr.Code)
	}

	// Explicit draft=false is an ordinary submit.
	body := `{"model":"doubao-seedance-2-5-260628","prompt":"a cat","draft":false,"metadata":{"resolution":"1080p","ratio":"16:9"}}`
	c, info := newSubmitFixture(t, body, "doubao-seedance-2-5-260628")
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
}

func TestBuildRequestBody4kLotteryRewritesResolution(t *testing.T) {
	body := `{"model":"doubao-seedance-2-0-260128","prompt":"a cat","metadata":{"resolution":"4k","ratio":"16:9"}}`
	c, info := newSubmitFixture(t, body, "doubao-seedance-2-0-260128")
	info.ChannelMeta.ChannelSetting = dto.ChannelSettings{Lottery4kTo1080Percent: 100}
	a := &TaskAdaptor{randIntn: func(int) int { return 0 }}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))

	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(raw, &payload))
	assert.Equal(t, Res1080p, payload["resolution"])
	assert.Equal(t, Res4k, info.RequestedResolution)
	assert.Equal(t, Res1080p, info.UpstreamResolution)

	req, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	assert.Equal(t, Res4k, req.Metadata["resolution"])
}

func TestGetModelListCoversTierTwentyFiveAndMini(t *testing.T) {
	list := (&TaskAdaptor{}).GetModelList()
	assert.Contains(t, list, "doubao-seedance-2-5-260628")
	assert.Contains(t, list, "doubao-seedance-2-0-mini-260615")
	assert.NotContains(t, list, "doubao-seedance-1-0-pro-250528")
	assert.NotContains(t, list, "doubao-seedance-1-5-pro-251215")

	// The 1.0 / 1.5 families are rejected at submit even if routed here anyway.
	for _, model := range list {
		assert.True(t, ResolutionSupported(model, Res480p), model)
		assert.True(t, ResolutionSupported(model, Res720p), model)
	}
}

func TestModelListDoesNotLeakIntoDoubaoChannels(t *testing.T) {
	for _, model := range ModelList {
		if model == "doubao-seedance-2-0-260128" || model == "doubao-seedance-2-0-fast-260128" {
			continue
		}
		assert.NotContains(t, taskdoubao.ModelList, model)
	}
}

func TestBuildRequestBodyRewritesUpstreamResolutionOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"doubao-seedance-2-0-260128","prompt":"a cat","metadata":{"resolution":"1080p","ratio":"16:9"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				Lottery1080To720Percent: 100,
			},
		},
	}
	a := &TaskAdaptor{randIntn: func(int) int { return 0 }}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))

	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(raw, &payload))
	assert.Equal(t, Res720p, payload["resolution"])
	assert.Equal(t, Res1080p, info.RequestedResolution)
	assert.Equal(t, Res720p, info.UpstreamResolution)

	req, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	assert.Equal(t, Res1080p, req.Metadata["resolution"])
}

func TestValidateRequestRejectsUnknownRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"doubao-seedance-2-0-260128","prompt":"a cat","metadata":{"resolution":"1080p","ratio":"2:1"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestExtractTaskUsageUsesPeeledTokens(t *testing.T) {
	taskData, err := common.Marshal(map[string]any{
		"usage": map[string]any{"completion_tokens": 243000, "total_tokens": 243000},
	})
	require.NoError(t, err)
	task := taskWithDowngrade()
	task.Data = taskData
	task.PrivateData.PeeledTotalTokens = 243000
	usage := (&TaskAdaptor{}).ExtractTaskUsage(task)
	require.NotNil(t, usage)
	assert.Equal(t, 243000, usage.TotalTokens)
	assert.Equal(t, 243000, usage.CompletionTokens)
}

func TestValidateRequestRejectsRemix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionRemix}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "remix_not_supported", taskErr.Code)
}

func TestValidateRequestAcceptsEmptyRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"doubao-seedance-2-0-260128","prompt":"a cat","metadata":{"resolution":"1080p"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{}}
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.Nil(t, taskErr)
}

func TestAdjustBillingOnCompletePeelsTokens(t *testing.T) {
	task := taskWithDowngrade()
	result := &relaycommon.TaskInfo{TotalTokens: 108000, CompletionTokens: 108000}
	got := (&TaskAdaptor{}).AdjustBillingOnComplete(task, result)
	assert.Equal(t, 0, got)
	assert.Equal(t, 243000, result.TotalTokens)
	assert.Equal(t, 243000, result.CompletionTokens)
	assert.Equal(t, 243000, task.PrivateData.PeeledTotalTokens)
	assert.Equal(t, 108000, task.PrivateData.UpstreamTotalTokens)
}

func TestConvertToOpenAIVideoHidesUpstreamURL(t *testing.T) {
	task := taskWithDowngrade()
	task.Status = model.TaskStatusInProgress
	raw, err := common.Marshal(map[string]any{
		"id":         "upstream-id",
		"status":     "succeeded",
		"resolution": Res720p,
		"content":    map[string]any{"video_url": "https://up.example/720.mp4"},
		"usage":      map[string]any{"completion_tokens": 108000, "total_tokens": 108000},
	})
	require.NoError(t, err)
	task.Data = raw
	out, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "720p")
	assert.NotContains(t, string(out), "https://up.example/720.mp4")
	assert.NotContains(t, string(out), "108000")
}

func TestDoResponseRedactsSubmitBody(t *testing.T) {
	raw := []byte(`{"id":"seed-1","status":"queued","resolution":"720p"}`)
	got := rewriteUserVisibleTaskData(raw, Res1080p, 0, 0, "", true, false)
	var m map[string]any
	require.NoError(t, common.Unmarshal(got, &m))
	assert.Equal(t, Res1080p, m["resolution"])
	assert.NotContains(t, string(got), "720p")
}

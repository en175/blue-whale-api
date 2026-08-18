package doubaoaudio

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The url-fetch path uses the general relay outbound client, which is only
// materialized by InitHttpClient (normally called at boot). Initialize it once
// before the suite so any test exercising the outbound client sees a non-nil
// instance.
func TestMain(m *testing.M) {
	service.InitHttpClient()
	os.Exit(m.Run())
}

func TestGetRequestURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "channel base url",
			baseURL: "https://my-proxy.example.com",
			want:    "https://my-proxy.example.com/api/v3/tts/create",
		},
		{
			name:    "default upstream base url",
			baseURL: "",
			want:    "https://openspeech.bytedance.com/api/v3/tts/create",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adaptor := &Adaptor{}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: tt.baseURL},
			}
			got, err := adaptor.GetRequestURL(info)
			require.Nil(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSetupRequestHeader(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "volc-secret-key"}}
	req := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &req, info))

	assert.Equal(t, "application/json", req.Get("Content-Type"))
	assert.Equal(t, "volc-secret-key", req.Get("X-Api-Key"))
}

func TestConvertAudioRequestPassthrough(t *testing.T) {
	t.Parallel()

	// The client writes the seed-audio protocol directly at the top level.
	// The gateway must forward every field verbatim, locking only the model.
	rawBody := []byte(`{
		"model": "evil-model",
		"text_prompt": "hello seed-audio",
		"speaker": "zh_female_linjiemei",
		"references": [{"speaker": "ref1", "audio_url": "https://example.com/ref.mp3"}],
		"audio_config": {
			"format": "ogg_opus",
			"sample_rate": 24000,
			"speech_rate": 50,
			"loudness_rate": 120,
			"enable_subtitle": true
		},
		"watermark": {"text": "bw"}
	}`)

	c := newContextWithBody(t, rawBody)
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"}}
	got, err := adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{})
	require.Nil(t, err)

	body, err := io.ReadAll(got)
	require.Nil(t, err)

	var req map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(body, &req))

	// model is locked to the upstream model; the client value cannot override it.
	assert.JSONEq(t, `"seed-audio-1.0"`, string(req["model"]))
	// every upstream field survives verbatim.
	assert.JSONEq(t, `"hello seed-audio"`, string(req["text_prompt"]))
	assert.JSONEq(t, `"zh_female_linjiemei"`, string(req["speaker"]))
	assert.JSONEq(t, `[{"speaker":"ref1","audio_url":"https://example.com/ref.mp3"}]`, string(req["references"]))
	assert.JSONEq(t, `{"format":"ogg_opus","sample_rate":24000,"speech_rate":50,"loudness_rate":120,"enable_subtitle":true}`, string(req["audio_config"]))
	assert.JSONEq(t, `{"text":"bw"}`, string(req["watermark"]))
}

func TestConvertAudioRequestNoOpenspeechFields(t *testing.T) {
	t.Parallel()

	// Only model is supplied: the passthrough forwards a body with just the
	// locked model, leaving any absent upstream fields absent.
	rawBody := []byte(`{"model": "client-model", "text_prompt": "hi"}`)

	c := newContextWithBody(t, rawBody)
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"}}
	got, err := adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{})
	require.Nil(t, err)

	body, err := io.ReadAll(got)
	require.Nil(t, err)

	var req map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(body, &req))
	assert.JSONEq(t, `"seed-audio-1.0"`, string(req["model"]))
	assert.JSONEq(t, `"hi"`, string(req["text_prompt"]))
	assert.NotContains(t, req, "references")
	assert.NotContains(t, req, "audio_config")
}

func TestConvertAudioRequestInvalidBody(t *testing.T) {
	t.Parallel()

	rawBody := []byte(`{"text_prompt": `) // malformed JSON
	c := newContextWithBody(t, rawBody)
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"}}
	_, err := adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{})
	require.Error(t, err)
}

func TestDoResponsePassthroughJSON(t *testing.T) {
	t.Parallel()

	// The upstream returns its native JSON: url/duration/original_duration.
	// The gateway forwards that body verbatim and bills by duration.
	seedBody := mustMarshal(t, map[string]any{
		"code":              0,
		"message":           "success",
		"url":               "https://oss.example.com/audio.mp3",
		"duration":          82.8,
		"original_duration": 82.8,
	})

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	info.SetEstimatePromptTokens(100)
	recorder, c, apiErr := runDoResponse(t, info, seedBody)
	require.Nil(t, apiErr)

	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	// body forwarded verbatim.
	assert.JSONEq(t, string(seedBody), recorder.Body.String())

	usage, ok := getUsage(t, c)
	require.True(t, ok)
	assert.Equal(t, 83, usage.PromptTokens) // ceil(82.8)
	assert.Equal(t, 83, usage.TotalTokens)
	assert.Nil(t, info.QuotaClamp)
}

func TestDoResponseFallsBackToEstimate(t *testing.T) {
	t.Parallel()

	// original_duration absent (0) means we cannot bill by duration, so the
	// pre-consume estimate is used unchanged.
	seedBody := mustMarshal(t, map[string]any{
		"code":     0,
		"message":  "success",
		"url":      "https://oss.example.com/audio.mp3",
		"duration": 0,
	})

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	info.SetEstimatePromptTokens(42)
	recorder, c, apiErr := runDoResponse(t, info, seedBody)
	require.Nil(t, apiErr)

	assert.JSONEq(t, string(seedBody), recorder.Body.String())
	usage, ok := getUsage(t, c)
	require.True(t, ok)
	assert.Equal(t, 42, usage.PromptTokens)
	assert.Nil(t, info.QuotaClamp)
}

func TestDoResponseUpstreamError(t *testing.T) {
	t.Parallel()

	seedBody := mustMarshal(t, map[string]any{
		"code":    40001,
		"message": "invalid request param",
	})

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	_, _, apiErr := runDoResponse(t, info, seedBody)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "invalid request param")
}

func TestDoResponseClampsHugeDuration(t *testing.T) {
	t.Parallel()

	// An upstream-controlled duration beyond the int32 quota range must saturate
	// and surface a QuotaClamp for audit instead of wrapping around.
	seedBody := mustMarshal(t, map[string]any{
		"code":              0,
		"message":           "success",
		"url":               "https://oss.example.com/audio.mp3",
		"duration":          3e9,
		"original_duration": 3e9,
	})

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	_, c, apiErr := runDoResponse(t, info, seedBody)
	require.Nil(t, apiErr)

	usage, ok := getUsage(t, c)
	require.True(t, ok)
	assert.Equal(t, math.MaxInt32, usage.PromptTokens)
	require.NotNil(t, info.QuotaClamp)
	assert.Equal(t, math.MaxInt32, info.QuotaClamp.Clamped)
}

// newContextWithBody builds a gin test context whose body storage is seeded
// from rawBody, mirroring what UnmarshalBodyReusable leaves in place for the
// adaptor to re-read.
func newContextWithBody(t *testing.T, rawBody []byte) *gin.Context {
	t.Helper()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", bytes.NewReader(rawBody))

	storage, err := common.CreateBodyStorage(rawBody)
	require.Nil(t, err)
	c.Set(common.KeyBodyStorage, storage)
	return c
}

// runDoResponse drives a single DoResponse call and returns the recorder
// (carrying the forwarded body), the gin context (usage stashed under
// "usage" on success), and the API error (nil on success).
func runDoResponse(t *testing.T, info *relaycommon.RelayInfo, seedBody []byte) (*httptest.ResponseRecorder, *gin.Context, *types.NewAPIError) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       nopCloser(bytes.NewReader(seedBody)),
	}

	adaptor := &Adaptor{}
	usage, apiErr := adaptor.DoResponse(c, resp, info)
	if apiErr == nil {
		c.Set("usage", usage)
	}
	return recorder, c, apiErr
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()

	body, err := common.Marshal(v)
	require.Nil(t, err)
	return body
}

// getUsage extracts the usage stashed on the gin context by runDoResponse.
func getUsage(t *testing.T, c *gin.Context) (*dto.Usage, bool) {
	t.Helper()

	raw, exists := c.Get("usage")
	if !exists {
		return nil, false
	}
	usage, ok := raw.(*dto.Usage)
	return usage, ok
}

type nopReadCloser struct {
	io.Reader
}

func (nopReadCloser) Close() error {
	return nil
}

func nopCloser(r io.Reader) nopReadCloser {
	return nopReadCloser{Reader: r}
}

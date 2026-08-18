package doubaoaudio

import (
	"bytes"
	"encoding/base64"
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
// before the suite so any test exercising fetchAudio sees a non-nil client.
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

func TestConvertAudioRequestBasic(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"}}
	speed := 1.5
	request := dto.AudioRequest{
		Model:          "seed-audio-1.0",
		Input:          "你好世界",
		Voice:          "zh_female_linjiemei",
		ResponseFormat: "wav",
		Speed:          &speed,
	}

	got, err := adaptor.ConvertAudioRequest(
		gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()),
		info, request,
	)
	require.Nil(t, err)

	req := unmarshalSeedAudioRequest(t, got)
	assert.Equal(t, "seed-audio-1.0", req.Model)
	assert.Equal(t, "你好世界", req.TextPrompt)
	assert.Equal(t, "zh_female_linjiemei", req.Speaker)
	require.NotNil(t, req.AudioConfig)
	assert.Equal(t, "wav", req.AudioConfig.Format)
	require.NotNil(t, req.AudioConfig.SpeechRate)
	assert.Equal(t, 1, *req.AudioConfig.SpeechRate)
	assert.Nil(t, req.AudioConfig.SampleRate)
}

func TestConvertAudioRequestDefaultFormat(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"}}
	request := dto.AudioRequest{Input: "hello"}

	got, err := adaptor.ConvertAudioRequest(
		gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()),
		info, request,
	)
	require.Nil(t, err)

	req := unmarshalSeedAudioRequest(t, got)
	require.NotNil(t, req.AudioConfig)
	assert.Equal(t, "mp3", req.AudioConfig.Format)
}

func TestConvertAudioRequestMetadataMerge(t *testing.T) {
	t.Parallel()

	// Metadata follows the task-channel semantics: model is locked, advanced
	// keys override, and the OpenAI-standard fields are re-applied so they win.
	metadata := json.RawMessage(`{
		"model": "evil-model",
		"text_prompt": "meta-prompt",
		"speaker": "meta-voice",
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
	speed := 2.0
	request := dto.AudioRequest{
		Model:          "client-model",
		Input:          "real input",
		Voice:          "real-voice",
		ResponseFormat: "wav",
		Speed:          &speed,
		Metadata:       metadata,
	}

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"}}
	got, err := adaptor.ConvertAudioRequest(
		gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()),
		info, request,
	)
	require.Nil(t, err)

	req := unmarshalSeedAudioRequest(t, got)
	// model is locked to the upstream model, metadata cannot override it.
	assert.Equal(t, "seed-audio-1.0", req.Model)
	// standard fields win over metadata.
	assert.Equal(t, "real input", req.TextPrompt)
	assert.Equal(t, "real-voice", req.Speaker)
	require.NotNil(t, req.AudioConfig)
	assert.Equal(t, "wav", req.AudioConfig.Format)
	require.NotNil(t, req.AudioConfig.SpeechRate)
	assert.Equal(t, 2, *req.AudioConfig.SpeechRate)
	// advanced fields from metadata take effect.
	require.NotNil(t, req.AudioConfig.SampleRate)
	assert.Equal(t, 24000, *req.AudioConfig.SampleRate)
	require.NotNil(t, req.AudioConfig.LoudnessRate)
	assert.Equal(t, 120, *req.AudioConfig.LoudnessRate)
	require.NotNil(t, req.AudioConfig.EnableSubtitle)
	assert.True(t, *req.AudioConfig.EnableSubtitle)
	require.Len(t, req.References, 1)
	assert.Equal(t, "ref1", req.References[0].Speaker)
	assert.Equal(t, "https://example.com/ref.mp3", req.References[0].AudioURL)
	require.NotNil(t, req.Watermark)
	assert.Equal(t, `{"text":"bw"}`, string(req.Watermark))
}

func TestConvertAudioRequestMetadataPartialAudioConfig(t *testing.T) {
	t.Parallel()

	// A partial audio_config from metadata must not drop the response-format
	// mapping re-applied from the OpenAI-standard field.
	metadata := json.RawMessage(`{"audio_config": {"sample_rate": 16000}}`)
	request := dto.AudioRequest{
		Input:          "hello",
		ResponseFormat: "ogg_opus",
		Metadata:       metadata,
	}

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"}}
	got, err := adaptor.ConvertAudioRequest(
		gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()),
		info, request,
	)
	require.Nil(t, err)

	req := unmarshalSeedAudioRequest(t, got)
	require.NotNil(t, req.AudioConfig)
	assert.Equal(t, "ogg_opus", req.AudioConfig.Format)
	require.NotNil(t, req.AudioConfig.SampleRate)
	assert.Equal(t, 16000, *req.AudioConfig.SampleRate)
	assert.Nil(t, req.AudioConfig.SpeechRate)
}

func TestConvertAudioRequestInvalidMetadata(t *testing.T) {
	t.Parallel()

	request := dto.AudioRequest{
		Input:    "hello",
		Metadata: json.RawMessage(`{"text_prompt": `), // malformed
	}
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"}}
	_, err := adaptor.ConvertAudioRequest(
		gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()),
		info, request,
	)
	require.Error(t, err)
}

func TestDoResponseFromURL(t *testing.T) {
	t.Parallel()

	audioBytes := []byte("fake-mp3-bytes")
	audioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(audioBytes)
	}))
	defer audioServer.Close()

	seedBody := mustMarshal(t, map[string]any{
		"code":              0,
		"message":           "success",
		"url":               audioServer.URL,
		"duration":          82.8,
		"original_duration": 82.8,
	})

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
	}
	info.SetEstimatePromptTokens(100)
	usage, err := runDoResponse(t, "wav", info, seedBody)
	require.Nil(t, err)
	require.NotNil(t, usage)

	u, ok := usage.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 83, u.PromptTokens) // ceil(82.8)
	assert.Equal(t, 83, u.TotalTokens)
	assert.Nil(t, info.QuotaClamp)
}

func TestDoResponseFromBase64(t *testing.T) {
	t.Parallel()

	audioBytes := []byte("fake-pcm-bytes")
	seedBody := mustMarshal(t, map[string]any{
		"code":              0,
		"message":           "success",
		"audio":             base64.StdEncoding.EncodeToString(audioBytes),
		"duration":          3.0,
		"original_duration": 3.0,
	})

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	usage, err := runDoResponse(t, "pcm", info, seedBody)
	require.Nil(t, err)
	require.NotNil(t, usage)

	u, ok := usage.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 3, u.PromptTokens)
}

func TestDoResponseFallsBackToEstimate(t *testing.T) {
	t.Parallel()

	// original_duration absent (0) means we cannot bill by duration, so the
	// pre-consume estimate is used unchanged. Base64 audio keeps the test
	// hermetic (no outbound fetch).
	audioBytes := []byte("fake-pcm-bytes")
	seedBody := mustMarshal(t, map[string]any{
		"code":     0,
		"message":  "success",
		"audio":    base64.StdEncoding.EncodeToString(audioBytes),
		"duration": 0,
	})

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	info.SetEstimatePromptTokens(42)
	usage, err := runDoResponse(t, "", info, seedBody)
	require.Nil(t, err)
	require.NotNil(t, usage)

	u, ok := usage.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 42, u.PromptTokens)
	assert.Nil(t, info.QuotaClamp)
}

func TestDoResponseUpstreamError(t *testing.T) {
	t.Parallel()

	seedBody := mustMarshal(t, map[string]any{
		"code":    40001,
		"message": "invalid request param",
	})

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	_, apiErr := runDoResponse(t, "", info, seedBody)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "invalid request param")
}

func TestDoResponseNoAudio(t *testing.T) {
	t.Parallel()

	seedBody := mustMarshal(t, map[string]any{
		"code":    0,
		"message": "success",
	})

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	_, apiErr := runDoResponse(t, "", info, seedBody)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "no audio data")
}

func TestDoResponseClampsHugeDuration(t *testing.T) {
	t.Parallel()

	// An upstream-controlled duration beyond the int32 quota range must saturate
	// and surface a QuotaClamp for audit instead of wrapping around. Base64 audio
	// keeps the test hermetic (no outbound fetch).
	audioBytes := []byte("fake-pcm-bytes")
	seedBody := mustMarshal(t, map[string]any{
		"code":              0,
		"message":           "success",
		"audio":             base64.StdEncoding.EncodeToString(audioBytes),
		"duration":          3e9,
		"original_duration": 3e9,
	})

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	usage, err := runDoResponse(t, "", info, seedBody)
	require.Nil(t, err)
	require.NotNil(t, usage)

	u, ok := usage.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, math.MaxInt32, u.PromptTokens)
	require.NotNil(t, info.QuotaClamp)
	assert.Equal(t, math.MaxInt32, info.QuotaClamp.Clamped)
}

func TestDoResponseContentTypeAndBody(t *testing.T) {
	t.Parallel()

	audioBytes := []byte("fake-wav-bytes")
	seedBody := mustMarshal(t, map[string]any{
		"code":              0,
		"message":           "success",
		"audio":             base64.StdEncoding.EncodeToString(audioBytes),
		"duration":          1.0,
		"original_duration": 1.0,
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(contextKeyResponseFormat, "wav")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       nopCloser(bytes.NewReader(seedBody)),
	}

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	_, err := adaptor.DoResponse(c, resp, info)
	require.Nil(t, err)

	assert.Equal(t, "audio/wav", recorder.Header().Get("Content-Type"))
	assert.Equal(t, audioBytes, recorder.Body.Bytes())
}

func TestDoResponseDefaultContentType(t *testing.T) {
	t.Parallel()

	// No response_format means the default mp3: the audio_config.format in the
	// request and the response Content-Type header must both be audio/mpeg.
	audioBytes := []byte("fake-mp3-bytes")
	seedBody := mustMarshal(t, map[string]any{
		"code":              0,
		"message":           "success",
		"audio":             base64.StdEncoding.EncodeToString(audioBytes),
		"duration":          1.0,
		"original_duration": 1.0,
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       nopCloser(bytes.NewReader(seedBody)),
	}

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	_, err := adaptor.DoResponse(c, resp, info)
	require.Nil(t, err)

	assert.Equal(t, "audio/mpeg", recorder.Header().Get("Content-Type"))
	assert.Equal(t, audioBytes, recorder.Body.Bytes())
}

// runDoResponse drives a single DoResponse call, returning the usage and the
// API error (nil when the call succeeded).
func runDoResponse(t *testing.T, responseFormat string, info *relaycommon.RelayInfo, seedBody []byte) (any, *types.NewAPIError) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(contextKeyResponseFormat, responseFormat)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       nopCloser(bytes.NewReader(seedBody)),
	}

	adaptor := &Adaptor{}
	return adaptor.DoResponse(c, resp, info)
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()

	body, err := common.Marshal(v)
	require.Nil(t, err)
	return body
}

func unmarshalSeedAudioRequest(t *testing.T, got io.Reader) *seedAudioRequest {
	t.Helper()

	body, err := io.ReadAll(got)
	require.Nil(t, err)
	var req seedAudioRequest
	require.NoError(t, common.Unmarshal(body, &req))
	return &req
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

package doubaoaudio

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"

	channelconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const contextKeyResponseFormat = "doubao_audio_response_format"

// seedAudioRequest mirrors the 豆包语音·音频生成大模型 create body
// (POST /api/v3/tts/create). Optional scalars use pointers so explicit zero
// values survive marshal and absent ones are omitted.
type seedAudioRequest struct {
	Model       string               `json:"model"`
	TextPrompt  string               `json:"text_prompt"`
	References  []seedAudioReference `json:"references,omitempty"`
	Speaker     string               `json:"speaker,omitempty"`
	AudioData   string               `json:"audio_data,omitempty"`
	AudioURL    string               `json:"audio_url,omitempty"`
	ImageData   string               `json:"image_data,omitempty"`
	ImageURL    string               `json:"image_url,omitempty"`
	AudioConfig *audioConfig         `json:"audio_config,omitempty"`
	Watermark   json.RawMessage      `json:"watermark,omitempty"`
}

type seedAudioReference struct {
	Speaker   string `json:"speaker,omitempty"`
	AudioData string `json:"audio_data,omitempty"`
	AudioURL  string `json:"audio_url,omitempty"`
}

type audioConfig struct {
	Format         string `json:"format,omitempty"`
	SampleRate     *int   `json:"sample_rate,omitempty"`
	SpeechRate     *int   `json:"speech_rate,omitempty"`
	LoudnessRate   *int   `json:"loudness_rate,omitempty"`
	PitchRate      *int   `json:"pitch_rate,omitempty"`
	EnableSubtitle *bool  `json:"enable_subtitle,omitempty"`
}

type seedAudioResponse struct {
	Code             int     `json:"code"`
	Message          string  `json:"message"`
	Audio            string  `json:"audio"`
	Duration         float64 `json:"duration"`
	OriginalDuration float64 `json:"original_duration"`
	URL              string  `json:"url"`
}

type Adaptor struct{}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseUrl := info.ChannelBaseUrl
	if baseUrl == "" {
		baseUrl = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeDoubaoAudio]
	}
	return fmt.Sprintf("%s/api/v3/tts/create", baseUrl), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Content-Type", "application/json")
	req.Set("X-Api-Key", info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("doubaoaudio channel only supports audio speech")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("doubaoaudio channel only supports audio speech")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("doubaoaudio channel only supports audio speech")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	// The OpenAI-standard fields are applied below and re-applied after the
	// metadata merge, so the base only needs the audio_config allocation.
	req := seedAudioRequest{
		AudioConfig: &audioConfig{},
	}

	// Metadata follows the task-channel merge semantics (see taskcommon):
	// model is locked, remaining top-level keys override, then the
	// OpenAI-standard fields are re-applied so they win.
	if len(request.Metadata) > 0 {
		var meta map[string]any
		if err := common.Unmarshal(request.Metadata, &meta); err != nil {
			return nil, fmt.Errorf("unmarshal metadata failed: %w", err)
		}
		if err := taskcommon.UnmarshalMetadata(meta, &req); err != nil {
			return nil, err
		}
		if req.AudioConfig == nil {
			req.AudioConfig = &audioConfig{}
		}
	}

	req.Model = info.UpstreamModelName
	if request.Input != "" {
		req.TextPrompt = request.Input
	}
	if request.Voice != "" {
		req.Speaker = request.Voice
	}
	req.AudioConfig.Format = audioFormat(request.ResponseFormat)
	if request.Speed != nil {
		speed := int(*request.Speed)
		req.AudioConfig.SpeechRate = &speed
	}

	c.Set(contextKeyResponseFormat, request.ResponseFormat)

	body, err := common.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal seed-audio request failed: %w", err)
	}
	return bytes.NewReader(body), nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("doubaoaudio channel only supports audio speech")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("doubaoaudio channel only supports audio speech")
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("doubaoaudio channel only supports audio speech")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to read seed-audio response"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	var seedResp seedAudioResponse
	if unmarshalErr := common.Unmarshal(body, &seedResp); unmarshalErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to parse seed-audio response"),
			types.ErrorCodeBadResponseBody,
			http.StatusInternalServerError,
		)
	}
	if seedResp.Code != 0 {
		return nil, types.NewErrorWithStatusCode(
			errors.New(seedResp.Message),
			types.ErrorCodeBadResponse,
			http.StatusBadRequest,
		)
	}

	contentType := audioContentType(c.GetString(contextKeyResponseFormat))
	c.Header("Content-Type", contentType)

	// Seed-audio returns the audio either inline (base64) or as a url. For the
	// url form we stream the upstream body straight to the client instead of
	// buffering it, so large outputs don't double in memory.
	var audioReader io.Reader
	var closeBody func() error
	switch {
	case seedResp.Audio != "":
		audioData, decodeErr := base64.StdEncoding.DecodeString(seedResp.Audio)
		if decodeErr != nil {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to obtain seed-audio result: %w", decodeErr),
				types.ErrorCodeBadResponse,
				http.StatusBadGateway,
			)
		}
		audioReader = bytes.NewReader(audioData)
	case seedResp.URL != "":
		audioResp, fetchErr := fetchAudio(seedResp.URL)
		if fetchErr != nil {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to obtain seed-audio result: %w", fetchErr),
				types.ErrorCodeBadResponse,
				http.StatusBadGateway,
			)
		}
		audioReader = audioResp.Body
		closeBody = audioResp.Body.Close
	default:
		return nil, types.NewErrorWithStatusCode(
			errors.New("seed-audio response contains no audio data"),
			types.ErrorCodeBadResponse,
			http.StatusBadRequest,
		)
	}
	if closeBody != nil {
		defer closeBody()
	}
	if _, copyErr := io.Copy(c.Writer, audioReader); copyErr != nil {
		logger.LogWarn(c, fmt.Sprintf("failed to stream seed-audio result: %v", copyErr))
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to stream seed-audio result: %w", copyErr),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
		)
	}

	// Charge by output duration (original_duration seconds, 1 token = 1 second).
	// The duration is upstream-controlled, so the conversion goes through the
	// saturating quota helper and any clamp is surfaced for audit.
	promptTokens := info.GetEstimatePromptTokens()
	if seedResp.OriginalDuration > 0 {
		rounded, clamp := common.QuotaRoundChecked(math.Ceil(seedResp.OriginalDuration))
		promptTokens = rounded
		if clamp != nil {
			info.QuotaClamp = clamp
			logger.LogWarn(c, fmt.Sprintf("seed-audio original_duration %v clamped to %d tokens", seedResp.OriginalDuration, promptTokens))
		}
	}

	usage = &dto.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: 0,
		TotalTokens:      promptTokens,
	}
	return usage, nil
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

// fetchAudio returns the upstream audio response after checking its status.
// The caller owns the returned body and must close it.
func fetchAudio(url string) (*http.Response, error) {
	audioResp, err := service.GetHttpClient().Get(url)
	if err != nil {
		return nil, err
	}
	if audioResp.StatusCode != http.StatusOK {
		audioResp.Body.Close()
		return nil, fmt.Errorf("fetch audio url failed: status %d", audioResp.StatusCode)
	}
	return audioResp, nil
}

func audioFormat(responseFormat string) string {
	if responseFormat == "" {
		return "mp3"
	}
	return responseFormat
}

func audioContentType(responseFormat string) string {
	contentTypeMap := map[string]string{
		"mp3":      "audio/mpeg",
		"ogg_opus": "audio/ogg",
		"wav":      "audio/wav",
		"pcm":      "audio/pcm",
	}
	if responseFormat == "" {
		// Default format is mp3, mirroring audioFormat.
		return "audio/mpeg"
	}
	if ct, ok := contentTypeMap[responseFormat]; ok {
		return ct
	}
	return "application/octet-stream"
}

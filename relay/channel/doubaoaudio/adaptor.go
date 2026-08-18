package doubaoaudio

import (
	"bytes"
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
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// seedAudioResponse 承载上游同步响应中计费所需的字段。完整响应体（含
// url/audio/duration）由 DoResponse 原样透传给客户端，这里只解析计费字段。
type seedAudioResponse struct {
	Code             int     `json:"code"`
	Message          string  `json:"message"`
	OriginalDuration float64 `json:"original_duration"`
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
	// 完全透传：客户端请求体直接按上游 seed-audio 协议编写（text_prompt /
	// references / audio_config / watermark 等顶层字段），网关不做字段映射，
	// 仅将 model 锁死为渠道模型以防计费绕过。原始 body 通过 GetBodyStorage
	// 重读——dto.AudioRequest 解析会丢弃上游私有字段，但 storage 里仍完整。
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body storage: %w", err)
	}
	raw, err := storage.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	var req map[string]json.RawMessage
	if err := common.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("unmarshal request body failed: %w", err)
	}
	// 值按字节保留（RawMessage），只替换 model，避免 map 中转改变数值/键序。
	model, err := common.Marshal(info.UpstreamModelName)
	if err != nil {
		return nil, fmt.Errorf("marshal model failed: %w", err)
	}
	req["model"] = model

	body, err := common.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request body failed: %w", err)
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

	// 原样透传上游响应体（含 url/audio），客户端自取音频下载。
	c.Header("Content-Type", "application/json")
	if _, writeErr := c.Writer.Write(body); writeErr != nil {
		logger.LogWarn(c, fmt.Sprintf("failed to write seed-audio response: %v", writeErr))
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to write seed-audio response: %w", writeErr),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
		)
	}

	// 计费按输出时长（original_duration 秒，1 token = 1 秒）。时长由上游控制，
	// 转换走饱和助手 QuotaRoundChecked，超界 clamp 落入审计字段。
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

package novitagemini

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

// badRequestError 让转换阶段的校验失败保留 400 状态码。ImageHelper 会用
// types.NewError 再包一层，但 NewError 对 *NewAPIError 直接透传，状态码不会丢失。
func badRequestError(format string, args ...any) error {
	return types.NewErrorWithStatusCode(fmt.Errorf(format, args...), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// GetRequestURL Novita 的 Gemini 端点：{baseURL}/v1/models/{model}:generateContent。
// base_url 默认 https://api.novita.ai/gemini，版本段固定为 v1（不是 Google 的 v1beta）。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.UpstreamModelName == "" {
		return "", errors.New("model name is empty")
	}
	return fmt.Sprintf("%s/v1/models/%s:generateContent",
		strings.TrimSuffix(info.ChannelBaseUrl, "/"),
		url.PathEscape(info.UpstreamModelName)), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	header.Set("Authorization", "Bearer "+info.ApiKey)
	// 编辑请求允许 multipart 入参，但上游只接收本适配器构造的 JSON 体，
	// 必须覆盖掉客户端原始 Content-Type。
	header.Set("Content-Type", "application/json")
	return nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.RelayMode != relayconstant.RelayModeImagesGenerations &&
		info.RelayMode != relayconstant.RelayModeImagesEdits {
		return nil, badRequestError("novitagemini channel only supports image generations")
	}
	if request.N != nil && *request.N > 1 {
		return nil, badRequestError("novitagemini channel only supports n=1")
	}
	if request.Stream != nil && *request.Stream {
		return nil, badRequestError("novitagemini channel does not support streaming")
	}
	if request.ResponseFormat != "" && request.ResponseFormat != "b64_json" {
		return nil, badRequestError("novitagemini channel only supports b64_json response format")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, badRequestError("prompt is required")
	}

	parts := []dto.GeminiPart{{Text: request.Prompt}}
	responseModalities := []string{"IMAGE"}
	if info.RelayMode == relayconstant.RelayModeImagesEdits {
		imageParts, err := collectEditImageParts(c, request)
		if err != nil {
			return nil, badRequestError("%s", err.Error())
		}
		parts = append(parts, imageParts...)
		responseModalities = []string{"TEXT", "IMAGE"}
	}

	return dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{
			Role:  "user",
			Parts: parts,
		}},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ResponseModalities: responseModalities,
		},
	}, nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, badRequestError("novitagemini channel only supports image generations")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, badRequestError("novitagemini channel only supports image generations")
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, badRequestError("novitagemini channel only supports image generations")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, badRequestError("novitagemini channel only supports image generations")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, badRequestError("novitagemini channel only supports image generations")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, badRequestError("novitagemini channel only supports image generations")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, badRequestError("novitagemini channel only supports image generations")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	// 透传模式会让 ImageHelper 跳过 ConvertImageRequest，把客户端原始
	// OpenAI 请求体直接打到 Gemini 端点；这里显式拦下，避免上游返回难以解读的 400。
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		return nil, badRequestError("novitagemini channel does not support request body passthrough")
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode != relayconstant.RelayModeImagesGenerations &&
		info.RelayMode != relayconstant.RelayModeImagesEdits {
		return nil, types.NewError(errors.New("not implemented"), types.ErrorCodeInvalidRequest)
	}
	return novitaGeminiImageHandler(c, resp, info)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

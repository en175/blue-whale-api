package mediakit

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	channelconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// envelope 是 MediaKit 同步工具的统一响应信封。完整响应体由 DoResponse
// 原样透传给客户端，这里只解析判定成功与否所需的字段。
type envelope struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Error   *upstreamError `json:"error"`
}

type Adaptor struct{}

// Init 必须是 no-op：controller/model.go 在刷新模型列表时会以仅含 ChannelType 的
// ChannelMeta 调用 Init，此处若读 UpstreamModelName 等字段会拿到零值。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

// GetRequestURL 由注册表给出上游路径。客户端传入的工具名只用于查表，
// 不参与路径拼接，避免路径穿越。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	spec, ok := ToolForModel(info.UpstreamModelName)
	if !ok {
		return "", fmt.Errorf("unknown mediakit tool: %s", info.UpstreamModelName)
	}
	if !spec.Sync {
		return "", fmt.Errorf("mediakit tool %s is asynchronous, use POST /v1/mediakit/%s", spec.Name, spec.Name)
	}
	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeMediaKit]
	}
	return strings.TrimSuffix(baseURL, "/") + spec.Path, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Content-Type", "application/json")
	req.Set("Accept", "application/json")
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errSyncOnly
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errSyncOnly
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errSyncOnly
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errSyncOnly
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errSyncOnly
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errSyncOnly
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errSyncOnly
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errSyncOnly
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 原样回写上游响应体，客户端自取产物 URL。上游以 HTTP 200 +
// success=false 表达业务失败，此处必须拦下并返错——controller.Relay 的 defer
// 会据此退款，未成功的调用不扣费（与官方「任务处理失败不触发计费」一致）。
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to read mediakit response"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	var env envelope
	if unmarshalErr := common.Unmarshal(body, &env); unmarshalErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to parse mediakit response"),
			types.ErrorCodeBadResponseBody,
			http.StatusInternalServerError,
		)
	}
	if !env.Success {
		return nil, types.NewErrorWithStatusCode(
			errors.New(failureReason(env.Error, env.Message)),
			types.ErrorCodeBadResponse,
			http.StatusBadRequest,
		)
	}

	c.Header("Content-Type", "application/json")
	if _, writeErr := c.Writer.Write(body); writeErr != nil {
		logger.LogWarn(c, fmt.Sprintf("failed to write mediakit response: %v", writeErr))
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to write mediakit response: %w", writeErr),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
		)
	}

	// 按次/按量工具无 token 概念，给一个非零 TotalTokens 让 PostTextConsumeQuota
	// 正常进入计费分支（与 relay/image_handler.go 的处理一致）。
	return &dto.Usage{PromptTokens: 1, CompletionTokens: 0, TotalTokens: 1}, nil
}

func (a *Adaptor) GetModelList() []string {
	return GetModelList()
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

var errSyncOnly = errors.New("mediakit channel only supports /v1/mediakit/* endpoints")

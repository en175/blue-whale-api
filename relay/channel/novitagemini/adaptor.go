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
	"github.com/QuantumNous/new-api/relay/channel/gemini"
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

// isImageRelayMode 判断本次请求走的是图片翻译链路。该判断在转换、请求、响应三处
// 都要用，集中一处，避免将来新增图片模式时漏改某一边。
func isImageRelayMode(mode int) bool {
	return mode == relayconstant.RelayModeImagesGenerations ||
		mode == relayconstant.RelayModeImagesEdits
}

// geminiActionFromPath 取下游 Gemini 原生路径里的动作后缀（`:generateContent` 等）。
// 图片翻译链路的路径（/v1/images/...）没有这个后缀，返回空串。
// 先截掉 query 再找冒号：RequestURLPath 是包含 query 的完整 URL，query 里也可能带冒号。
func geminiActionFromPath(path string) string {
	if query := strings.IndexByte(path, '?'); query >= 0 {
		path = path[:query]
	}
	colon := strings.LastIndex(path, ":")
	if colon < 0 {
		return ""
	}
	return path[colon+1:]
}

// isUnsupportedGeminiAction 识别本渠道无法代理的 Gemini 原生动作。Path2RelayMode 把整个
// /v1beta/models 前缀（含 :embedContent、:batchEmbedContents、:countTokens）都归到
// RelayModeGemini，而 GetRequestURL 只会拼 :generateContent。不在这里拦，这些请求会被
// 改写成 :generateContent 打给图片模型：既语义错位，又会产生一次真实的图片生成并计费。
// 向量端点还额外绕过了 ConvertGeminiRequest（GeminiEmbeddingHandler 直接序列化请求体），
// 只能在这一层拦。
func isUnsupportedGeminiAction(info *relaycommon.RelayInfo) bool {
	if info.RelayMode != relayconstant.RelayModeGemini {
		return false
	}
	action := geminiActionFromPath(info.RequestURLPath)
	return action != "" && action != "generateContent" && action != "streamGenerateContent"
}

// GetRequestURL Novita 的 Gemini 端点：{baseURL}/v1/models/{model}:{action}。
// base_url 默认 https://api.novita.ai/gemini，版本段固定为 v1（不是 Google 的 v1beta）。
// 图片翻译链路永远非流式（ConvertImageRequest 已拒绝 stream），流式只来自原生链路。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.UpstreamModelName == "" {
		return "", errors.New("model name is empty")
	}
	action := "generateContent"
	if info.IsStream {
		action = "streamGenerateContent?alt=sse"
		// 上游 SSE 已自带事件流节奏，混入网关 ping 帧会破坏下游解析。
		info.DisablePing = true
	}
	return fmt.Sprintf("%s/v1/models/%s:%s",
		strings.TrimSuffix(info.ChannelBaseUrl, "/"),
		url.PathEscape(info.UpstreamModelName),
		action), nil
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
	if !isImageRelayMode(info.RelayMode) {
		return nil, badRequestError("novitagemini channel only supports image generations and edits")
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
	// Multipart edits are parsed by the framework without populating
	// dto.ImageRequest.ResponseFormat, so read the field from the form directly.
	if info.RelayMode == relayconstant.RelayModeImagesEdits && c.Request != nil &&
		strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") &&
		c.Request.MultipartForm != nil {
		if values := c.Request.MultipartForm.Value["response_format"]; len(values) > 0 && values[0] != "" && values[0] != "b64_json" {
			return nil, badRequestError("novitagemini channel only supports b64_json response format")
		}
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, badRequestError("prompt is required")
	}
	if len(request.Mask) > 0 && string(request.Mask) != "null" {
		return nil, badRequestError("novitagemini channel does not support mask")
	}
	// Multipart edits carry mask as a file part instead of a JSON field.
	if c.Request != nil && c.Request.MultipartForm != nil &&
		(len(c.Request.MultipartForm.File["mask"]) > 0 || len(c.Request.MultipartForm.Value["mask"]) > 0) {
		return nil, badRequestError("novitagemini channel does not support mask")
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
	return nil, badRequestError("novitagemini channel only supports image requests")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, badRequestError("novitagemini channel only supports image requests")
}

// ConvertGeminiRequest 是原生链路：下游直接按 Gemini 协议调用，网关只做边界校验
// 后原样转发（语义透传）。未建模字段会在 dto 往返中丢失，这是已知承诺边界。
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if err := validateNativeRequest(request); err != nil {
		return nil, badRequestError("%s", err.Error())
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, badRequestError("novitagemini channel only supports image and Gemini native requests")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, badRequestError("novitagemini channel only supports image and Gemini native requests")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, badRequestError("novitagemini channel only supports image and Gemini native requests")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, badRequestError("novitagemini channel only supports image and Gemini native requests")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	// 向量、计数等非生图动作与图片模型无关，GetRequestURL 会把它们统一改写成
	// :generateContent，上游收到一个语义完全不匹配的请求，还会产生一次真实出图计费。
	if isUnsupportedGeminiAction(info) {
		return nil, badRequestError("novitagemini channel only supports :generateContent and :streamGenerateContent, got %s", geminiActionFromPath(info.RequestURLPath))
	}
	passthrough := model_setting.GetGlobalSettings().PassThroughRequestEnabled ||
		info.ChannelSetting.PassThroughBodyEnabled
	// 图片链路是翻译链路：透传会让 ImageHelper 跳过 ConvertImageRequest，把客户端
	// 原始 OpenAI 请求体直接打到 Gemini 端点，上游只会返回难以解读的 400。
	if isImageRelayMode(info.RelayMode) {
		if passthrough {
			return nil, badRequestError("novitagemini channel does not support request body passthrough")
		}
		return channel.DoApiRequest(a, c, info, requestBody)
	}
	// 原生 Gemini 链路相反——原始体本来就是 Gemini 协议，透传即期望行为。但透传会
	// 跳过 ConvertGeminiRequest，批量与内联图上限必须在这里补上，否则静默失效。
	if info.RelayMode == relayconstant.RelayModeGemini && passthrough {
		request, ok := info.Request.(*dto.GeminiChatRequest)
		if !ok {
			return nil, badRequestError("request is required for passthrough, got %T", info.Request)
		}
		if err := validateNativeRequest(request); err != nil {
			return nil, badRequestError("%s", err.Error())
		}
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == relayconstant.RelayModeGemini {
		// 响应原字节回下游，旁路解析只为计费，复用 Gemini 渠道已验证的实现。
		if info.IsStream {
			usage, apiErr := gemini.GeminiTextGenerationStreamHandler(c, info, resp)
			clampEstimatedImageBilling(c, usage)
			return usage, apiErr
		}
		// 共享处理器是无上限 io.ReadAll；这里换成带上限的 Body，避免异常上游
		// 把整包读进内存。浅拷贝不动原 resp.Header，关闭仍会落到原始 Body。
		respWithBoundedBody := *resp
		respWithBoundedBody.Body = &boundedReadCloser{ReadCloser: resp.Body, remaining: maxUpstreamResponseBytes}
		usage, apiErr := gemini.GeminiTextGenerationHandler(c, info, &respWithBoundedBody)
		clampEstimatedImageBilling(c, usage)
		return usage, apiErr
	}
	if !isImageRelayMode(info.RelayMode) {
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

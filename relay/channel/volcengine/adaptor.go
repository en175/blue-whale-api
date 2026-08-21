package volcengine

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	channelconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

const (
	contextKeyTTSRequest     = "volcengine_tts_request"
	contextKeyResponseFormat = "response_format"
	// maxImageEditImages：Seedream 多参考图上限（官方 4.0 单次复合编辑最多 10 张，
	// 见 IKA5R5 调研）。联调时与 ARK 实测确认数组接受度与上限口径（image vs images）。
	maxImageEditImages = 10
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	if _, ok := channelconstant.ChannelSpecialBases[info.ChannelBaseUrl]; ok {
		adaptor := claude.Adaptor{}
		return adaptor.ConvertClaudeRequest(c, info, req)
	}
	adaptor := openai.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.RelayMode != constant.RelayModeAudioSpeech {
		return nil, errors.New("unsupported audio relay mode")
	}

	appID, token, err := parseVolcengineAuth(info.ApiKey)
	if err != nil {
		return nil, err
	}

	voiceType := mapVoiceType(request.Voice)
	speedRatio := lo.FromPtrOr(request.Speed, 0.0)
	encoding := mapEncoding(request.ResponseFormat)

	c.Set(contextKeyResponseFormat, encoding)

	volcRequest := VolcengineTTSRequest{
		App: VolcengineTTSApp{
			AppID:   appID,
			Token:   token,
			Cluster: "volcano_tts",
		},
		User: VolcengineTTSUser{
			UID: "openai_relay_user",
		},
		Audio: VolcengineTTSAudio{
			VoiceType:  voiceType,
			Encoding:   encoding,
			SpeedRatio: speedRatio,
			Rate:       24000,
		},
		Request: VolcengineTTSReqInfo{
			ReqID:     generateRequestID(),
			Text:      request.Input,
			Operation: "submit",
			Model:     info.OriginModelName,
		},
	}

	if len(request.Metadata) > 0 {
		if err = json.Unmarshal(request.Metadata, &volcRequest); err != nil {
			return nil, fmt.Errorf("error unmarshalling metadata to volcengine request: %w", err)
		}
	}

	c.Set(contextKeyTTSRequest, volcRequest)

	if volcRequest.Request.Operation == "submit" {
		info.IsStream = true
	}

	jsonData, err := json.Marshal(volcRequest)
	if err != nil {
		return nil, fmt.Errorf("error marshalling volcengine request: %w", err)
	}

	return bytes.NewReader(jsonData), nil
}

// volcImageGenerationRequest：豆包 ARK images/generations JSON 请求结构。
// 图生图（/v1/images/edits）也走同一 generations 端点，参考图用顶层 image 字段
// （base64 data URI 或公网 URL）——联调实测（IK9ZQM）ARK 仅消费顶层 image，
// content 数组会被静默忽略。
// Seedream 4.0/5.0 支持多参考图（image 数组，prompt 用"图1/图2"指代），Image 用
// json.RawMessage 承载单图字符串 / 多图数组两种形态，保持已验证的单图 wire 格式不变。
type volcImageGenerationRequest struct {
	Model          string          `json:"model"`
	Prompt         string          `json:"prompt"`
	Size           string          `json:"size,omitempty"`
	N              *uint           `json:"n,omitempty"`
	ResponseFormat string          `json:"response_format,omitempty"`
	Watermark      *bool           `json:"watermark,omitempty"`
	Image          json.RawMessage `json:"image,omitempty"`
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	switch info.RelayMode {
	case constant.RelayModeImagesGenerations:
		return request, nil
	case constant.RelayModeImagesEdits:
		return a.convertImageEditRequest(c, request)
	default:
		return request, nil
	}
}

// convertImageEditRequest 把 OpenAI /v1/images/edits 图生图请求转换为豆包 ARK
// images/generations JSON（图生图走同一 generations 端点，官方不支持表单请求）：
// multipart 参考图 → 顶层 image 的 base64 data URI；多图（image[]）→ image 数组
// （Seedream 4.0/5.0 多参考图，prompt 以"图1/图2"指代）；文本 image URL 字段直接透传。
// 参考火山方舟 Seedream 文档：https://www.volcengine.com/docs/82379/1824121
func (a *Adaptor) convertImageEditRequest(c *gin.Context, request dto.ImageRequest) (any, error) {
	req := volcImageGenerationRequest{
		Model:          request.Model,
		Prompt:         request.Prompt,
		Size:           request.Size,
		N:              request.N,
		ResponseFormat: request.ResponseFormat,
		Watermark:      request.Watermark,
	}

	mf := c.Request.MultipartForm
	if mf == nil {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return nil, fmt.Errorf("failed to parse multipart form: %w", err)
		}
		mf = form
		c.Request.MultipartForm = form
		c.Request.PostForm = url.Values(form.Value)
	}

	// multipart 图片文件（image / image[] / image[ 前缀）→ base64 data URI：
	// 单图保持已验证的字符串 wire 格式，多图输出数组（ARK 多参考图协议）。
	if mf != nil && mf.File != nil {
		if imageFiles := imageEditFiles(mf.File); len(imageFiles) > 0 {
			uris, err := imageFileDataURIs(imageFiles)
			if err != nil {
				return nil, err
			}
			req.Image, err = marshalImageField(uris)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal image field: %w", err)
			}
		}
	}

	// 无 multipart 文件时兜底：客户端以文本 image 字段传公网 URL（单 URL 或 URL 数组）
	if len(req.Image) == 0 && len(request.Image) > 0 {
		imageURLs, ok := parseImageURLs(request.Image)
		if !ok {
			return nil, errors.New("image is required")
		}
		if len(imageURLs) > maxImageEditImages {
			return nil, fmt.Errorf("too many reference images: got %d, max %d", len(imageURLs), maxImageEditImages)
		}
		imageJSON, err := marshalImageField(imageURLs)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal image field: %w", err)
		}
		req.Image = imageJSON
	}

	if len(req.Image) == 0 {
		return nil, errors.New("image is required")
	}
	return &req, nil
}

// imageFileDataURIs 把图生图参考图片文件读为 base64 data URI 列表；超过 Seedream
// 多参考图上限时拒绝（避免超量文件塞进出站 JSON）。
func imageFileDataURIs(files []*multipart.FileHeader) ([]string, error) {
	if len(files) > maxImageEditImages {
		return nil, fmt.Errorf("too many reference images: got %d, max %d", len(files), maxImageEditImages)
	}
	uris := make([]string, 0, len(files))
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open image file: %w", err)
		}
		data, readErr := io.ReadAll(file)
		file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read image file: %w", readErr)
		}
		uris = append(uris, "data:"+detectImageMimeType(fileHeader.Filename)+";base64,"+base64.StdEncoding.EncodeToString(data))
	}
	return uris, nil
}

// marshalImageField 按数量编码 ARK image 字段：单张为字符串、多张为数组，
// 保持联调已验证的单图 wire 格式（image: "data:..."）不变。
func marshalImageField(values []string) (json.RawMessage, error) {
	var (
		data []byte
		err  error
	)
	if len(values) == 1 {
		data, err = common.Marshal(values[0])
	} else {
		data, err = common.Marshal(values)
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// parseImageURLs 解析文本 image 字段为参考图 URL 列表（兼容单 URL 字符串与 URL 数组）。
func parseImageURLs(raw json.RawMessage) ([]string, bool) {
	var single string
	if common.Unmarshal(raw, &single) == nil && single != "" {
		return []string{single}, true
	}
	var urls []string
	if common.Unmarshal(raw, &urls) == nil && len(urls) > 0 {
		return urls, true
	}
	return nil, false
}

// imageEditFiles 从 multipart 表单收集图生图参考图片文件（兼容 image / image[] / image[ 前缀）。
// Seedream 4.0/5.0 支持多参考图，全部文件按序进入 image 数组。
func imageEditFiles(files map[string][]*multipart.FileHeader) []*multipart.FileHeader {
	if imageFiles, ok := files["image"]; ok && len(imageFiles) > 0 {
		return imageFiles
	}
	if imageFiles, ok := files["image[]"]; ok && len(imageFiles) > 0 {
		return imageFiles
	}
	var imageFiles []*multipart.FileHeader
	for fieldName, fieldFiles := range files {
		if strings.HasPrefix(fieldName, "image[") && len(fieldFiles) > 0 {
			imageFiles = append(imageFiles, fieldFiles...)
		}
	}
	return imageFiles
}

func detectImageMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		if strings.HasPrefix(ext, ".jp") {
			return "image/jpeg"
		}
		return "image/png"
	}
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseUrl := info.ChannelBaseUrl
	if baseUrl == "" {
		baseUrl = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeVolcEngine]
	}
	specialPlan, hasSpecialPlan := channelconstant.ChannelSpecialBases[baseUrl]

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		if hasSpecialPlan && specialPlan.ClaudeBaseURL != "" {
			return fmt.Sprintf("%s/v1/messages", specialPlan.ClaudeBaseURL), nil
		}
		if strings.HasPrefix(info.UpstreamModelName, "bot") {
			return fmt.Sprintf("%s/api/v3/bots/chat/completions", baseUrl), nil
		}
		return fmt.Sprintf("%s/api/v3/chat/completions", baseUrl), nil
	default:
		switch info.RelayMode {
		case constant.RelayModeChatCompletions:
			if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
				return fmt.Sprintf("%s/chat/completions", specialPlan.OpenAIBaseURL), nil
			}
			if strings.HasPrefix(info.UpstreamModelName, "bot") {
				return fmt.Sprintf("%s/api/v3/bots/chat/completions", baseUrl), nil
			}
			return fmt.Sprintf("%s/api/v3/chat/completions", baseUrl), nil
		case constant.RelayModeEmbeddings:
			return fmt.Sprintf("%s/api/v3/embeddings", baseUrl), nil
		//豆包的图生图也走generations接口: https://www.volcengine.com/docs/82379/1824121
		case constant.RelayModeImagesGenerations, constant.RelayModeImagesEdits:
			return fmt.Sprintf("%s/api/v3/images/generations", baseUrl), nil
		case constant.RelayModeRerank:
			return fmt.Sprintf("%s/api/v3/rerank", baseUrl), nil
		case constant.RelayModeResponses:
			return fmt.Sprintf("%s/api/v3/responses", baseUrl), nil
		case constant.RelayModeAudioSpeech:
			if baseUrl == channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeVolcEngine] {
				return "wss://openspeech.bytedance.com/api/v1/tts/ws_binary", nil
			}
			return fmt.Sprintf("%s/v1/audio/speech", baseUrl), nil
		default:
		}
	}
	return "", fmt.Errorf("unsupported relay mode: %d", info.RelayMode)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)

	if info.RelayMode == constant.RelayModeAudioSpeech {
		parts := strings.Split(info.ApiKey, "|")
		if len(parts) == 2 {
			req.Set("Authorization", "Bearer;"+parts[1])
		}
		req.Set("Content-Type", "application/json")
		return nil
	} else if info.RelayMode == constant.RelayModeImagesEdits {
		req.Set("Content-Type", gin.MIMEJSON)
	}

	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	if !model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) &&
		strings.HasSuffix(info.UpstreamModelName, "-thinking") &&
		strings.HasPrefix(info.UpstreamModelName, "deepseek") {
		info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
		request.Model = info.UpstreamModelName
		request.THINKING = json.RawMessage(`{"type": "enabled"}`)
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info.RelayMode == constant.RelayModeAudioSpeech {
		baseUrl := info.ChannelBaseUrl
		if baseUrl == "" {
			baseUrl = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeVolcEngine]
		}

		if baseUrl == channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeVolcEngine] {
			if info.IsStream {
				return nil, nil
			}
		}
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayFormat == types.RelayFormatClaude {
		if _, ok := channelconstant.ChannelSpecialBases[info.ChannelBaseUrl]; ok {
			adaptor := claude.Adaptor{}
			return adaptor.DoResponse(c, resp, info)
		}
	}

	if info.RelayMode == constant.RelayModeAudioSpeech {
		encoding := mapEncoding(c.GetString(contextKeyResponseFormat))
		if info.IsStream {
			volcRequestInterface, exists := c.Get(contextKeyTTSRequest)
			if !exists {
				return nil, types.NewErrorWithStatusCode(
					errors.New("volcengine TTS request not found in context"),
					types.ErrorCodeBadRequestBody,
					http.StatusInternalServerError,
				)
			}

			volcRequest, ok := volcRequestInterface.(VolcengineTTSRequest)
			if !ok {
				return nil, types.NewErrorWithStatusCode(
					errors.New("invalid volcengine TTS request type"),
					types.ErrorCodeBadRequestBody,
					http.StatusInternalServerError,
				)
			}

			// Get the WebSocket URL
			requestURL, urlErr := a.GetRequestURL(info)
			if urlErr != nil {
				return nil, types.NewErrorWithStatusCode(
					urlErr,
					types.ErrorCodeBadRequestBody,
					http.StatusInternalServerError,
				)
			}
			return handleTTSWebSocketResponse(c, requestURL, volcRequest, info, encoding)
		}
		return handleTTSResponse(c, resp, info, encoding)
	}

	adaptor := openai.Adaptor{}
	usage, err = adaptor.DoResponse(c, resp, info)
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

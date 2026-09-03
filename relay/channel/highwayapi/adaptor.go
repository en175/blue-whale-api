package highwayapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL HighwayAPI 的模型名不放在请求体，而是拼接在请求路径上：
// {baseURL}/{model}，例如 https://api.highwayapi.ai/v3/gpt-image-2-text-to-image
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.UpstreamModelName == "" {
		return "", errors.New("model name is empty")
	}
	return strings.TrimSuffix(info.ChannelBaseUrl, "/") + "/" + info.UpstreamModelName, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertImageRequest 除 model 外全部透传：model 已拼接进请求路径，其余客户端
// 字段（已知字段 + 未知字段）原样转发给上游，不逐项列白名单。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.RelayMode == relayconstant.RelayModeImagesEdits {
		return nil, errors.New("highwayapi channel does not support image edits")
	}
	// 上游只返回图片 URL，b64_json 请求显式拒绝，避免静默返回不一致的格式。
	if request.ResponseFormat == "b64_json" {
		return nil, errors.New("highwayapi channel only supports url response format")
	}

	// 利用 ImageRequest.MarshalJSON 把全部已定义字段平铺成 map（Extra 除外）。
	data, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	var body map[string]json.RawMessage
	if err := common.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	// 只剔除 model 与 extra_fields 占位（extra_fields 下面单独展开合并）。
	delete(body, "model")
	delete(body, "extra_fields")

	// 兼容 ExtraFields 透传约定（与 jimeng/replicate 一致）：其中的顶层字段合并进请求体。
	if len(request.ExtraFields) > 0 {
		var extraFields map[string]json.RawMessage
		if err := common.Unmarshal(request.ExtraFields, &extraFields); err != nil {
			return nil, fmt.Errorf("invalid extra_fields: %w", err)
		}
		for key, value := range extraFields {
			body[key] = value
		}
	}
	// 客户端传入的顶层未知字段原样透传（优先级最高），兼容上游后续新增参数。
	for key, value := range request.Extra {
		body[key] = value
	}
	return body, nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("highwayapi channel only supports image generations")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("highwayapi channel only supports image generations")
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("highwayapi channel only supports image generations")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("highwayapi channel only supports image generations")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("highwayapi channel only supports image generations")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("highwayapi channel only supports image generations")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("highwayapi channel only supports image generations")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	// 透传模式（PassThroughBodyEnabled）会跳过 ConvertImageRequest，在这里补拦
	// 不支持的中继模式，避免请求先打到上游才报错。
	if info.RelayMode == relayconstant.RelayModeImagesEdits {
		return nil, errors.New("highwayapi channel does not support image edits")
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == relayconstant.RelayModeImagesGenerations {
		usage, err = highwayImageHandler(c, resp, info)
	} else {
		err = types.NewError(errors.New("not implemented"), types.ErrorCodeInvalidRequest)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

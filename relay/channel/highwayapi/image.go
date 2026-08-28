package highwayapi

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// imageResponsePayload HighwayAPI 同步文生图响应。gpt-image 系列返回 images
// 字段（图片 URL 数组），部分模型（如 gemini 系列变体）返回 image_urls。
type imageResponsePayload struct {
	Images    []string `json:"images"`
	ImageURLs []string `json:"image_urls"`
}

// parseImageResponse 把上游图片数组解析为 OpenAI 格式的 data 列表。
// http(s):// 开头的值按 URL 处理，其余按 base64 图片数据兜底。
func parseImageResponse(payload *imageResponsePayload) ([]dto.ImageData, error) {
	images := payload.Images
	if len(images) == 0 {
		images = payload.ImageURLs
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("upstream response contains no images")
	}

	data := make([]dto.ImageData, 0, len(images))
	for _, image := range images {
		if image == "" {
			continue
		}
		item := dto.ImageData{}
		if strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
			item.Url = image
		} else {
			item.B64Json = image
		}
		data = append(data, item)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("upstream response contains no images")
	}
	return data, nil
}

// highwayImageHandler 处理 HighwayAPI 图像生成响应，转换为 OpenAI images 响应
// 格式。上游不返回 token 用量，返回空 usage（计费走图片数量倍率）。
func highwayImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var upstream imageResponsePayload
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	imageData, err := parseImageResponse(&upstream)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	imageResponse := dto.ImageResponse{
		Created: info.StartTime.Unix(),
		Data:    imageData,
	}
	jsonResponse, err := common.Marshal(imageResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err := c.Writer.Write(jsonResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	return &dto.Usage{}, nil
}

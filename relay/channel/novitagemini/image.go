package novitagemini

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// collectEditImageParts 把编辑请求的输入图转成 Gemini inlineData parts。
// 支持 multipart 上传与 JSON 的 images[].image_url（仅 data URL）。
func collectEditImageParts(c *gin.Context, request dto.ImageRequest) ([]dto.GeminiPart, error) {
	if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		return collectMultipartImageParts(c)
	}
	return collectJSONImageParts(request.Images)
}

func collectMultipartImageParts(c *gin.Context) ([]dto.GeminiPart, error) {
	form := c.Request.MultipartForm
	if form == nil {
		parsed, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return nil, fmt.Errorf("failed to parse image edit form: %w", err)
		}
		c.Request.MultipartForm = parsed
		form = parsed
	}

	files := make([]*multipart.FileHeader, 0)
	files = append(files, form.File["image"]...)
	files = append(files, form.File["image[]"]...)
	for key, list := range form.File {
		if key == "image" || key == "image[]" || !strings.HasPrefix(key, "image[") {
			continue
		}
		files = append(files, list...)
	}

	if len(files) == 0 {
		return nil, errors.New("image is required for image edits")
	}
	if len(files) > maxInputImages {
		return nil, fmt.Errorf("too many input images: %d (max %d)", len(files), maxInputImages)
	}

	parts := make([]dto.GeminiPart, 0, len(files))
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open image file: %w", err)
		}
		data, readErr := io.ReadAll(file)
		_ = file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read image file: %w", readErr)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("image file %s is empty", fileHeader.Filename)
		}
		mimeType := http.DetectContentType(data)
		if !strings.HasPrefix(mimeType, "image/") {
			mimeType = fileHeader.Header.Get("Content-Type")
			if !strings.HasPrefix(mimeType, "image/") {
				return nil, fmt.Errorf("unsupported image file: %s", fileHeader.Filename)
			}
		}
		parts = append(parts, dto.GeminiPart{
			InlineData: &dto.GeminiInlineData{
				MimeType: mimeType,
				Data:     base64.StdEncoding.EncodeToString(data),
			},
		})
	}
	return parts, nil
}

func collectJSONImageParts(raw json.RawMessage) ([]dto.GeminiPart, error) {
	if len(raw) == 0 {
		return nil, errors.New("image is required for image edits")
	}
	var images []struct {
		ImageURL string `json:"image_url"`
	}
	if err := common.Unmarshal(raw, &images); err != nil {
		return nil, fmt.Errorf("invalid images: %w", err)
	}
	if len(images) == 0 {
		return nil, errors.New("image is required for image edits")
	}
	if len(images) > maxInputImages {
		return nil, fmt.Errorf("too many input images: %d (max %d)", len(images), maxInputImages)
	}

	parts := make([]dto.GeminiPart, 0, len(images))
	for i, image := range images {
		mimeType, data, ok := parseImageDataURL(image.ImageURL)
		if !ok {
			return nil, fmt.Errorf("images[%d].image_url must be a data URL (data:image/...;base64,...)", i)
		}
		parts = append(parts, dto.GeminiPart{
			InlineData: &dto.GeminiInlineData{
				MimeType: mimeType,
				Data:     data,
			},
		})
	}
	return parts, nil
}

// parseImageDataURL 只接受 data:image/...;base64,...；http(s) 等外链一律拒绝，
// 避免网关替客户端发起任意 URL 请求（SSRF）。
func parseImageDataURL(value string) (mimeType string, data string, ok bool) {
	if !strings.HasPrefix(value, "data:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(value, "data:")
	comma := strings.Index(rest, ",")
	if comma < 0 {
		return "", "", false
	}
	meta := rest[:comma]
	payload := rest[comma+1:]
	if !strings.HasSuffix(meta, ";base64") {
		return "", "", false
	}
	mimeType = strings.TrimSuffix(meta, ";base64")
	if !strings.HasPrefix(mimeType, "image/") || payload == "" {
		return "", "", false
	}
	return mimeType, payload, true
}

// novitaGeminiImageHandler 把 Gemini generateContent 响应转换成 OpenAI 图片响应。
// 只提取 inlineData 图片；响应里的文本 part 不返回给客户端。
func novitaGeminiImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var geminiResponse dto.GeminiChatResponse
	if err := common.Unmarshal(responseBody, &geminiResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	imageData := make([]dto.ImageData, 0, 1)
	for _, candidate := range geminiResponse.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData == nil || part.InlineData.Data == "" {
				continue
			}
			imageData = append(imageData, dto.ImageData{B64Json: part.InlineData.Data})
		}
	}
	if len(imageData) == 0 {
		return nil, types.NewOpenAIError(errors.New("upstream response contains no image"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	imageResponse := dto.ImageResponse{
		Created: time.Now().Unix(),
		Data:    imageData,
	}
	jsonResponse, err := common.Marshal(imageResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, jsonResponse)

	return buildImageUsage(&geminiResponse, len(imageData)), nil
}

// buildImageUsage 以 upstream usageMetadata 计费（包含 thought tokens）。
// 部分上游只返回 prompt 侧 token，此时按「每张 1400 token」兜底，与现有
// Gemini 原生图片路径的估算口径一致。
func buildImageUsage(response *dto.GeminiChatResponse, imageCount int) *dto.Usage {
	var usage dto.Usage
	if dto.HasGeminiUsageMetadataTokens(&response.UsageMetadata) {
		if converted := relayconvert.UsageFromGeminiMetadata(&response.UsageMetadata, 0); converted != nil {
			usage = *converted
		}
	}

	patched := false
	if usage.CompletionTokens == 0 && imageCount > 0 {
		usage.CompletionTokens = imageCount * 1400
		patched = true
	}
	if usage.TotalTokens < usage.PromptTokens+usage.CompletionTokens {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.BillingUsage == nil || patched {
		usage.BillingUsage = dto.NewEstimatedGeminiChatBillingUsage(&usage)
	}
	return &usage
}

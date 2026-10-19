package novitagemini

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sort"
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
	// Sort indexed fields so multi-reference edits keep a deterministic image
	// order across requests (Go map iteration order is random).
	indexedKeys := make([]string, 0)
	for key := range form.File {
		if key == "image" || key == "image[]" || !strings.HasPrefix(key, "image[") {
			continue
		}
		indexedKeys = append(indexedKeys, key)
	}
	sort.Strings(indexedKeys)
	for _, key := range indexedKeys {
		files = append(files, form.File[key]...)
	}

	if len(files) == 0 {
		return nil, errors.New("image is required for image edits")
	}
	if len(files) > maxInputImages {
		return nil, fmt.Errorf("too many input images: %d (max %d)", len(files), maxInputImages)
	}

	parts := make([]dto.GeminiPart, 0, len(files))
	totalBytes := 0
	for _, fileHeader := range files {
		if fileHeader.Size > maxInputImageBytes {
			return nil, fmt.Errorf("image file %s is too large: %d bytes (max %d)", fileHeader.Filename, fileHeader.Size, maxInputImageBytes)
		}
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open image file: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxInputImageBytes+1))
		_ = file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read image file: %w", readErr)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("image file %s is empty", fileHeader.Filename)
		}
		if len(data) > maxInputImageBytes {
			return nil, fmt.Errorf("image file %s is too large: max %d bytes", fileHeader.Filename, maxInputImageBytes)
		}
		totalBytes += len(data)
		if totalBytes > maxTotalInputImageBytes {
			return nil, fmt.Errorf("input images are too large in total: max %d bytes", maxTotalInputImageBytes)
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
	totalBytes := 0
	for i, image := range images {
		mimeType, data, decodedSize, err := parseImageDataURL(image.ImageURL)
		if err != nil {
			return nil, fmt.Errorf("images[%d].image_url %w", i, err)
		}
		if decodedSize > maxInputImageBytes {
			return nil, fmt.Errorf("images[%d] is too large: %d bytes (max %d)", i, decodedSize, maxInputImageBytes)
		}
		totalBytes += decodedSize
		if totalBytes > maxTotalInputImageBytes {
			return nil, fmt.Errorf("input images are too large in total: max %d bytes", maxTotalInputImageBytes)
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
func parseImageDataURL(value string) (mimeType string, data string, decodedSize int, err error) {
	if !strings.HasPrefix(value, "data:") {
		return "", "", 0, errors.New("must be a data URL (data:image/...;base64,...)")
	}
	rest := strings.TrimPrefix(value, "data:")
	comma := strings.Index(rest, ",")
	if comma < 0 {
		return "", "", 0, errors.New("must be a data URL (data:image/...;base64,...)")
	}
	meta := rest[:comma]
	payload := rest[comma+1:]
	if !strings.HasSuffix(meta, ";base64") {
		return "", "", 0, errors.New("must use base64 encoding")
	}
	mimeType = strings.TrimSuffix(meta, ";base64")
	if !strings.HasPrefix(mimeType, "image/") {
		return "", "", 0, errors.New("must be an image MIME type")
	}
	decoded, decodeErr := base64.StdEncoding.DecodeString(payload)
	if decodeErr != nil {
		return "", "", 0, errors.New("base64 payload is invalid")
	}
	if len(decoded) == 0 {
		return "", "", 0, errors.New("base64 payload is empty")
	}
	return mimeType, payload, len(decoded), nil
}

// novitaGeminiImageHandler 把 Gemini generateContent 响应转换成 OpenAI 图片响应。
// 只提取 inlineData 图片；响应里的文本 part 不返回给客户端。
func novitaGeminiImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamResponseBytes+1))
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if len(responseBody) > maxUpstreamResponseBytes {
		return nil, types.NewOpenAIError(fmt.Errorf("upstream response exceeds %d bytes", maxUpstreamResponseBytes), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

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
		return nil, noImageError(&geminiResponse)
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

// noImageError 把上游「没有图」的原因保留到错误消息里。内容安全类拦截属于
// 请求问题，返回 400，避免调用方把 500 当成网关故障反复重试。
func noImageError(response *dto.GeminiChatResponse) *types.NewAPIError {
	for _, candidate := range response.Candidates {
		if candidate.FinishReason == nil || *candidate.FinishReason == "" {
			continue
		}
		reason := *candidate.FinishReason
		message := fmt.Sprintf("upstream returned no image (finishReason=%s)", reason)
		if candidate.FinishMessage != "" {
			message += ": " + candidate.FinishMessage
		}
		if isContentBlockReason(reason) {
			return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if response.PromptFeedback != nil && response.PromptFeedback.BlockReason != nil && *response.PromptFeedback.BlockReason != "" {
		message := fmt.Sprintf("upstream blocked the request (blockReason=%s)", *response.PromptFeedback.BlockReason)
		return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	return types.NewErrorWithStatusCode(errors.New("upstream response contains no image"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
}

func isContentBlockReason(reason string) bool {
	switch reason {
	case "IMAGE_RECITATION", "SAFETY", "PROHIBITED_CONTENT", "BLOCKLIST", "SPII", "RECITATION":
		return true
	default:
		return false
	}
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
		if imageCount > maxResponseImages {
			imageCount = maxResponseImages
		}
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

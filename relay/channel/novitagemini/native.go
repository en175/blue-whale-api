package novitagemini

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
)

// validateNativeRequest 是原生链路的全部请求边界校验：拒绝批量端点，并限制内联
// 图片的张数与体积。正常链路由 ConvertGeminiRequest 调用；透传模式会跳过转换，
// DoRequest 必须再调一次，否则这两条约束在透传下静默失效。
func validateNativeRequest(request *dto.GeminiChatRequest) error {
	if request == nil {
		return errors.New("request is required")
	}
	// 批量端点走的是 batchEmbedContents 语义，图片模型不支持。
	if len(request.Requests) > 0 {
		return errors.New("novitagemini channel does not support batch requests")
	}
	return validateInlineImages(request)
}

// boundedReadCloser 给复用自 gemini 包的响应处理器补上体积上限：那边是无上限的
// io.ReadAll，异常上游可以把整包读进内存。超限时让 Read 返回错误，ReadAll 随即
// 失败，不会把响应写回下游，也不会按半截字节计费。
type boundedReadCloser struct {
	io.ReadCloser
	remaining int64
}

func (b *boundedReadCloser) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return n, fmt.Errorf("upstream response exceeds %d bytes", maxUpstreamResponseBytes)
	}
	return n, err
}

// clampEstimatedImageBilling 给共享 Gemini 处理器的「每张 1400 token」兜底补上
// 张数上限。gemini 包的 geminiResponseInlineImageCount / 流式计数都不设上限，
// 异常上游塞进大量内联 part 时估算 token 会按张数线性放大。只收紧估算分支，
// 上游真实 usageMetadata 原样保留。收紧属于计费异常，必须留痕。
func clampEstimatedImageBilling(c *gin.Context, usage *dto.Usage) {
	maxEstimated := maxResponseImages * estimatedTokensPerImage
	if usage == nil || usage.BillingUsage == nil || !usage.BillingUsage.Estimated {
		return
	}
	if usage.CompletionTokens <= maxEstimated {
		return
	}
	logger.LogWarn(c, fmt.Sprintf(
		"novitagemini estimated image billing clamped: completion_tokens=%d max=%d",
		usage.CompletionTokens, maxEstimated))
	usage.CompletionTokens = maxEstimated
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.BillingUsage = dto.NewEstimatedGeminiChatBillingUsage(usage)
}

// validateInlineImages 给原生链路的内联 base64 加上限。下游可以直接把参考图塞进
// contents[].parts[].inlineData，这是网关内存与上游计费的双重入口；请求体总量虽已
// 被 MAX_REQUEST_BODY_MB 兜住，但 base64 解码会额外放大占用，所以逐张与总量都要
// 单独设限，并且先按编码后长度判断，避免为超限图片先分配一次解码缓冲。
// 超限直接拒绝，不做静默截断。
func validateInlineImages(request *dto.GeminiChatRequest) error {
	// systemInstruction 也是 GeminiChatContent，其中的 parts 同样可以带 inlineData，
	// 且会被原样转发；只扫 contents 会留下一条绕过上限的路径。
	contents := request.Contents
	if request.SystemInstructions != nil {
		contents = append(contents[:len(contents):len(contents)], *request.SystemInstructions)
	}
	count := 0
	totalBytes := 0
	for _, content := range contents {
		for _, part := range content.Parts {
			if part.InlineData == nil || part.InlineData.Data == "" {
				continue
			}
			count++
			if count > maxInputImages {
				return fmt.Errorf("too many inline images: %d (max %d)", count, maxInputImages)
			}
			decodedSize := base64.StdEncoding.DecodedLen(len(part.InlineData.Data))
			if decodedSize > maxInputImageBytes {
				return fmt.Errorf("inline image %d is too large: %d bytes (max %d)", count, decodedSize, maxInputImageBytes)
			}
			totalBytes += decodedSize
			if totalBytes > maxTotalInputImageBytes {
				return fmt.Errorf("inline images are too large in total: max %d bytes", maxTotalInputImageBytes)
			}
			if _, err := base64.StdEncoding.DecodeString(part.InlineData.Data); err != nil {
				return fmt.Errorf("inline image %d is not valid base64", count)
			}
		}
	}
	return nil
}

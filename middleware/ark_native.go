package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel/task/arknative"

	"github.com/gin-gonic/gin"
)

// ArkNativeRequestConvert 把火山方舟原生视频请求改写成网关内部任务请求。
//
// 下游按官方文档发 {model, content[], resolution, ...}，而网关的通用任务链路读
// 的是 {model, prompt, metadata}。这里只做形状归一：提示词取自 content[] 的文本
// 项，整个原生 body 原样放进 metadata，供适配器构造上游请求与判定档位。
//
// 改写后的 body 必须写回 BodyStorage 缓存：后续所有读取都走 KeyBodyStorage，
// 只替换 c.Request.Body 会被旧缓存覆盖。
func ArkNativeRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		storage, err := common.GetBodyStorage(c)
		if err != nil {
			WriteArkError(c, http.StatusBadRequest, "InvalidRequest", err.Error())
			return
		}
		raw, err := storage.Bytes()
		if err != nil {
			WriteArkError(c, http.StatusBadRequest, "InvalidRequest", err.Error())
			return
		}

		native, err := arknative.ParseNativeRequest(raw)
		if err != nil {
			if errors.Is(err, arknative.ErrNativeBodyNotObject) {
				WriteArkError(c, http.StatusBadRequest, "InvalidParameter", err.Error())
				return
			}
			// 顶层是对象但字段类型不对（如 content 传字符串）：把细节写日志，
			// 对外给出可行动的提示，避免一律报「请求体不是 JSON 对象」。
			logger.LogWarn(c.Request.Context(), "ark native request parse failed: "+err.Error())
			WriteArkError(c, http.StatusBadRequest, "InvalidParameter", "request body field type mismatch")
			return
		}
		if strings.TrimSpace(native.Model) == "" {
			WriteArkError(c, http.StatusBadRequest, "InvalidParameter", "model is required")
			return
		}
		// 网关侧不按 token 归属改写请求，只登记解析结果供适配器与 relay 层读取。
		arknative.StoreNativeRequest(c, native)

		// draft_task 项存在但 id 为空时，第二步没有可引用的样片，必须在这里拒绝，
		// 否则会退化成一次普通的空 content 提交。
		if native.HasDraftTask() && native.DraftTaskID() == "" {
			WriteArkError(c, http.StatusBadRequest, "InvalidParameter", "content[].draft_task.id is required")
			return
		}

		internal := map[string]any{
			"model":    native.Model,
			"metadata": native.Raw,
		}
		if prompt := native.Prompt(); prompt != "" {
			internal["prompt"] = prompt
		}
		body, err := common.Marshal(internal)
		if err != nil {
			WriteArkError(c, http.StatusInternalServerError, "InternalError", err.Error())
			return
		}

		rewritten, err := common.CreateBodyStorage(body)
		if err != nil {
			WriteArkError(c, http.StatusInternalServerError, "InternalError", err.Error())
			return
		}
		_ = storage.Close()
		c.Set(common.KeyBodyStorage, rewritten)
		c.Set(common.KeyRequestBody, body)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Next()
	}
}

// IsArkNativePath 报告当前请求是否属于火山原生协议路径。错误信封据此在
// OpenAI 形状与 ResponseMetadata 形状之间分流。
func IsArkNativePath(c *gin.Context) bool {
	return strings.HasPrefix(c.Request.URL.Path, constant.NativeArkVideoTaskPath)
}

// WriteArkError 按火山方舟的错误信封写出 4xx/5xx 并中断链路。
func WriteArkError(c *gin.Context, statusCode int, code string, message string) {
	c.AbortWithStatusJSON(statusCode, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
			"param":   "",
			"type":    arkErrorType(statusCode),
		},
	})
}

// arkErrorType 是火山错误信封里的粗分类，对齐官方文档用词。
func arkErrorType(statusCode int) string {
	switch {
	case statusCode == http.StatusBadRequest:
		return "BadRequest"
	case statusCode == http.StatusUnauthorized:
		return "Unauthorized"
	case statusCode == http.StatusForbidden:
		return "Forbidden"
	case statusCode == http.StatusNotFound:
		return "NotFound"
	case statusCode == http.StatusConflict:
		return "Conflict"
	case statusCode == http.StatusTooManyRequests:
		return "TooManyRequests"
	case statusCode >= 500:
		return "InternalServerError"
	default:
		return "BadRequest"
	}
}

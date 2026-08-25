package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func RunCapabilityOrTokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := c.GetHeader("Authorization")
		value := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		if !strings.HasPrefix(value, "bwcap_") {
			TokenAuth()(c)
			return
		}
		if c.Request.URL.Path != "/v1/chat/completions" {
			abortWithOpenAiMessage(c, http.StatusForbidden, "Run capability scope denied")
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "invalid request body")
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		var request struct {
			Model               string `json:"model"`
			MaxTokens           *int   `json:"max_tokens"`
			MaxCompletionTokens *int   `json:"max_completion_tokens"`
		}
		if common.Unmarshal(body, &request) != nil || request.Model == "" {
			abortWithOpenAiMessage(c, http.StatusForbidden, "Run capability scope denied")
			return
		}
		requestedOutput := 0
		for _, candidate := range []*int{request.MaxTokens, request.MaxCompletionTokens} {
			if candidate != nil && *candidate > requestedOutput {
				requestedOutput = *candidate
			}
		}
		capability, err := service.ConsumeRunCapability(c, value, request.Model, requestedOutput)
		if err != nil {
			// An exhausted capability is answered distinctly so the consumer can tell the breaker
			// tripping apart from a bad credential. 429 rather than 401: nothing is wrong with the
			// credential, it has simply spent what it was given, and the consumer's correct response
			// is to stop rather than to re-authenticate.
			if errors.Is(err, service.ErrRunCapabilityExhausted) {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, "Run capability exhausted", "run_capability_exhausted")
				return
			}
			abortWithOpenAiMessage(c, http.StatusUnauthorized, "Run capability invalid or out of scope")
			return
		}
		user, err := model.GetUserCache(capability.SubjectUserId)
		if err != nil || user.Status != common.UserStatusEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, "Run capability subject unavailable")
			return
		}
		c.Set("id", capability.SubjectUserId)
		user.WriteContext(c)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, user.Group)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "")
		common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
		common.SetContextKey(c, constant.ContextKeyRunCapabilityBilling, true)
		common.SetContextKey(c, constant.ContextKeyRunCapabilityId, capability.CapabilityId)
		common.SetContextKey(c, constant.ContextKeyRunCapabilityMaxInput, capability.MaxInputTokens)
		// Carried so the post-billing path can charge real usage against this same record.
		common.SetContextKey(c, constant.ContextKeyRunCapabilityDigest, capability.Digest)
		c.Next()
	}
}

func RunCapabilityInputBudget() gin.HandlerFunc {
	return func(c *gin.Context) {
		maxInput := common.GetContextKeyInt(c, constant.ContextKeyRunCapabilityMaxInput)
		if maxInput > 0 && common.GetContextKeyInt(c, constant.ContextKeyEstimatedTokens) > maxInput {
			abortWithOpenAiMessage(c, http.StatusForbidden, "Run capability input budget exhausted")
			return
		}
		c.Next()
	}
}

func containsCapabilityModel(models []string, model string) bool {
	for _, allowed := range models {
		if allowed == model {
			return true
		}
	}
	return false
}
func exceedsCapabilityOutput(a, b *int, max int) bool {
	for _, value := range []*int{a, b} {
		if value != nil && (*value < 0 || *value > max) {
			return true
		}
	}
	return false
}

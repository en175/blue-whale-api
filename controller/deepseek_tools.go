package controller

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func DeepSeekToolSearch(c *gin.Context) {
	relayDeepSeekTool(c, service.DeepSeekToolSearch)
}

func DeepSeekToolFetch(c *gin.Context) {
	relayDeepSeekTool(c, service.DeepSeekToolFetch)
}

func relayDeepSeekTool(c *gin.Context, tool string) {
	apiErr := service.DeepSeekToolProxy(c, tool)
	if apiErr == nil {
		return
	}
	logger.LogError(c, fmt.Sprintf("deepseek tool %s relay error: %s", tool, apiErr.Error()))
	apiErr.SetMessage(common.MessageWithRequestId(apiErr.Error(), c.GetString(common.RequestIdKey)))
	c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
}

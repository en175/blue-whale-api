package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// clientSystemFromContext 从 X-Client-System 解析客户端系统；缺失/未知 → "api"（new-api 控制台兜底）。
// 值大小写不敏感，只认 bluewhale/api 两个已知系统。
func clientSystemFromContext(c *gin.Context) string {
	h := strings.ToLower(strings.TrimSpace(c.GetHeader(common.HeaderClientSystem)))
	if h == common.AllowedSystemBlueWhale || h == common.AllowedSystemApi {
		return h
	}
	return common.AllowedSystemApi
}

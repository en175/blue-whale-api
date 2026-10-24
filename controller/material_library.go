package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	materiallib "github.com/QuantumNous/new-api/service/material_library"
	"github.com/QuantumNous/new-api/setting/material_setting"

	"github.com/gin-gonic/gin"
)

// ArkMaterialProxy 是对客的火山管控面形状素材库入口：POST /?Action=X&Version=Y
// 与别名 POST /api/ark?Action=X&Version=Y。响应保留上游 ResponseMetadata 信封，
// 与 /api/material/{Action} 并存（ADR 0024）。
func ArkMaterialProxy(c *gin.Context) {
	cfg := material_setting.GetConfig()
	if !cfg.IsReady() {
		logger.LogWarn(c.Request.Context(), "material library native proxy called but not ready")
		c.JSON(http.StatusServiceUnavailable, map[string]any{
			"ResponseMetadata": map[string]any{
				"Error": map[string]any{
					"Code":    "ServiceUnavailable",
					"Message": "material library is not configured",
				},
				"RequestId": c.GetString(common.RequestIdKey),
			},
		})
		return
	}
	materiallib.HandleNativeRequest(c)
}

// MaterialLibraryProxy 把 /api/material/{Action} 透明转发到上游素材库（火山 ARK 兼容接口）。
// 凭证由网关注入，客户端只携带自己的登录态。未启用或凭证不全时直接拒绝。
// 转发的同时静默记录"用户↔素材组"归属（见 service/material_library 的观察层）。
func MaterialLibraryProxy(c *gin.Context) {
	cfg := material_setting.GetConfig()
	if !cfg.IsReady() {
		logger.LogWarn(c.Request.Context(), "material library proxy called but not ready")
		common.ApiErrorMsg(c, "material library is not configured")
		return
	}
	c.Request = c.Request.WithContext(materiallib.RecordContext(c))
	materiallib.Proxy().ServeHTTP(c.Writer, c.Request)
}

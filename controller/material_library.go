package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	materiallib "github.com/QuantumNous/new-api/service/material_library"
	"github.com/QuantumNous/new-api/setting/material_setting"

	"github.com/gin-gonic/gin"
)

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

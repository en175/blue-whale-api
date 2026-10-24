package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetArkRouter 注册火山管控面形状的素材库入口。
//
// 主路径是根路径 POST /?Action=X&Version=2024-01-01，与官方文档的
// https://ark.cn-beijing.volcengineapi.com/?Action=... 逐字对应；/api/ark 是
// 反代挡掉根路径 POST 时的备用入口，行为完全一致。
//
// 这两个路径不在 /api 分组里，必须显式挂 TokenAuth 与 GlobalAPIRateLimit——
// 分组中间件不会继承过来。
func SetArkRouter(router *gin.Engine) {
	handlers := []gin.HandlerFunc{
		middleware.RouteTag("api"),
		middleware.BodyStorageCleanup(),
		middleware.GlobalAPIRateLimit(),
		middleware.TokenAuth(),
		controller.ArkMaterialProxy,
	}
	router.POST("/", handlers...)
	router.POST("/api/ark", handlers...)
}

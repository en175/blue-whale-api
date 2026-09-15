package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetMediaKitRouter 注册火山 AI MediaKit 的对客路由。
//
// 路由形状（均在 /v1/mediakit 下）：
//
//	POST /v1/mediakit/sync/{tool}   同步工具（图像类），一次往返返回产物
//
// 工具名是路径段，但网关不拿它拼接上游 URL——先经注册表查表，查不到一律 400，
// 路径只来自服务端写死的表项。见 relay/channel/mediakit/registry.go。
func SetMediaKitRouter(router *gin.Engine) {
	mediaKitRouter := router.Group("/v1/mediakit")
	mediaKitRouter.Use(middleware.RouteTag("relay"))
	mediaKitRouter.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		mediaKitRouter.POST("/sync/:tool", controller.RelayMediaKitSync)
	}
}

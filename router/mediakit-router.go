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
//	POST /v1/mediakit/sync/{tool}    同步工具（图像类），一次往返返回产物
//	POST /v1/mediakit/{tool}         异步工具，提交后落 tasks 表，由轮询推进
//	GET  /v1/mediakit/tasks/{task_id} 查询任务状态，只读本库 tasks 表
//
// 工具名是路径段，但网关不拿它拼接上游 URL——先经注册表查表，查不到一律 400，
// 路径只来自服务端写死的表项。见 relay/channel/mediakit/registry.go。
//
// 查询走本库而非代理上游 /api/v1/tasks/：客户端持有的是网关公开 task_xxx，
// 直连上游既暴露上游 task_id，也允许枚举他人任务。
func SetMediaKitRouter(router *gin.Engine) {
	mediaKitRouter := router.Group("/v1/mediakit")
	mediaKitRouter.Use(middleware.RouteTag("relay"))
	mediaKitRouter.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		mediaKitRouter.POST("/sync/:tool", controller.RelayMediaKitSync)
		mediaKitRouter.GET("/tasks/:task_id", controller.RelayTaskFetch)
		mediaKitRouter.POST("/:tool", controller.RelayTask)
	}
}

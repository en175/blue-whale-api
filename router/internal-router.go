package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/gin-gonic/gin"
)

func SetInternalRouter(router *gin.Engine) {
	internal := router.Group("/internal/v1")
	internal.POST("/topup/pay", controller.CollectBlueWhalePay)
}

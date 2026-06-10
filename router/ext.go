package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetExtRouter(apiRouter *gin.RouterGroup) {
	apiRouter.POST("/ratios", middleware.TokenAuthReadOnly(), controller.GetRequestRatios)
}

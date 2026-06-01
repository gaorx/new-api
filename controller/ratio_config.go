package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// GetRatioConfig 返回对前端公开的倍率配置数据。
// 参数：
//   - c：当前请求上下文，用于输出倍率配置或权限错误。
func GetRatioConfig(c *gin.Context) {
	// 只有显式开启公开倍率配置时，外部调用方才能访问这个接口。
	if !ratio_setting.IsExposeRatioEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "倍率配置接口未启用",
		})
		return
	}

	// 读取已经脱敏并允许公开的倍率配置数据，按统一格式返回。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    ratio_setting.GetExposedData(),
	})
}

package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetMissingModels 获取“渠道已引用但模型元数据表中不存在”的模型名称列表。
// 这个接口用于帮助管理员快速发现仍需补充配置的模型项。
// 参数：
//   - c：当前请求上下文，用于返回缺失模型列表。
func GetMissingModels(c *gin.Context) {
	// 查询所有缺失的模型名称。
	missing, err := model.GetMissingModels()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 返回缺失模型列表。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    missing,
	})
}

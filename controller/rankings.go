package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetRankings 返回指定时间周期内的排行榜快照数据。
// 参数：
//   - c：当前请求上下文，用于读取 period 参数并输出排行榜结果。
func GetRankings(c *gin.Context) {
	// 从查询参数中读取统计周期；未传时默认按 week 统计。
	result, err := service.GetRankingsSnapshot(c.DefaultQuery("period", "week"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 成功时直接返回排行榜快照数据。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetAllQuotaDates 返回指定时间范围内、按条件筛选后的全部额度统计日期数据。
// 参数：
//   - c：当前请求上下文，用于读取时间范围和用户名筛选参数。
func GetAllQuotaDates(c *gin.Context) {
	// 解析时间范围和用户名查询参数，供 model 层做聚合查询使用。
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")

	// 查询符合条件的额度统计日期数据；失败时走统一错误响应。
	dates, err := model.GetAllQuotaDates(startTimestamp, endTimestamp, username)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 把日期聚合结果返回给调用方。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

// GetQuotaDatesByUser 返回指定时间范围内按用户聚合的额度统计数据。
// 参数：
//   - c：当前请求上下文，用于读取时间范围并输出聚合结果。
func GetQuotaDatesByUser(c *gin.Context) {
	// 解析时间范围参数，交给 model 层进行按用户维度聚合。
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	// 读取聚合结果；如果查询失败则返回统一错误。
	dates, err := model.GetQuotaDataGroupByUser(startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 成功时返回按用户分组的额度统计数据。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
}

// GetUserQuotaDates 返回当前登录用户在指定时间范围内的额度统计数据。
// 参数：
//   - c：当前请求上下文，用于读取登录用户 ID 和时间范围参数。
func GetUserQuotaDates(c *gin.Context) {
	// 提取用户身份以及时间范围，准备按单用户维度查询额度走势。
	userId := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	// 判断时间跨度是否超过 1 个月
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}

	// 查询当前用户在给定时间范围内的额度数据；错误时走统一错误响应。
	dates, err := model.GetQuotaDataByUserId(userId, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回当前用户的额度统计结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

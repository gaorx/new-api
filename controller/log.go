package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetAllLogs 分页获取全站日志列表，支持多条件筛选。
// 参数：
//   - c：当前请求上下文，用于读取筛选条件和分页参数。
func GetAllLogs(c *gin.Context) {
	// 解析分页参数以及各类筛选条件。
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	// 查询全站日志列表和总数。
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId, upstreamRequestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 把查询结果写入分页对象并返回。
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// GetUserLogs 分页获取当前登录用户自己的日志列表。
// 参数：
//   - c：当前请求上下文，用于读取当前用户、筛选条件和分页参数。
func GetUserLogs(c *gin.Context) {
	// 解析分页参数、当前用户 ID 和筛选条件。
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	// 查询当前用户自己的日志列表和总数。
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, requestId, upstreamRequestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 把查询结果写入分页对象并返回。
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
// 参数：
//   - c：当前请求上下文，用于直接返回废弃提示。
func SearchAllLogs(c *gin.Context) {
	// 该接口已经废弃，始终返回不可用提示。
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
// 参数：
//   - c：当前请求上下文，用于直接返回废弃提示。
func SearchUserLogs(c *gin.Context) {
	// 该接口已经废弃，始终返回不可用提示。
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// GetLogByKey 获取当前 token 对应的日志记录列表。
// 参数：
//   - c：当前请求上下文，用于读取 token_id 并返回该 token 的日志。
func GetLogByKey(c *gin.Context) {
	// 从上下文中读取 token_id；缺失时直接返回错误。
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	// 查询该 token 的日志列表。
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	// 返回 token 关联的日志数据。
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

// GetLogsStat 汇总全站日志的额度与速率统计数据。
// 参数：
//   - c：当前请求上下文，用于读取筛选条件并返回统计结果。
func GetLogsStat(c *gin.Context) {
	// 解析统计范围和筛选条件。
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	// 统计符合条件日志的额度消耗、RPM 和 TPM。
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	// 返回全站日志统计结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": stat.Quota,
			"rpm":   stat.Rpm,
			"tpm":   stat.Tpm,
		},
	})
	return
}

// GetLogsSelfStat 汇总当前登录用户自己的日志统计数据。
// 参数：
//   - c：当前请求上下文，用于读取当前用户名和筛选条件并返回统计结果。
func GetLogsSelfStat(c *gin.Context) {
	// 解析当前用户名以及统计筛选条件。
	username := c.GetString("username")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	// 统计当前用户在筛选范围内的额度消耗、RPM 和 TPM。
	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	// 返回当前用户自己的日志统计结果。
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum.Quota,
			"rpm":   quotaNum.Rpm,
			"tpm":   quotaNum.Tpm,
			//"token": tokenNum,
		},
	})
	return
}

// DeleteHistoryLogs 删除指定时间戳之前的历史日志。
// 参数：
//   - c：当前请求上下文，用于读取目标时间戳并返回删除数量。
func DeleteHistoryLogs(c *gin.Context) {
	// 读取目标时间戳；缺失时无法执行删除。
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	// 分批删除指定时间戳之前的旧日志。
	count, err := model.DeleteOldLog(c.Request.Context(), targetTimestamp, 100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回本次删除数量。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}

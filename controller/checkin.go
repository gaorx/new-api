package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// GetCheckinStatus 获取用户签到状态和历史记录
// 参数：
//   - c：当前请求上下文，用于读取用户身份和月份参数并返回签到统计。
func GetCheckinStatus(c *gin.Context) {
	// 先读取签到配置；如果功能未启用则直接拒绝请求。
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "签到功能未启用")
		return
	}

	// 获取当前用户 ID 和目标月份；未传月份时默认查询当前月份。
	userId := c.GetInt("id")
	// 获取月份参数，默认为当前月份
	month := c.DefaultQuery("month", time.Now().Format("2006-01"))

	// 查询用户该月份的签到统计信息。
	stats, err := model.GetUserCheckinStats(userId, month)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 将签到开关、奖励范围和统计结果一起返回给前端。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":   setting.Enabled,
			"min_quota": setting.MinQuota,
			"max_quota": setting.MaxQuota,
			"stats":     stats,
		},
	})
}

// DoCheckin 执行用户签到
// 参数：
//   - c：当前请求上下文，用于读取当前用户并执行签到动作。
func DoCheckin(c *gin.Context) {
	// 执行签到前先确认签到功能处于开启状态。
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "签到功能未启用")
		return
	}

	// 获取当前登录用户 ID，后续签到、记账和日志都以此为准。
	userId := c.GetInt("id")

	// 调用 model 层执行签到，并返回本次签到获得的奖励信息。
	checkin, err := model.UserCheckin(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 签到成功后写入系统日志，方便后台审计用户的奖励发放记录。
	model.RecordLog(userId, model.LogTypeSystem, fmt.Sprintf("用户签到，获得额度 %s", logger.LogQuota(checkin.QuotaAwarded)))

	// 返回本次签到成功结果以及奖励额度、签到日期等关键信息。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "签到成功",
		"data": gin.H{
			"quota_awarded": checkin.QuotaAwarded,
			"checkin_date":  checkin.CheckinDate},
	})
}

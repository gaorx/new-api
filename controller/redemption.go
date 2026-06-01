package controller

import (
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// GetAllRedemptions 分页获取全部兑换码列表。
// 参数：
//   - c：当前请求上下文，用于读取分页参数并返回兑换码列表。
func GetAllRedemptions(c *gin.Context) {
	// 查询兑换码分页数据与总数。
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.GetAllRedemptions(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 把查询结果写回分页对象并返回。
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

// SearchRedemptions 按关键字搜索兑换码。
// 参数：
//   - c：当前请求上下文，用于读取关键字和分页参数。
func SearchRedemptions(c *gin.Context) {
	// 读取关键字并查询匹配的兑换码列表。
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	redemptions, total, err := model.SearchRedemptions(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 把搜索结果写回分页对象并返回。
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(redemptions)
	common.ApiSuccess(c, pageInfo)
	return
}

// GetRedemption 根据 ID 获取单个兑换码详情。
// 参数：
//   - c：当前请求上下文，用于读取兑换码 ID 并返回详情。
func GetRedemption(c *gin.Context) {
	// 先解析路径中的兑换码 ID。
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	redemption, err := model.GetRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回兑换码详情。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    redemption,
	})
	return
}

// AddRedemption 创建一批新的兑换码。
// 参数：
//   - c：当前请求上下文，用于读取兑换码模板并返回生成出的 key 列表。
func AddRedemption(c *gin.Context) {
	// 创建兑换码前要求系统已完成支付合规确认。
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
		return
	}

	// 解析兑换码创建请求体。
	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 校验名称、数量和过期时间等基础参数。
	if utf8.RuneCountInString(redemption.Name) == 0 || utf8.RuneCountInString(redemption.Name) > 20 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionNameLength)
		return
	}
	if redemption.Count <= 0 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountPositive)
		return
	}
	if redemption.Count > 100 {
		common.ApiErrorI18n(c, i18n.MsgRedemptionCountMax)
		return
	}
	if valid, msg := validateExpiredTime(c, redemption.ExpiredTime); !valid {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
		return
	}
	var keys []string
	// 按数量循环生成兑换码并逐条写入数据库。
	for i := 0; i < redemption.Count; i++ {
		key := common.GetUUID()
		cleanRedemption := model.Redemption{
			UserId:      c.GetInt("id"),
			Name:        redemption.Name,
			Key:         key,
			CreatedTime: common.GetTimestamp(),
			Quota:       redemption.Quota,
			ExpiredTime: redemption.ExpiredTime,
		}
		err = cleanRedemption.Insert()
		if err != nil {
			common.SysError("failed to insert redemption: " + err.Error())
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": i18n.T(c, i18n.MsgRedemptionCreateFailed),
				"data":    keys,
			})
			return
		}
		keys = append(keys, key)
	}
	// 返回成功生成的兑换码 key 列表。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    keys,
	})
	return
}

// DeleteRedemption 删除指定兑换码。
// 参数：
//   - c：当前请求上下文，用于读取兑换码 ID 并执行删除。
func DeleteRedemption(c *gin.Context) {
	// 解析路径中的兑换码 ID 并删除记录。
	id, _ := strconv.Atoi(c.Param("id"))
	err := model.DeleteRedemptionById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回删除成功结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

// UpdateRedemption 更新兑换码信息，或仅更新兑换码状态。
// 参数：
//   - c：当前请求上下文，用于读取更新内容和 status_only 开关。
func UpdateRedemption(c *gin.Context) {
	// 读取是否仅更新状态的查询参数。
	statusOnly := c.Query("status_only")
	// 解析兑换码更新请求体。
	redemption := model.Redemption{}
	err := c.ShouldBindJSON(&redemption)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 先读取数据库中的原始兑换码记录。
	cleanRedemption, err := model.GetRedemptionById(redemption.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if statusOnly == "" {
		// 非 status_only 模式下更新名称、额度和过期时间等主要字段。
		if valid, msg := validateExpiredTime(c, redemption.ExpiredTime); !valid {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": msg})
			return
		}
		// If you add more fields, please also update redemption.Update()
		cleanRedemption.Name = redemption.Name
		cleanRedemption.Quota = redemption.Quota
		cleanRedemption.ExpiredTime = redemption.ExpiredTime
	}
	if statusOnly != "" {
		// 仅更新状态模式下只回写 Status 字段。
		cleanRedemption.Status = redemption.Status
	}
	// 保存更新后的兑换码记录。
	err = cleanRedemption.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回更新后的兑换码对象。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanRedemption,
	})
	return
}

// DeleteInvalidRedemption 删除失效或无效的兑换码记录。
// 参数：
//   - c：当前请求上下文，用于返回删除行数。
func DeleteInvalidRedemption(c *gin.Context) {
	// 执行批量清理无效兑换码。
	rows, err := model.DeleteInvalidRedemptions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回本次清理影响的行数。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

// validateExpiredTime 校验兑换码过期时间是否合法。
// 参数：
//   - c：当前请求上下文，用于国际化错误消息。
//   - expired：待校验的过期时间戳。
//
// 返回：
//   - bool：true 表示过期时间合法。
//   - string：不合法时返回对应错误消息。
func validateExpiredTime(c *gin.Context, expired int64) (bool, string) {
	if expired != 0 && expired < common.GetTimestamp() {
		return false, i18n.T(c, i18n.MsgRedemptionExpireTimeInvalid)
	}
	return true, ""
}

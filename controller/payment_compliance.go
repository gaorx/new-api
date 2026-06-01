package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// PaymentComplianceRequest 表示支付合规确认接口的请求体。
type PaymentComplianceRequest struct {
	Confirmed bool `json:"confirmed"` // 用户是否已勾选并确认合规声明。
}

// requirePaymentCompliance 校验当前系统是否已经完成支付合规确认。
// 参数：
//   - c：当前请求上下文；未确认时用于直接返回错误响应。
//
// 返回：
//   - bool：true 表示已确认合规要求，false 表示已返回错误且不应继续处理。
func requirePaymentCompliance(c *gin.Context) bool {
	// 若系统尚未完成支付合规确认，则直接返回国际化错误提示。
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
		return false
	}
	return true
}

// ConfirmPaymentCompliance 确认支付合规声明，并将确认结果持久化到系统配置中。
// 参数：
//   - c：当前请求上下文，用于读取登录用户、请求体和客户端 IP。
func ConfirmPaymentCompliance(c *gin.Context) {
	// 该操作只能通过控制台会话完成，禁止使用 API access token 直接调用。
	if c.GetBool("use_access_token") {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "This operation requires dashboard session authentication. API access token is not allowed.",
		})
		return
	}

	// 解析确认请求体，并校验用户确实勾选了确认状态。
	var req PaymentComplianceRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if !req.Confirmed {
		common.ApiErrorMsg(c, "请确认合规声明")
		return
	}

	// 提取确认时间、操作者用户 ID 和来源 IP，供后续落库与审计使用。
	now := time.Now().Unix()
	userId := c.GetInt("id")
	clientIP := c.ClientIP()

	// 组织需要写入的合规确认配置项。
	updates := map[string]string{
		"payment_setting.compliance_confirmed":     "true",
		"payment_setting.compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"payment_setting.compliance_confirmed_at":  strconv.FormatInt(now, 10),
		"payment_setting.compliance_confirmed_by":  strconv.Itoa(userId),
		"payment_setting.compliance_confirmed_ip":  clientIP,
	}

	// 逐项写入系统配置，确保合规状态和审计字段全部保存。
	for key, value := range updates {
		if err := model.UpdateOption(key, value); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	// 记录一条操作日志，便于后续追溯是谁在何时何地完成了确认。
	logger.LogInfo(c.Request.Context(), fmt.Sprintf(
		"payment compliance confirmed user_id=%d ip=%s terms_version=%s confirmed_at=%d",
		userId,
		clientIP,
		operation_setting.CurrentComplianceTermsVersion,
		now,
	))

	// 返回确认成功结果以及关键确认元数据。
	common.ApiSuccess(c, gin.H{
		"confirmed":     true,
		"terms_version": operation_setting.CurrentComplianceTermsVersion,
		"confirmed_at":  now,
		"confirmed_by":  userId,
	})
}

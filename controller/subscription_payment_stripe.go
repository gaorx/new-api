package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/thanhpk/randstr"
)

// SubscriptionStripePayRequest 表示 Stripe 订阅支付请求体。
type SubscriptionStripePayRequest struct {
	PlanId int `json:"plan_id"` // 目标订阅套餐 ID。
}

// SubscriptionRequestStripePay 发起 Stripe 订阅支付流程。
// 参数：
//   - c：当前请求上下文，用于读取套餐 ID、当前用户并返回支付链接。
func SubscriptionRequestStripePay(c *gin.Context) {
	// 发起支付前要求系统已完成支付合规确认。
	if !requirePaymentCompliance(c) {
		return
	}

	// 解析请求体并校验套餐 ID。
	var req SubscriptionStripePayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 读取订阅套餐并校验套餐状态与 Stripe 配置完整性。
	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.StripePriceId == "" {
		common.ApiErrorMsg(c, "该套餐未配置 StripePriceId")
		return
	}
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		common.ApiErrorMsg(c, "Stripe 未配置或密钥无效")
		return
	}
	if setting.StripeWebhookSecret == "" {
		common.ApiErrorMsg(c, "Stripe Webhook 未配置")
		return
	}

	// 读取当前用户信息，后续用于限购校验和创建结账链接。
	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}

	// 如套餐配置了单用户购买上限，则先检查购买次数。
	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
	}

	// 生成内部订阅订单 referenceId。
	reference := fmt.Sprintf("sub-stripe-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "sub_ref_" + common.Sha1([]byte(reference))

	// 先调用 Stripe 生成订阅结账链接。
	payLink, err := genStripeSubscriptionLink(referenceId, user.StripeCustomer, user.Email, plan.StripePriceId)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 订阅支付链接创建失败 trade_no=%s plan_id=%d error=%q", referenceId, plan.Id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	// 再落一条待支付订阅订单，供后续 Webhook 完成。
	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           plan.PriceAmount,
		TradeNo:         referenceId,
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// 返回 Stripe 支付链接。
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payLink,
		},
	})
}

// genStripeSubscriptionLink 生成 Stripe 订阅结账链接。
// 参数：
//   - referenceId：内部订单引用号。
//   - customerId：已有 Stripe Customer ID。
//   - email：当前用户邮箱。
//   - priceId：Stripe 套餐 Price ID。
//
// 返回：
//   - string：可直接跳转的 Stripe Checkout URL。
//   - error：创建结账会话失败时返回错误。
func genStripeSubscriptionLink(referenceId string, customerId string, email string, priceId string) (string, error) {
	// 设置当前请求使用的 Stripe Secret Key。
	stripe.Key = setting.StripeApiSecret

	// 组装 Stripe Checkout Session 参数。
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(paymentReturnPath("/console/topup")),
		CancelURL:         stripe.String(paymentReturnPath("/console/topup")),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceId),
				Quantity: stripe.Int64(1),
			},
		},
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
	}

	// 已有 customerId 时直接复用；否则按邮箱创建新 customer。
	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}
		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	// 创建 Stripe Checkout Session 并返回最终 URL。
	result, err := session.New(params)
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

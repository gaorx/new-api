package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// GetSubscription 返回 OpenAI 兼容格式的订阅额度信息。
// 参数：
//   - c：当前请求上下文，用于读取用户或令牌身份并输出兼容响应。
func GetSubscription(c *gin.Context) {
	// 预先声明额度、令牌与过期时间等变量，便于后面按展示模式分别填充。
	var remainQuota int
	var usedQuota int
	var err error
	var token *model.Token
	var expiredTime int64

	// 根据站点配置决定从令牌维度还是用户维度读取剩余额度和已用额度。
	if common.DisplayTokenStatEnabled {
		tokenId := c.GetInt("token_id")
		token, err = model.GetTokenById(tokenId)
		expiredTime = token.ExpiredTime
		remainQuota = token.RemainQuota
		usedQuota = token.UsedQuota
	} else {
		userId := c.GetInt("id")
		remainQuota, err = model.GetUserQuota(userId, false)
		usedQuota, err = model.GetUserUsedQuota(userId)
	}

	// 统一清洗过期时间字段，避免返回负数或未初始化值。
	if expiredTime <= 0 {
		expiredTime = 0
	}

	// 数据读取失败时按 OpenAI 风格错误结构返回，保持兼容接口行为一致。
	if err != nil {
		openAIError := types.OpenAIError{
			Message: err.Error(),
			Type:    "upstream_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}

	// 汇总总额度，并准备转换为兼容接口里的金额展示字段。
	quota := remainQuota + usedQuota
	amount := float64(quota)
	// OpenAI 兼容接口中的 *_USD 字段含义保持“额度单位”对应值：
	// 我们将其解释为以“站点展示类型”为准：
	// - USD: 直接除以 QuotaPerUnit
	// - CNY: 先转 USD 再乘汇率
	// - TOKENS: 直接使用 tokens 数量

	// 按当前站点的额度展示模式把内部 quota 转换成返回给客户端的展示值。
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		amount = amount / common.QuotaPerUnit * operation_setting.USDExchangeRate
	case operation_setting.QuotaDisplayTypeTokens:
		// amount 保持 tokens 数值
	default:
		amount = amount / common.QuotaPerUnit
	}

	// 对无限额度令牌返回一个约定的大值，兼容旧客户端的显示逻辑。
	if token != nil && token.UnlimitedQuota {
		amount = 100000000
	}

	// 组装 OpenAI 兼容的订阅结构体并直接输出。
	subscription := OpenAISubscriptionResponse{
		Object:             "billing_subscription",
		HasPaymentMethod:   true,
		SoftLimitUSD:       amount,
		HardLimitUSD:       amount,
		SystemHardLimitUSD: amount,
		AccessUntil:        expiredTime,
	}
	c.JSON(200, subscription)
	return
}

// GetUsage 返回 OpenAI 兼容格式的已使用额度信息。
// 参数：
//   - c：当前请求上下文，用于读取用户或令牌维度的使用数据。
func GetUsage(c *gin.Context) {
	// 预先声明使用额度与令牌变量，便于按不同展示模式分支赋值。
	var quota int
	var err error
	var token *model.Token

	// 根据配置决定统计令牌已用额度还是用户总已用额度。
	if common.DisplayTokenStatEnabled {
		tokenId := c.GetInt("token_id")
		token, err = model.GetTokenById(tokenId)
		quota = token.UsedQuota
	} else {
		userId := c.GetInt("id")
		quota, err = model.GetUserUsedQuota(userId)
	}

	// 查询失败时返回兼容接口约定的错误结构。
	if err != nil {
		openAIError := types.OpenAIError{
			Message: err.Error(),
			Type:    "new_api_error",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
		return
	}

	// 先把内部 quota 转成浮点数，后续再按展示模式转换为不同单位。
	amount := float64(quota)

	// 按站点展示模式把已用额度转换成对应的返回值。
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		amount = amount / common.QuotaPerUnit * operation_setting.USDExchangeRate
	case operation_setting.QuotaDisplayTypeTokens:
		// tokens 保持原值
	default:
		amount = amount / common.QuotaPerUnit
	}

	// 组装 OpenAI 兼容 usage 结构，注意 TotalUsage 保持旧接口使用的 *100 语义。
	usage := OpenAIUsageResponse{
		Object:     "list",
		TotalUsage: amount * 100,
	}
	c.JSON(200, usage)
	return
}

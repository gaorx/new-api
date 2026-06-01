package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// isPaymentComplianceConfirmed 判断系统是否已经完成支付合规确认。
//
// 返回：
//   - bool：true 表示合规确认已完成。
func isPaymentComplianceConfirmed() bool {
	return operation_setting.IsPaymentComplianceConfirmed()
}

// isStripeTopUpEnabled 判断 Stripe 充值能力当前是否可用。
//
// 返回：
//   - bool：true 表示 Stripe 所需配置完整且合规已确认。
func isStripeTopUpEnabled() bool {
	// 先校验支付合规确认状态，未确认时统一视为不可用。
	if !isPaymentComplianceConfirmed() {
		return false
	}
	// 需要同时具备 API Secret、Webhook Secret 和价格 ID 才可启用。
	return strings.TrimSpace(setting.StripeApiSecret) != "" &&
		strings.TrimSpace(setting.StripeWebhookSecret) != "" &&
		strings.TrimSpace(setting.StripePriceId) != ""
}

// isStripeWebhookConfigured 判断 Stripe Webhook 所需配置是否已填写。
//
// 返回：
//   - bool：true 表示已配置 Stripe Webhook Secret。
func isStripeWebhookConfigured() bool {
	return strings.TrimSpace(setting.StripeWebhookSecret) != ""
}

// isStripeWebhookEnabled 判断 Stripe Webhook 功能是否处于启用状态。
//
// 返回：
//   - bool：true 表示 Stripe 充值整体可用。
func isStripeWebhookEnabled() bool {
	return isStripeTopUpEnabled()
}

// isCreemTopUpEnabled 判断 Creem 充值能力当前是否可用。
//
// 返回：
//   - bool：true 表示 Creem 所需配置完整且合规已确认。
func isCreemTopUpEnabled() bool {
	// 未完成支付合规确认时，直接关闭 Creem 充值入口。
	if !isPaymentComplianceConfirmed() {
		return false
	}
	// 除了 API Key 外，还要求产品配置存在且不是空数组。
	products := strings.TrimSpace(setting.CreemProducts)
	return strings.TrimSpace(setting.CreemApiKey) != "" &&
		products != "" &&
		products != "[]"
}

// isCreemWebhookConfigured 判断 Creem Webhook 配置是否完整。
//
// 返回：
//   - bool：true 表示已配置 Creem Webhook Secret。
func isCreemWebhookConfigured() bool {
	return strings.TrimSpace(setting.CreemWebhookSecret) != ""
}

// isCreemWebhookEnabled 判断 Creem Webhook 是否启用。
//
// 返回：
//   - bool：true 表示 Creem 充值和 Webhook 配置都可用。
func isCreemWebhookEnabled() bool {
	return isCreemTopUpEnabled() && isCreemWebhookConfigured()
}

// isWaffoTopUpEnabled 判断 Waffo 充值能力当前是否可用。
//
// 返回：
//   - bool：true 表示 Waffo 开关已开启且相关配置完整。
func isWaffoTopUpEnabled() bool {
	// 未通过支付合规确认时，不允许开启支付能力。
	if !isPaymentComplianceConfirmed() {
		return false
	}
	// 总开关未打开时，无需继续检查具体凭证。
	if !setting.WaffoEnabled {
		return false
	}

	// Waffo 是否可用最终取决于当前环境下 Webhook/签名凭证是否齐备。
	return isWaffoWebhookConfigured()
}

// isWaffoWebhookConfigured 判断当前模式下 Waffo Webhook 所需配置是否完整。
//
// 返回：
//   - bool：true 表示当前沙箱或正式环境的凭证齐备。
func isWaffoWebhookConfigured() bool {
	// 沙箱模式下校验沙箱凭证集合。
	if setting.WaffoSandbox {
		return strings.TrimSpace(setting.WaffoSandboxApiKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPrivateKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPublicCert) != ""
	}

	// 正式模式下校验正式环境凭证集合。
	return strings.TrimSpace(setting.WaffoApiKey) != "" &&
		strings.TrimSpace(setting.WaffoPrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPublicCert) != ""
}

// isWaffoWebhookEnabled 判断 Waffo Webhook 是否启用。
//
// 返回：
//   - bool：true 表示 Waffo 充值整体处于可用状态。
func isWaffoWebhookEnabled() bool {
	return isWaffoTopUpEnabled()
}

// isWaffoPancakeTopUpEnabled 判断 Waffo Pancake 充值能力是否可用。
//
// 返回：
//   - bool：true 表示所需凭证和商品配置已齐备。
func isWaffoPancakeTopUpEnabled() bool {
	// 同样先经过支付合规确认门槛。
	if !isPaymentComplianceConfirmed() {
		return false
	}
	// Presence-of-credentials = enabled. Webhook public keys ship inside
	// the SDK; mode (test/prod) is read from each event.
	return strings.TrimSpace(setting.WaffoPancakeMerchantID) != "" &&
		strings.TrimSpace(setting.WaffoPancakePrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPancakeProductID) != ""
}

// isWaffoPancakeWebhookConfigured 判断 Waffo Pancake Webhook 是否已具备可用配置。
//
// 返回：
//   - bool：true 表示 Pancake 充值能力本身已可用。
func isWaffoPancakeWebhookConfigured() bool {
	return isWaffoPancakeTopUpEnabled()
}

// isWaffoPancakeWebhookEnabled 判断 Waffo Pancake Webhook 是否启用。
//
// 返回：
//   - bool：true 表示 Pancake 充值整体已启用。
func isWaffoPancakeWebhookEnabled() bool {
	return isWaffoPancakeTopUpEnabled()
}

// isEpayTopUpEnabled 判断易支付充值能力当前是否可用。
//
// 返回：
//   - bool：true 表示易支付配置完整且存在支付方式。
func isEpayTopUpEnabled() bool {
	// 未通过支付合规确认时，不允许启用易支付能力。
	if !isPaymentComplianceConfirmed() {
		return false
	}
	// 除了基础凭证外，还要求至少配置一种支付方式。
	return isEpayWebhookConfigured() && len(operation_setting.PayMethods) > 0
}

// isEpayWebhookConfigured 判断易支付 Webhook 所需配置是否完整。
//
// 返回：
//   - bool：true 表示地址、商户号和密钥都已填写。
func isEpayWebhookConfigured() bool {
	return strings.TrimSpace(operation_setting.PayAddress) != "" &&
		strings.TrimSpace(operation_setting.EpayId) != "" &&
		strings.TrimSpace(operation_setting.EpayKey) != ""
}

// isEpayWebhookEnabled 判断易支付 Webhook 是否启用。
//
// 返回：
//   - bool：true 表示易支付充值能力整体可用。
func isEpayWebhookEnabled() bool {
	return isEpayTopUpEnabled()
}

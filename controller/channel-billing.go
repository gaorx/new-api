package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

// https://github.com/songquanpeng/one-api/issues/79

// OpenAISubscriptionResponse 表示 OpenAI 兼容订阅额度接口的响应结构。
type OpenAISubscriptionResponse struct {
	Object             string  `json:"object"`                // 响应对象类型标识。
	HasPaymentMethod   bool    `json:"has_payment_method"`    // 是否绑定了支付方式。
	SoftLimitUSD       float64 `json:"soft_limit_usd"`        // 软额度上限。
	HardLimitUSD       float64 `json:"hard_limit_usd"`        // 硬额度上限。
	SystemHardLimitUSD float64 `json:"system_hard_limit_usd"` // 系统级硬额度上限。
	AccessUntil        int64   `json:"access_until"`          // 访问有效截止时间。
}

// OpenAIUsageDailyCost 表示 OpenAI 兼容日消耗明细中的单日数据。
type OpenAIUsageDailyCost struct {
	Timestamp float64 `json:"timestamp"` // 当日时间戳。
	LineItems []struct {
		Name string  `json:"name"` // 费用项目名称。
		Cost float64 `json:"cost"` // 该项目费用。
	} // 每日费用明细列表。
}

// OpenAICreditGrants 表示 OpenAI 兼容额度授予接口响应。
type OpenAICreditGrants struct {
	Object         string  `json:"object"`          // 响应对象类型。
	TotalGranted   float64 `json:"total_granted"`   // 总授予额度。
	TotalUsed      float64 `json:"total_used"`      // 总已用额度。
	TotalAvailable float64 `json:"total_available"` // 当前可用额度。
}

// OpenAIUsageResponse 表示 OpenAI 兼容 usage 接口响应。
type OpenAIUsageResponse struct {
	Object string `json:"object"` // 响应对象类型。
	//DailyCosts []OpenAIUsageDailyCost `json:"daily_costs"`
	TotalUsage float64 `json:"total_usage"` // 总使用量，单位为 0.01 美元。
}

// OpenAISBUsageResponse 表示 openai-sb 平台的余额查询响应。
type OpenAISBUsageResponse struct {
	Msg  string `json:"msg"` // 平台返回的消息文本。
	Data *struct {
		Credit string `json:"credit"` // 剩余额度字符串。
	} `json:"data"` // 响应主体数据。
}

// AIProxyUserOverviewResponse 表示 AIProxy 用户概览接口响应。
type AIProxyUserOverviewResponse struct {
	Success   bool   `json:"success"`    // 请求是否成功。
	Message   string `json:"message"`    // 平台返回消息。
	ErrorCode int    `json:"error_code"` // 平台错误码。
	Data      struct {
		TotalPoints float64 `json:"totalPoints"` // 用户总积分余额。
	} `json:"data"` // 用户概览主体数据。
}

// API2GPTUsageResponse 表示 API2GPT 平台的额度响应结构。
type API2GPTUsageResponse struct {
	Object         string  `json:"object"`          // 响应对象类型。
	TotalGranted   float64 `json:"total_granted"`   // 总授予额度。
	TotalUsed      float64 `json:"total_used"`      // 总已用额度。
	TotalRemaining float64 `json:"total_remaining"` // 总剩余额度。
}

// APGC2DGPTUsageResponse 表示 AIGC2D 平台的额度响应结构。
type APGC2DGPTUsageResponse struct {
	//Grants         interface{} `json:"grants"`
	Object         string  `json:"object"`          // 响应对象类型。
	TotalAvailable float64 `json:"total_available"` // 当前可用额度。
	TotalGranted   float64 `json:"total_granted"`   // 总授予额度。
	TotalUsed      float64 `json:"total_used"`      // 总已用额度。
}

// SiliconFlowUsageResponse 表示 SiliconFlow 用户信息与余额响应。
type SiliconFlowUsageResponse struct {
	Code    int    `json:"code"`    // 平台业务状态码。
	Message string `json:"message"` // 平台返回消息。
	Status  bool   `json:"status"`  // 请求是否成功。
	Data    struct {
		ID            string `json:"id"`            // 用户 ID。
		Name          string `json:"name"`          // 用户名称。
		Image         string `json:"image"`         // 头像地址。
		Email         string `json:"email"`         // 邮箱地址。
		IsAdmin       bool   `json:"isAdmin"`       // 是否管理员。
		Balance       string `json:"balance"`       // 基础余额。
		Status        string `json:"status"`        // 用户状态。
		Introduction  string `json:"introduction"`  // 个人简介。
		Role          string `json:"role"`          // 用户角色。
		ChargeBalance string `json:"chargeBalance"` // 充值余额。
		TotalBalance  string `json:"totalBalance"`  // 总余额。
		Category      string `json:"category"`      // 账户分类。
	} `json:"data"` // 用户详情主体数据。
}

// DeepSeekUsageResponse 表示 DeepSeek 余额查询响应。
type DeepSeekUsageResponse struct {
	IsAvailable  bool `json:"is_available"` // 账户是否可用。
	BalanceInfos []struct {
		Currency        string `json:"currency"`          // 币种代码。
		TotalBalance    string `json:"total_balance"`     // 总余额。
		GrantedBalance  string `json:"granted_balance"`   // 赠送余额。
		ToppedUpBalance string `json:"topped_up_balance"` // 充值余额。
	} `json:"balance_infos"` // 各币种余额信息列表。
}

// OpenRouterCreditResponse 表示 OpenRouter credits 接口响应。
type OpenRouterCreditResponse struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"` // 总可授信额度。
		TotalUsage   float64 `json:"total_usage"`   // 总已用额度。
	} `json:"data"` // OpenRouter 主体数据。
}

// GetAuthHeader get auth header
// 参数：
//   - token：需要放入 Authorization 头中的 API Key。
//
// 返回：
//   - http.Header：带 Bearer 认证头的请求头集合。
func GetAuthHeader(token string) http.Header {
	// 构造标准 Bearer 认证请求头，供大多数 OpenAI 兼容上游复用。
	h := http.Header{}
	h.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	return h
}

// GetClaudeAuthHeader get claude auth header
// 参数：
//   - token：Claude 上游使用的 API Key。
//
// 返回：
//   - http.Header：符合 Anthropic 协议要求的请求头集合。
func GetClaudeAuthHeader(token string) http.Header {
	// Claude 兼容接口使用 x-api-key 和 anthropic-version 两个关键请求头。
	h := http.Header{}
	h.Add("x-api-key", token)
	h.Add("anthropic-version", "2023-06-01")
	return h
}

// GetResponseBody 发起一个不带请求体的上游请求，并返回成功响应的原始字节内容。
// 参数：
//   - method：HTTP 方法。
//   - url：目标上游地址。
//   - channel：当前渠道对象，用于读取代理配置。
//   - headers：需要附加到请求上的请求头集合。
//
// 返回：
//   - []byte：响应体原始字节。
//   - error：请求、代理创建、状态码检查或读取响应失败时返回错误。
func GetResponseBody(method, url string, channel *model.Channel, headers http.Header) ([]byte, error) {
	// 先构造基础请求对象，后续再把上游所需头信息逐项附加进去。
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	// 逐个写入调用方提供的请求头，保持与各上游接口要求一致。
	for k := range headers {
		req.Header.Add(k, headers.Get(k))
	}

	// 根据渠道代理配置创建 HTTP 客户端，兼容 per-channel proxy 设置。
	client, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}

	// 发起上游请求；网络异常时直接返回错误给调用方。
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// 非 200 状态码统一视为失败，交由上层决定如何处理。
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", res.StatusCode)
	}

	// 读取并关闭响应体，返回原始字节内容供上层自行解析。
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	err = res.Body.Close()
	if err != nil {
		return nil, err
	}
	return body, nil
}

// updateChannelCloseAIBalance 从 OpenAI 兼容 credit grants 接口读取渠道余额并写回数据库。
// 参数：
//   - channel：需要更新余额的渠道对象。
//
// 返回：
//   - float64：解析得到的可用余额。
//   - error：请求或解析失败时返回错误。
func updateChannelCloseAIBalance(channel *model.Channel) (float64, error) {
	// 调用 OpenAI 兼容 credit grants 接口获取额度总览。
	url := fmt.Sprintf("%s/dashboard/billing/credit_grants", channel.GetBaseURL())
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))

	if err != nil {
		return 0, err
	}

	// 解析返回值并把可用余额写回渠道记录。
	response := OpenAICreditGrants{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(response.TotalAvailable)
	return response.TotalAvailable, nil
}

// updateChannelOpenAISBBalance 从 openai-sb 平台查询渠道余额。
func updateChannelOpenAISBBalance(channel *model.Channel) (float64, error) {
	// 通过平台专用用户状态接口拉取 credit 文本余额。
	url := fmt.Sprintf("https://api.openai-sb.com/sb-api/user/status?api_key=%s", channel.Key)
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := OpenAISBUsageResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if response.Data == nil {
		return 0, errors.New(response.Msg)
	}

	// 把平台返回的字符串余额解析成浮点数并写回渠道。
	balance, err := strconv.ParseFloat(response.Data.Credit, 64)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

// updateChannelAIProxyBalance 从 AIProxy 用户概览接口查询渠道积分余额。
func updateChannelAIProxyBalance(channel *model.Channel) (float64, error) {
	// AIProxy 使用 Api-Key 头认证，因此这里手动构造请求头。
	url := "https://aiproxy.io/api/report/getUserOverview"
	headers := http.Header{}
	headers.Add("Api-Key", channel.Key)
	body, err := GetResponseBody("GET", url, channel, headers)
	if err != nil {
		return 0, err
	}
	response := AIProxyUserOverviewResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if !response.Success {
		return 0, fmt.Errorf("code: %d, message: %s", response.ErrorCode, response.Message)
	}

	// 将总积分直接作为渠道余额保存。
	channel.UpdateBalance(response.Data.TotalPoints)
	return response.Data.TotalPoints, nil
}

// updateChannelAPI2GPTBalance 从 API2GPT 平台查询剩余额度。
func updateChannelAPI2GPTBalance(channel *model.Channel) (float64, error) {
	// 调用平台提供的 credit grants 接口读取 remaining 字段。
	url := "https://api.api2gpt.com/dashboard/billing/credit_grants"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))

	if err != nil {
		return 0, err
	}
	response := API2GPTUsageResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(response.TotalRemaining)
	return response.TotalRemaining, nil
}

// updateChannelSiliconFlowBalance 从 SiliconFlow 用户信息接口查询余额。
func updateChannelSiliconFlowBalance(channel *model.Channel) (float64, error) {
	// 访问 SiliconFlow 用户信息接口，读取 total balance 字段。
	url := "https://api.siliconflow.cn/v1/user/info"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := SiliconFlowUsageResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if response.Code != 20000 {
		return 0, fmt.Errorf("code: %d, message: %s", response.Code, response.Message)
	}

	// 将字符串格式的总余额解析成浮点数并保存。
	balance, err := strconv.ParseFloat(response.Data.TotalBalance, 64)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelDeepSeekBalance(channel *model.Channel) (float64, error) {
	url := "https://api.deepseek.com/user/balance"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := DeepSeekUsageResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	index := -1
	for i, balanceInfo := range response.BalanceInfos {
		if balanceInfo.Currency == "CNY" {
			index = i
			break
		}
	}
	if index == -1 {
		return 0, errors.New("currency CNY not found")
	}
	balance, err := strconv.ParseFloat(response.BalanceInfos[index].TotalBalance, 64)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelAIGC2DBalance(channel *model.Channel) (float64, error) {
	url := "https://api.aigc2d.com/dashboard/billing/credit_grants"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := APGC2DGPTUsageResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(response.TotalAvailable)
	return response.TotalAvailable, nil
}

func updateChannelOpenRouterBalance(channel *model.Channel) (float64, error) {
	url := "https://openrouter.ai/api/v1/credits"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := OpenRouterCreditResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	balance := response.Data.TotalCredits - response.Data.TotalUsage
	channel.UpdateBalance(balance)
	return balance, nil
}

func updateChannelMoonshotBalance(channel *model.Channel) (float64, error) {
	url := "https://api.moonshot.cn/v1/users/me/balance"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}

	type MoonshotBalanceData struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	}

	type MoonshotBalanceResponse struct {
		Code   int                 `json:"code"`
		Data   MoonshotBalanceData `json:"data"`
		Scode  string              `json:"scode"`
		Status bool                `json:"status"`
	}

	response := MoonshotBalanceResponse{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if !response.Status || response.Code != 0 {
		return 0, fmt.Errorf("failed to update moonshot balance, status: %v, code: %d, scode: %s", response.Status, response.Code, response.Scode)
	}
	availableBalanceCny := response.Data.AvailableBalance
	availableBalanceUsd := decimal.NewFromFloat(availableBalanceCny).Div(decimal.NewFromFloat(operation_setting.Price)).InexactFloat64()
	channel.UpdateBalance(availableBalanceUsd)
	return availableBalanceUsd, nil
}

func updateChannelBalance(channel *model.Channel) (float64, error) {
	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() == "" {
		channel.BaseURL = &baseURL
	}
	switch channel.Type {
	case constant.ChannelTypeOpenAI:
		if channel.GetBaseURL() != "" {
			baseURL = channel.GetBaseURL()
		}
	case constant.ChannelTypeAzure:
		return 0, errors.New("尚未实现")
	case constant.ChannelTypeCustom:
		baseURL = channel.GetBaseURL()
	//case common.ChannelTypeOpenAISB:
	//	return updateChannelOpenAISBBalance(channel)
	case constant.ChannelTypeAIProxy:
		return updateChannelAIProxyBalance(channel)
	case constant.ChannelTypeAPI2GPT:
		return updateChannelAPI2GPTBalance(channel)
	case constant.ChannelTypeAIGC2D:
		return updateChannelAIGC2DBalance(channel)
	case constant.ChannelTypeSiliconFlow:
		return updateChannelSiliconFlowBalance(channel)
	case constant.ChannelTypeDeepSeek:
		return updateChannelDeepSeekBalance(channel)
	case constant.ChannelTypeOpenRouter:
		return updateChannelOpenRouterBalance(channel)
	case constant.ChannelTypeMoonshot:
		return updateChannelMoonshotBalance(channel)
	default:
		return 0, errors.New("尚未实现")
	}
	url := fmt.Sprintf("%s/v1/dashboard/billing/subscription", baseURL)

	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	subscription := OpenAISubscriptionResponse{}
	err = json.Unmarshal(body, &subscription)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	startDate := fmt.Sprintf("%s-01", now.Format("2006-01"))
	endDate := now.Format("2006-01-02")
	if !subscription.HasPaymentMethod {
		startDate = now.AddDate(0, 0, -100).Format("2006-01-02")
	}
	url = fmt.Sprintf("%s/v1/dashboard/billing/usage?start_date=%s&end_date=%s", baseURL, startDate, endDate)
	body, err = GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	usage := OpenAIUsageResponse{}
	err = json.Unmarshal(body, &usage)
	if err != nil {
		return 0, err
	}
	balance := subscription.HardLimitUSD - usage.TotalUsage/100
	channel.UpdateBalance(balance)
	return balance, nil
}

func UpdateChannelBalance(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "多密钥渠道不支持余额查询",
		})
		return
	}
	balance, err := updateChannelBalance(channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"balance": balance,
	})
}

func updateAllChannelsBalance() error {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue
		}
		if channel.ChannelInfo.IsMultiKey {
			continue // skip multi-key channels
		}
		// TODO: support Azure
		//if channel.Type != common.ChannelTypeOpenAI && channel.Type != common.ChannelTypeCustom {
		//	continue
		//}
		balance, err := updateChannelBalance(channel)
		if err != nil {
			continue
		} else {
			// err is nil & balance <= 0 means quota is used up
			if balance <= 0 {
				service.DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan()), "余额不足")
			}
		}
		time.Sleep(common.RequestInterval)
	}
	return nil
}

func UpdateAllChannelsBalance(c *gin.Context) {
	// TODO: make it async
	err := updateAllChannelsBalance()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func AutomaticallyUpdateChannels(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Minute)
		common.SysLog("updating all channels")
		_ = updateAllChannelsBalance()
		common.SysLog("channels update done")
	}
}

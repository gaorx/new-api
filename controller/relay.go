package controller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// relayHandler 根据通用的 RelayMode 将同步请求分派到具体的业务处理器。
// 参数：
//   - c：当前请求的 Gin 上下文，里面已经写入了用户、令牌、渠道、日志等运行时信息。
//   - info：本次请求对应的 RelayInfo，包含模型、协议、计费、重试等完整上下文。
//
// 返回：
//   - *types.NewAPIError：统一错误对象；返回 nil 表示本次转发已经成功完成。
func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	// 按业务模式选择对应的 relay helper，避免在主流程里堆叠大量协议分支。
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

// geminiRelayHandler 处理 Gemini 协议下的细分路由分发。
// 参数：
//   - c：当前请求上下文，用于读取 URL 路径并承载后续响应输出。
//   - info：已经生成完成的 RelayInfo，上层已经补齐模型、渠道、计费等信息。
//
// 返回：
//   - *types.NewAPIError：统一错误对象；返回 nil 表示 Gemini 请求处理成功。
func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	// Gemini 的 embedding 与通用文本能力共用入口，需要先按路径做一次轻量分流。
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

// Relay 是同步 Relay 请求的总入口，负责完成校验、计费、选路、转发、重试和错误回写。
// 参数：
//   - c：当前 HTTP/WebSocket 请求上下文，贯穿整条请求生命周期。
//   - relayFormat：本次请求采用的入口协议格式，例如 OpenAI、Claude、Gemini、Realtime 等。
func Relay(c *gin.Context, relayFormat types.RelayFormat) {
	// 先提取请求级元数据，后续无论成功还是失败都要用于日志与错误信息增强。
	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	// 统一准备主流程中会复用的错误对象和 WebSocket 连接句柄。
	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	// Realtime 协议需要先把 HTTP 请求升级为 WebSocket，后续才可以走实时消息转发。
	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	// 统一的错误收口逻辑放在 defer 中，确保任意阶段出错都能回写成对应协议的错误格式。
	defer func() {
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("relay error: %s", common.LocalLogPreview(newAPIError.Error())))
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	// 先读取并校验请求体，确保后续计费、路由和协议转换都建立在合法输入之上。
	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	// 基于当前请求与上下文生成 RelayInfo，后续所有处理都围绕这个统一上下文对象展开。
	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	// 根据配置决定是否构建完整的 TokenCountMeta，避免在无必要时做大字符串拼接。
	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	// Avoid building huge CombineText (strings.Join) when token counting and sensitive check are both disabled.
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	// 如果启用了敏感词检测，就在真正发请求前先拦截明显违规的 prompt。
	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	// 估算请求 token，用于预扣费、路由决策以及日志统计等后续环节。
	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	// 将估算出的 prompt token 回填到 RelayInfo，后续计费和观测逻辑都会复用它。
	relayInfo.SetEstimatePromptTokens(tokens)

	// 结合模型、分组和请求元信息计算本次调用的价格快照。
	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	// 对免费模型直接跳过预扣费，其余模型在真正发请求前先完成统一预扣。
	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	// 如果后续任一步失败，这里负责兜底退款，并在需要时补收违规费用。
	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	// 初始化重试状态，后面的每一轮尝试都通过 RetryParam 驱动选路和重试次数控制。
	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: relayInfo.TokenGroup,
		ModelName:  relayInfo.OriginModelName,
		Retry:      common.GetPointer(0),
	}
	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil

	// 主重试循环：每轮重新选可用渠道、恢复请求体、执行转发，并根据错误类型决定是否继续。
	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		// 把当前重试序号写入 RelayInfo，便于下游日志、指标和调试输出复用。
		relayInfo.RetryIndex = retryParam.GetRetry()
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			newAPIError = channelErr
			break
		}

		// 记录本轮实际命中的渠道，便于最终输出重试路径和渠道诊断信息。
		addUsedChannel(c, channel.Id)

		// 每次重试前都要重新恢复请求体，否则前一轮读取后 Body 已经不可再次消费。
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		// 按入口协议执行真正的下游转发逻辑，不同协议走各自的 helper。
		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}

		// 当前渠道执行成功时立即结束主流程，不再继续重试。
		if newAPIError == nil {
			relayInfo.LastError = nil
			return
		}

		// 失败后先统一归一化错误，再保存最近一次错误供后续决策与日志使用。
		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		relayInfo.LastError = newAPIError

		// 把本轮失败渠道记录到错误处理链路，必要时会触发熔断、错误日志和后台禁用。
		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		// 根据错误类型、状态码、是否锁定渠道等条件决定要不要继续尝试下一条渠道。
		if !shouldRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	// 如果中途切换过多个渠道，则把完整重试路径打印出来，方便定位分发行为。
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// 对最终失败的请求补充性能采样，让监控侧能够看到失败请求的分布。
	if newAPIError != nil {
		gopool.Go(func() {
			perfmetrics.RecordRelaySample(relayInfo, false, 0)
		})
	}
}

// upgrader 是 realtime 协议专用的 WebSocket 升级器。
// 这里显式声明允许的子协议，并放行跨域检查，以兼容 OpenAI Realtime 风格客户端。
var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

// addUsedChannel 把当前轮实际使用的渠道 ID 追加到上下文中，供重试日志和错误分析使用。
// 参数：
//   - c：当前请求上下文，用于读取和写回 use_channel 列表。
//   - channelId：本轮命中的渠道 ID。
func addUsedChannel(c *gin.Context, channelId int) {
	// 复用 context 中的切片累计记录整次请求经过的所有渠道。
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

// fastTokenCountMetaForPricing 在关闭完整 token 统计时，构建一个“只满足定价需要”的轻量元信息。
// 参数：
//   - request：已经通过基础校验的统一请求对象，可能是文本、图片或其他协议实现。
//
// 返回：
//   - *types.TokenCountMeta：用于后续价格计算的最小 token 元信息。
func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	// 空请求兜底返回空结构，避免上层继续判断 nil。
	if request == nil {
		return &types.TokenCountMeta{}
	}

	// 先构造一个基础元信息对象，默认按 tokenizer 计数类型处理。
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}

	// 针对不同请求类型，尽量提取出定价必须的 max token / image 信息，避免做额外大对象构造。
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		maxCompletionTokens := lo.FromPtrOr(r.MaxCompletionTokens, uint(0))
		maxTokens := lo.FromPtrOr(r.MaxTokens, uint(0))
		if maxCompletionTokens > maxTokens {
			meta.MaxTokens = int(maxCompletionTokens)
		} else {
			meta.MaxTokens = int(maxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(lo.FromPtrOr(r.MaxOutputTokens, uint(0)))
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(lo.FromPtr(r.MaxTokens))
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

// getChannel 为当前这一次尝试解析出真正要使用的渠道，并同步刷新上下文中的渠道信息。
// 参数：
//   - c：当前请求上下文，用于读取锁定渠道信息，并写回命中渠道的上下文变量。
//   - info：本次请求的 RelayInfo，里面保存了模型名、渠道元信息和价格快照等数据。
//   - retryParam：本轮重试参数，包含模型、分组和当前重试计数。
//
// 返回：
//   - *model.Channel：当前轮最终命中的渠道对象。
//   - *types.NewAPIError：获取或初始化渠道失败时返回的统一错误。
func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	// 如果本次请求已经锁定了渠道元信息，则直接从 context 恢复一个轻量渠道对象，避免重新选路。
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}

	// 正常场景下按当前分组、模型和重试状态重新选择一个满足条件的可用渠道。
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)

	// 每轮选路后都重新计算一次分组倍率信息，保证日志和计费上下文与当前实际分组一致。
	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)

	// 如果选路过程出错或没有拿到渠道，统一转换成获取渠道失败的业务错误返回给上层。
	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, info.OriginModelName, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	// 将本轮命中渠道的配置重新写回 context，保证后续 helper 读取到的是最新渠道状态。
	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if newAPIError != nil {
		return nil, newAPIError
	}
	return channel, nil
}

// shouldRetry 根据错误类型和运行时条件判断同步 Relay 是否应该继续重试。
// 参数：
//   - c：当前请求上下文，用于判断是否命中特定渠道、Affinity 策略等限制。
//   - openaiErr：本轮执行返回的统一错误对象。
//   - retryTimes：当前还剩余的可用重试次数。
//
// 返回：
//   - bool：true 表示允许继续重试，false 表示应立即停止。
func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	// 空错误说明当前已经成功，不需要再进入重试逻辑。
	if openaiErr == nil {
		return false
	}

	// 命中 Affinity 禁止重试策略时，直接终止，避免破坏黏性选路预期。
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}

	// 渠道错误通常意味着可以尝试下一条渠道，优先快速放行。
	if types.IsChannelError(openaiErr) {
		return true
	}

	// 显式标记为 skip-retry 的错误必须立即终止，避免无意义重复请求。
	if types.IsSkipRetryError(openaiErr) {
		return false
	}

	// 没有剩余重试次数时，后续状态码判断也没有意义，直接结束。
	if retryTimes <= 0 {
		return false
	}

	// 如果调用方锁定了 specific channel，则不允许切换渠道重试。
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	code := openaiErr.StatusCode

	// 成功状态码不应进入重试。
	if code >= 200 && code < 300 {
		return false
	}

	// 非法状态码说明错误并非标准 HTTP 结果，保守起见允许重试。
	if code < 100 || code > 599 {
		return true
	}

	// 业务上被明确声明为“永不重试”的错误码要优先拦截。
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}

	// 其余情况交给状态码配置表统一决定。
	return operation_setting.ShouldRetryByStatusCode(code)
}

// processChannelError 统一处理渠道级失败，包括日志、自动禁用和错误日志落库。
// 参数：
//   - c：当前请求上下文，用于提取用户、令牌、渠道、耗时等诊断信息。
//   - channelError：当前失败渠道的结构化信息，会被用于禁用渠道和错误诊断。
//   - err：本轮请求最终得到的统一错误对象。
func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	// 先打印标准化渠道错误日志，便于线上快速定位是哪条渠道在什么状态码下失败。
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.Error())))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously

	// 如果当前错误满足自动封禁条件，就异步触发渠道禁用，避免坏渠道持续被选中。
	if service.ShouldDisableChannel(err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.ErrorWithStatusCode())
		})
	}

	// 如果开启了错误日志功能，则尽可能采集完整上下文并落库，供后台审计和排障使用。
	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		service.AppendChannelAffinityAdminInfo(c, adminInfo)
		other["admin_info"] = adminInfo
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	}

}

// RelayMidjourney 是 Midjourney 相关接口的统一入口，负责生成上下文并分派到具体动作处理器。
// 参数：
//   - c：当前请求上下文，包含已完成的认证、分发以及渠道选择结果。
func RelayMidjourney(c *gin.Context) {
	// Midjourney 也复用统一 RelayInfo，只是入口格式改为专用的 MjProxy。
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	// 按 Midjourney 的具体动作类型分发到不同处理器，覆盖提交、查询、换脸等多种模式。
	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)

	// 统一转换 Midjourney 错误响应格式，并补充渠道级错误日志。
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

// RelayNotImplemented 返回尚未实现的兼容接口响应。
// 参数：
//   - c：当前请求上下文，用于向客户端输出固定的 501 错误结构。
func RelayNotImplemented(c *gin.Context) {
	// 统一包装成 OpenAI 风格错误对象，方便兼容客户端处理。
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

// RelayNotFound 返回 Relay 体系内的未匹配路由错误。
// 参数：
//   - c：当前请求上下文，用于读取请求方法和路径并输出 404 响应。
func RelayNotFound(c *gin.Context) {
	// 把请求方法和路径带回给客户端，便于调试错误调用地址。
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

// RelayTaskFetch 处理异步任务结果查询请求，例如按任务 ID 拉取当前状态。
// 参数：
//   - c：当前请求上下文，里面已经包含调用身份和渠道上下文。
func RelayTaskFetch(c *gin.Context) {
	// 先为任务查询请求生成统一 RelayInfo，保证任务查询也能复用公共上下文。
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	// 调用任务查询逻辑；如果上游返回错误，则统一改写为 TaskError 输出。
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

// RelayTask 是异步任务提交入口，负责预扣费、选路、提交任务、重试以及任务落库。
// 参数：
//   - c：当前请求上下文，贯穿任务提交、计费和错误处理全过程。
func RelayTask(c *gin.Context) {
	// 先生成任务模式下的 RelayInfo，作为整个异步提交流程的共享上下文。
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	// 解析任务原始请求并完成预处理，这一步会把动作、模型和计费基础信息补齐。
	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}

	// 初始化任务提交阶段要复用的结果和错误对象。
	var result *relay.TaskSubmitResult
	var taskErr *dto.TaskError

	// 如果任务提交流程最终失败，则把已经预扣的额度退回去。
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	// 初始化任务提交的重试参数，后续会按模型与分组重复尝试不同渠道。
	retryParam := &service.RetryParam{
		Ctx:        c,
		TokenGroup: relayInfo.TokenGroup,
		ModelName:  relayInfo.OriginModelName,
		Retry:      common.GetPointer(0),
	}

	// 主提交流程重试循环：每轮选择渠道、恢复请求体、提交任务并按错误结果决定是否继续。
	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		var channel *model.Channel

		// 如果上游流程已经锁定了渠道，则优先复用锁定渠道，保证任务和后续轮询的一致性。
		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh

			// 锁定渠道在重试时也要重新写回 context，确保后续 helper 读取到完整渠道配置。
			if retryParam.GetRetry() > 0 {
				if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
					taskErr = service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
					break
				}
			}
		} else {
			// 普通场景下每轮都重新选择一个可用渠道，和同步 Relay 保持一致的选路策略。
			var channelErr *types.NewAPIError
			channel, channelErr = getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusInternalServerError)
				break
			}
		}

		// 记录本轮命中的渠道，为后续重试链路和错误日志提供上下文。
		addUsedChannel(c, channel.Id)

		// 每轮提交前都要恢复请求体，避免因为上一次读取导致后续重试拿不到内容。
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		// 执行真正的异步任务提交；成功时会拿到上游任务 ID、额度和任务初始数据。
		result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
		if taskErr == nil {
			break
		}

		// 非本地错误通常代表渠道或上游失败，需要进入统一渠道错误处理链路。
		if !taskErr.LocalError {
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
				types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode))
		}

		// 根据异步任务自己的重试规则判断是否继续尝试其他渠道。
		if !shouldRetryTaskRelay(c, channel.Id, taskErr, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	// 如果任务提交过程中切换过多个渠道，这里输出完整重试轨迹。
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// ── 成功：结算 + 日志 + 插入任务 ──
	if taskErr == nil {
		// 提交成功后，先把预扣费结算成真实额度消费。
		if settleErr := service.SettleBilling(c, relayInfo, result.Quota); settleErr != nil {
			common.SysError("settle task billing error: " + settleErr.Error())
		}

		// 记录本次任务型消费日志，保证异步任务也能进入统一账单统计。
		service.LogTaskConsumption(c, relayInfo)

		// 将任务的关键运行时信息持久化到本地任务表，供后续轮询和前端查询使用。
		task := model.InitTask(result.Platform, relayInfo)
		task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
		task.PrivateData.BillingSource = relayInfo.BillingSource
		task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
		task.PrivateData.TokenId = relayInfo.TokenId
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			ModelPrice:      relayInfo.PriceData.ModelPrice,
			GroupRatio:      relayInfo.PriceData.GroupRatioInfo.GroupRatio,
			ModelRatio:      relayInfo.PriceData.ModelRatio,
			OtherRatios:     relayInfo.PriceData.OtherRatios,
			OriginModelName: relayInfo.OriginModelName,
			PerCallBilling:  common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice,
		}
		task.Quota = result.Quota
		task.Data = result.TaskData
		task.Action = relayInfo.Action
		if insertErr := task.Insert(); insertErr != nil {
			common.SysError("insert task error: " + insertErr.Error())
		}
	}

	// 如果任务提交最终失败，则统一按 TaskError 格式返回给客户端。
	if taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
// 参数：
//   - c：当前请求上下文，用于输出任务接口约定的错误响应。
//   - taskErr：已经标准化的任务错误对象，包含状态码、错误码和消息。
func respondTaskError(c *gin.Context, taskErr *dto.TaskError) {
	// 对上游限流错误做统一文案改写，避免将过于底层的错误信息直接暴露给用户。
	if taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

// shouldRetryTaskRelay 根据任务接口的错误类型判断异步任务提交是否应该继续重试。
// 参数：
//   - c：当前请求上下文，用于判断 specific channel、Affinity 等运行时限制。
//   - channelId：当前轮失败的渠道 ID；此参数主要用于语义表达，当前函数内部未直接使用。
//   - taskErr：本轮任务提交得到的任务错误对象。
//   - retryTimes：剩余可用重试次数。
//
// 返回：
//   - bool：true 表示可以继续尝试其他渠道，false 表示应立即停止。
func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	// 没有错误说明提交已经成功，不需要再继续判断。
	if taskErr == nil {
		return false
	}

	// 命中 Affinity 禁止重试策略时，任务提交流程也要遵守同样约束。
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}

	// 没有剩余次数时直接结束，避免无效循环。
	if retryTimes <= 0 {
		return false
	}

	// 指定了 specific channel 的请求不能切换渠道重试。
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}

	// 上游限流通常意味着换其他渠道仍有成功机会，因此允许重试。
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}

	// 307 常被某些上游实现用作重定向或中间跳转异常，这里保守允许重试。
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			return false
		}
		return true
	}

	// 请求参数错误属于调用方问题，切换渠道也无法修复，不应重试。
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}

	// 本地错误通常表示网关侧前置流程失败，换渠道没有意义。
	if taskErr.LocalError {
		return false
	}

	// 2xx 说明提交已经成功，不应该继续重试。
	if taskErr.StatusCode/100 == 2 {
		return false
	}

	// 其余未明确排除的情况，默认保守放行一次重试机会。
	return true
}

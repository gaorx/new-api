package controller

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Playground 以当前登录用户身份进入 Playground 转发流程，并复用标准 Relay 逻辑。
// 参数：
//   - c：当前请求上下文，用于生成 relay 信息、写入用户上下文并发起转发。
func Playground(c *gin.Context) {
	// 统一捕获 new-api 错误并转换为 OpenAI 兼容错误格式返回。
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	// Playground 不支持使用 access token，必须走控制台登录态。
	useAccessToken := c.GetBool("use_access_token")
	if useAccessToken {
		newAPIError = types.NewError(errors.New("暂不支持使用 access token"), types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
		return
	}

	// 基于当前请求生成标准 relay 上下文信息。
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		return
	}

	userId := c.GetInt("id")

	// Write user context to ensure acceptUnsetRatio is available
	// 读取用户缓存并写回 gin 上下文，确保后续分组和倍率相关逻辑可正常使用。
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		return
	}
	userCache.WriteContext(c)

	// 构造一个临时 token 上下文，让后续 Relay 流程能够复用标准 token 处理逻辑。
	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", relayInfo.UsingGroup),
		Group:  relayInfo.UsingGroup,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	// 进入标准 OpenAI Relay 流程处理 Playground 请求。
	Relay(c, types.RelayFormatOpenAI)
}

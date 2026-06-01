package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/codex"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// codexOAuthCompleteRequest 表示完成 Codex OAuth 授权时提交的请求体。
type codexOAuthCompleteRequest struct {
	Input string `json:"input"` // 用户粘贴的授权回调内容，可以是 code/state 组合或完整 URL。
}

// codexOAuthSessionKey 生成当前会话中保存 Codex OAuth 临时状态的键名。
// 参数：
//   - channelID：目标渠道 ID；为 0 时表示生成独立 key 而不直接绑定渠道。
//   - field：要保存的具体字段名，如 state、verifier、created_at。
//
// 返回：
//   - string：拼接后的 session 键名。
func codexOAuthSessionKey(channelID int, field string) string {
	// 通过 field 和 channelID 组合出唯一 session key，避免不同渠道流程互相覆盖。
	return fmt.Sprintf("codex_oauth_%s_%d", field, channelID)
}

// parseCodexAuthorizationInput 从用户输入中解析出 code 和 state。
// 参数：
//   - input：用户输入的授权结果文本，可以是 code#state、URL 或 query string。
//
// 返回：
//   - code：解析出的授权码。
//   - state：解析出的 state 值。
//   - err：输入为空或无法解析时返回错误。
func parseCodexAuthorizationInput(input string) (code string, state string, err error) {
	// 先做 trim，空输入直接视为非法。
	v := strings.TrimSpace(input)
	if v == "" {
		return "", "", errors.New("empty input")
	}
	// 支持 code#state 这种简化输入格式。
	if strings.Contains(v, "#") {
		parts := strings.SplitN(v, "#", 2)
		code = strings.TrimSpace(parts[0])
		state = strings.TrimSpace(parts[1])
		return code, state, nil
	}
	// 支持完整 URL 或原始 query string，两者都尝试从 code/state 参数中提取。
	if strings.Contains(v, "code=") {
		u, parseErr := url.Parse(v)
		if parseErr == nil {
			q := u.Query()
			code = strings.TrimSpace(q.Get("code"))
			state = strings.TrimSpace(q.Get("state"))
			return code, state, nil
		}
		q, parseErr := url.ParseQuery(v)
		if parseErr == nil {
			code = strings.TrimSpace(q.Get("code"))
			state = strings.TrimSpace(q.Get("state"))
			return code, state, nil
		}
	}

	// 如果以上格式都不匹配，则退化为把整个输入当作 code。
	code = v
	return code, "", nil
}

// StartCodexOAuth 启动不直接绑定具体渠道的 Codex OAuth 授权流程。
// 参数：
//   - c：当前请求上下文，用于返回授权地址。
func StartCodexOAuth(c *gin.Context) {
	// 复用统一入口逻辑，并用 channelID=0 表示只生成凭证而不直接保存到渠道。
	startCodexOAuthWithChannelID(c, 0)
}

// StartCodexOAuthForChannel 启动某个指定 Codex 渠道的 OAuth 授权流程。
// 参数：
//   - c：当前请求上下文，用于读取渠道 ID 并返回授权地址。
func StartCodexOAuthForChannel(c *gin.Context) {
	// 先解析路径中的渠道 ID，再进入带渠道绑定的授权流程。
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	startCodexOAuthWithChannelID(c, channelID)
}

// startCodexOAuthWithChannelID 创建 Codex OAuth 授权流，并把 state/verifier 保存到 session。
// 参数：
//   - c：当前请求上下文，用于写入 session 并返回 authorize_url。
//   - channelID：目标渠道 ID；大于 0 时会校验该渠道必须存在且为 Codex 类型。
func startCodexOAuthWithChannelID(c *gin.Context, channelID int) {
	// 如果传入了渠道 ID，则先校验渠道存在且类型正确。
	if channelID > 0 {
		ch, err := model.GetChannelById(channelID, false)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if ch == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
			return
		}
		if ch.Type != constant.ChannelTypeCodex {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not Codex"})
			return
		}
	}

	// 创建授权流，拿到授权地址、state 和 PKCE verifier。
	flow, err := service.CreateCodexOAuthAuthorizationFlow()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 把本次授权流程的 state、verifier 和创建时间写入 session，供回调完成时校验。
	session := sessions.Default(c)
	session.Set(codexOAuthSessionKey(channelID, "state"), flow.State)
	session.Set(codexOAuthSessionKey(channelID, "verifier"), flow.Verifier)
	session.Set(codexOAuthSessionKey(channelID, "created_at"), time.Now().Unix())
	_ = session.Save()

	// 返回前端需要跳转的授权地址。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"authorize_url": flow.AuthorizeURL,
		},
	})
}

// CompleteCodexOAuth 完成不直接绑定渠道的 Codex OAuth 授权流程。
// 参数：
//   - c：当前请求上下文，用于读取授权输入并返回生成的凭证。
func CompleteCodexOAuth(c *gin.Context) {
	// 复用统一完成逻辑，并用 channelID=0 表示只生成 key。
	completeCodexOAuthWithChannelID(c, 0)
}

// CompleteCodexOAuthForChannel 完成某个指定 Codex 渠道的 OAuth 授权流程。
// 参数：
//   - c：当前请求上下文，用于读取渠道 ID 与授权输入。
func CompleteCodexOAuthForChannel(c *gin.Context) {
	// 先解析路径中的渠道 ID，再进入带渠道绑定的完成逻辑。
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	completeCodexOAuthWithChannelID(c, channelID)
}

// completeCodexOAuthWithChannelID 完成授权码交换、提取账户信息，并选择保存到渠道或直接返回。
// 参数：
//   - c：当前请求上下文，用于读取输入、校验 session、调用交换接口并输出结果。
//   - channelID：目标渠道 ID；为 0 时只返回编码后的 key。
func completeCodexOAuthWithChannelID(c *gin.Context, channelID int) {
	// 先解析请求体，拿到用户输入的授权信息。
	req := codexOAuthCompleteRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// 从用户输入中拆解出 code 和 state，并对缺失字段给出明确提示。
	code, state, err := parseCodexAuthorizationInput(req.Input)
	if err != nil {
		common.SysError("failed to parse codex authorization input: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "解析授权信息失败，请检查输入格式"})
		return
	}
	if strings.TrimSpace(code) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "missing authorization code"})
		return
	}
	if strings.TrimSpace(state) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "missing state in input"})
		return
	}

	// 如果是绑定到现有渠道的流程，则先校验目标渠道类型并读取其代理配置。
	channelProxy := ""
	if channelID > 0 {
		ch, err := model.GetChannelById(channelID, false)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if ch == nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel not found"})
			return
		}
		if ch.Type != constant.ChannelTypeCodex {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel type is not Codex"})
			return
		}
		channelProxy = ch.GetSetting().Proxy
	}

	// 从 session 中读取发起授权时保存的 state 和 verifier，并校验 state 一致性。
	session := sessions.Default(c)
	expectedState, _ := session.Get(codexOAuthSessionKey(channelID, "state")).(string)
	verifier, _ := session.Get(codexOAuthSessionKey(channelID, "verifier")).(string)
	if strings.TrimSpace(expectedState) == "" || strings.TrimSpace(verifier) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "oauth flow not started or session expired"})
		return
	}
	if state != expectedState {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "state mismatch"})
		return
	}

	// 使用授权码和 PKCE verifier 与上游交换 access token / refresh token。
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	tokenRes, err := service.ExchangeCodexAuthorizationCodeWithProxy(ctx, code, verifier, channelProxy)
	if err != nil {
		common.SysError("failed to exchange codex authorization code: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "授权码交换失败，请重试"})
		return
	}

	// 从 access token 中提取 account_id 和 email，并编码成 Codex OAuthKey。
	accountID, ok := service.ExtractCodexAccountIDFromJWT(tokenRes.AccessToken)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "failed to extract account_id from access_token"})
		return
	}
	email, _ := service.ExtractEmailFromJWT(tokenRes.AccessToken)

	key := codex.OAuthKey{
		AccessToken:  tokenRes.AccessToken,
		RefreshToken: tokenRes.RefreshToken,
		AccountID:    accountID,
		LastRefresh:  time.Now().Format(time.RFC3339),
		Expired:      tokenRes.ExpiresAt.Format(time.RFC3339),
		Email:        email,
		Type:         "codex",
	}
	encoded, err := common.Marshal(key)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 授权成功后清理 session 中的临时状态，避免重复使用旧流程。
	session.Delete(codexOAuthSessionKey(channelID, "state"))
	session.Delete(codexOAuthSessionKey(channelID, "verifier"))
	session.Delete(codexOAuthSessionKey(channelID, "created_at"))
	_ = session.Save()

	// 带渠道 ID 的流程会把生成的 OAuthKey 直接写回渠道，并刷新运行时缓存。
	if channelID > 0 {
		if err := model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("key", string(encoded)).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		model.InitChannelCache()
		service.ResetProxyClientCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "saved",
			"data": gin.H{
				"channel_id":   channelID,
				"account_id":   accountID,
				"email":        email,
				"expires_at":   key.Expired,
				"last_refresh": key.LastRefresh,
			},
		})
		return
	}

	// 否则直接把编码后的 key 返回给前端，由调用方自行处理后续保存。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "generated",
		"data": gin.H{
			"key":          string(encoded),
			"account_id":   accountID,
			"email":        email,
			"expires_at":   key.Expired,
			"last_refresh": key.LastRefresh,
		},
	})
}

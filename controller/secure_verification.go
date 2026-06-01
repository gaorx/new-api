package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	// SecureVerificationSessionKey means the user has fully passed secure verification.
	SecureVerificationSessionKey       = "secure_verified_at"
	secureVerificationMethodSessionKey = "secure_verified_method"
	secureVerificationMethod2FA        = "2fa"
	secureVerificationMethodPasskey    = "passkey"
	// PasskeyReadySessionKey means WebAuthn finished and /api/verify can finalize step-up verification.
	PasskeyReadySessionKey = "secure_passkey_ready_at"
	// SecureVerificationTimeout 验证有效期（秒）
	SecureVerificationTimeout = 300 // 5分钟
	// PasskeyReadyTimeout passkey ready 标记有效期（秒）
	PasskeyReadyTimeout = 60
)

// UniversalVerifyRequest 表示通用安全验证接口的请求体。
type UniversalVerifyRequest struct {
	Method string `json:"method"`          // 验证方式，支持 "2fa" 或 "passkey"。
	Code   string `json:"code,omitempty"` // 2FA 验证码；Passkey 流程下可为空。
}

// VerificationStatusResponse 表示安全验证状态的响应结构。
type VerificationStatusResponse struct {
	Verified  bool  `json:"verified"`             // 当前是否已通过安全验证。
	ExpiresAt int64 `json:"expires_at,omitempty"` // 验证状态过期时间戳。
}

// UniversalVerify 通用安全验证接口。
// 支持 2FA 和 Passkey 两种验证方式，验证成功后会在 session 中写入短期已验证状态。
// 参数：
//   - c：当前请求上下文，用于读取登录用户、验证参数并写入 session。
func UniversalVerify(c *gin.Context) {
	// 仅允许已登录用户发起二次安全验证。
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录",
		})
		return
	}

	// 解析通用验证请求体。
	var req UniversalVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, fmt.Errorf("参数错误: %v", err))
		return
	}

	// 读取用户信息，并确认账号当前仍然处于启用状态。
	user := &model.User{Id: userId}
	if err := user.FillUserById(); err != nil {
		common.ApiError(c, fmt.Errorf("获取用户信息失败: %v", err))
		return
	}

	if user.Status != common.UserStatusEnabled {
		common.ApiError(c, fmt.Errorf("该用户已被禁用"))
		return
	}

	// 检查用户已启用的验证方式，至少需要有 2FA 或 Passkey 之一。
	twoFA, _ := model.GetTwoFAByUserId(userId)
	has2FA := twoFA != nil && twoFA.IsEnabled

	passkey, passkeyErr := model.GetPasskeyByUserID(userId)
	hasPasskey := passkeyErr == nil && passkey != nil

	if !has2FA && !hasPasskey {
		common.ApiError(c, fmt.Errorf("用户未启用2FA或Passkey"))
		return
	}

	// 根据请求指定的验证方式执行对应校验逻辑。
	var verified bool
	var verifyMethod string
	var err error

	switch req.Method {
	case "2fa":
		// 2FA 模式下要求用户已启用 2FA 且必须提交验证码。
		if !has2FA {
			common.ApiError(c, fmt.Errorf("用户未启用2FA"))
			return
		}
		if req.Code == "" {
			common.ApiError(c, fmt.Errorf("验证码不能为空"))
			return
		}
		verified = validateTwoFactorAuth(twoFA, req.Code)
		verifyMethod = "2FA"

	case "passkey":
		// Passkey 模式下只消费前一步 Passkey 流程写入的短期就绪标记。
		if !hasPasskey {
			common.ApiError(c, fmt.Errorf("用户未启用Passkey"))
			return
		}
		// Passkey branch only trusts the short-lived marker written by PasskeyVerifyFinish.
		verified, err = consumePasskeyReady(c)
		if err != nil {
			common.ApiError(c, fmt.Errorf("Passkey 验证状态异常: %v", err))
			return
		}
		if !verified {
			common.ApiError(c, fmt.Errorf("请先完成 Passkey 验证"))
			return
		}
		verifyMethod = "Passkey"

	default:
		// 不支持的验证方式直接返回错误。
		common.ApiError(c, fmt.Errorf("不支持的验证方式: %s", req.Method))
		return
	}

	// 校验失败时统一返回验证失败提示。
	if !verified {
		common.ApiError(c, fmt.Errorf("验证失败，请检查验证码"))
		return
	}

	// 验证成功后，在 session 中记录验证时间和验证方式。
	now, err := setSecureVerificationSession(c, req.Method)
	if err != nil {
		common.ApiError(c, fmt.Errorf("保存验证状态失败: %v", err))
		return
	}

	// 记录一条系统日志，便于审计用户完成了二次验证。
	model.RecordLog(userId, model.LogTypeSystem, fmt.Sprintf("通用安全验证成功 (验证方式: %s)", verifyMethod))

	// 返回验证成功结果和本次验证状态的过期时间。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "验证成功",
		"data": gin.H{
			"verified":   true,
			"expires_at": now + SecureVerificationTimeout,
		},
	})
}

// setSecureVerificationSession 在 session 中写入“已完成安全验证”的状态。
// 参数：
//   - c：当前请求上下文，用于访问并保存 session。
//   - method：本次验证所使用的方法标识。
//
// 返回：
//   - int64：写入 session 的当前时间戳。
//   - error：保存 session 失败时返回错误。
func setSecureVerificationSession(c *gin.Context, method string) (int64, error) {
	// 写入正式验证状态前，先清理 passkey ready 的一次性标记。
	session := sessions.Default(c)
	session.Delete(PasskeyReadySessionKey)
	now := time.Now().Unix()
	// 保存验证时间和验证方式，供后续安全敏感操作判断。
	session.Set(SecureVerificationSessionKey, now)
	session.Set(secureVerificationMethodSessionKey, method)
	if err := session.Save(); err != nil {
		return 0, err
	}
	return now, nil
}

// consumePasskeyReady 消费一次性的 Passkey ready 标记。
// 参数：
//   - c：当前请求上下文，用于读取并清除 session 中的 ready 标记。
//
// 返回：
//   - bool：true 表示 ready 标记有效且已被成功消费。
//   - error：session 状态非法或保存失败时返回错误。
func consumePasskeyReady(c *gin.Context) (bool, error) {
	// 从 session 中读取 Passkey 完成流程预写入的 ready 时间戳。
	session := sessions.Default(c)
	readyAtRaw := session.Get(PasskeyReadySessionKey)
	if readyAtRaw == nil {
		return false, nil
	}

	// 若标记类型异常，则清理无效状态并返回错误。
	readyAt, ok := readyAtRaw.(int64)
	if !ok {
		session.Delete(PasskeyReadySessionKey)
		_ = session.Save()
		return false, fmt.Errorf("无效的 Passkey 验证状态")
	}
	// 该标记只能使用一次，因此无论结果如何都先删除并保存。
	session.Delete(PasskeyReadySessionKey)
	if err := session.Save(); err != nil {
		return false, err
	}
	// Expired ready markers cannot be reused.
	// 超过有效期的 ready 标记视为过期，不允许继续完成安全验证。
	if time.Now().Unix()-readyAt >= PasskeyReadyTimeout {
		return false, nil
	}
	return true, nil
}

package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// TelegramBind 将当前登录用户与 Telegram 账户绑定。
// 参数：
//   - c：当前请求上下文，用于读取 Telegram 登录参数并完成绑定。
func TelegramBind(c *gin.Context) {
	// 未启用 Telegram OAuth 时，不允许进行绑定。
	if !common.TelegramOAuthEnabled {
		c.JSON(200, gin.H{
			"message": "管理员未开启通过 Telegram 登录以及注册",
			"success": false,
		})
		return
	}
	// 校验 Telegram 回调参数签名，防止伪造请求。
	params := c.Request.URL.Query()
	if !checkTelegramAuthorization(params, common.TelegramBotToken) {
		c.JSON(200, gin.H{
			"message": "无效的请求",
			"success": false,
		})
		return
	}
	// 提取 Telegram 用户 ID，并检查是否已被其他账号绑定。
	telegramId := params["id"][0]
	if model.IsTelegramIdAlreadyTaken(telegramId) {
		c.JSON(200, gin.H{
			"message": "该 Telegram 账户已被绑定",
			"success": false,
		})
		return
	}

	// 从 session 中获取当前登录用户，并加载完整用户记录。
	session := sessions.Default(c)
	id := session.Get("id")
	user := model.User{Id: id.(int)}
	if err := user.FillUserById(); err != nil {
		c.JSON(200, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	if user.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户已注销",
		})
		return
	}
	// 把 TelegramId 写回当前用户，完成绑定。
	user.TelegramId = telegramId
	if err := user.Update(false); err != nil {
		c.JSON(200, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}

	// 绑定成功后跳转回个人设置页。
	c.Redirect(302, common.ThemeAwarePath("/console/personal"))
}

// TelegramLogin 处理 Telegram 登录回调。
// 参数：
//   - c：当前请求上下文，用于读取 Telegram 参数并完成登录。
func TelegramLogin(c *gin.Context) {
	// 未启用 Telegram OAuth 时，不允许通过 Telegram 登录。
	if !common.TelegramOAuthEnabled {
		c.JSON(200, gin.H{
			"message": "管理员未开启通过 Telegram 登录以及注册",
			"success": false,
		})
		return
	}
	// 校验 Telegram 登录回调参数签名。
	params := c.Request.URL.Query()
	if !checkTelegramAuthorization(params, common.TelegramBotToken) {
		c.JSON(200, gin.H{
			"message": "无效的请求",
			"success": false,
		})
		return
	}

	// 读取 Telegram 用户 ID 并查找已绑定用户。
	telegramId := params["id"][0]
	user := model.User{TelegramId: telegramId}
	if err := user.FillUserByTelegramId(); err != nil {
		c.JSON(200, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	// 进入统一登录收尾逻辑。
	setupLogin(&user, c)
}

// checkTelegramAuthorization 校验 Telegram Login Widget 回调参数签名。
// 参数：
//   - params：请求 URL 中解析出的参数集合。
//   - token：Telegram Bot Token。
//
// 返回：
//   - bool：true 表示签名校验通过。
func checkTelegramAuthorization(params map[string][]string, token string) bool {
	// 先把除 hash 外的全部参数按要求拼接成待签名字符串。
	strs := []string{}
	var hash = ""
	for k, v := range params {
		if k == "hash" {
			hash = v[0]
			continue
		}
		strs = append(strs, k+"="+v[0])
	}
	sort.Strings(strs)
	var imploded = ""
	for _, s := range strs {
		if imploded != "" {
			imploded += "\n"
		}
			imploded += s
	}
	// 按 Telegram 规则用 bot token 派生密钥，再计算 HMAC-SHA256。
	sha256hash := sha256.New()
	io.WriteString(sha256hash, token)
	hmachash := hmac.New(sha256.New, sha256hash.Sum(nil))
	io.WriteString(hmachash, imploded)
	ss := hex.EncodeToString(hmachash.Sum(nil))
	// 只有签名完全一致时才认为请求可信。
	return hash == ss
}

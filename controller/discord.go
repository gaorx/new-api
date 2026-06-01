package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// DiscordResponse 表示 Discord OAuth token 交换接口的响应结构。
type DiscordResponse struct {
	AccessToken  string `json:"access_token"`  // 访问令牌。
	IDToken      string `json:"id_token"`      // OIDC ID Token。
	RefreshToken string `json:"refresh_token"` // 刷新令牌。
	TokenType    string `json:"token_type"`    // 令牌类型。
	ExpiresIn    int    `json:"expires_in"`    // 过期秒数。
	Scope        string `json:"scope"`         // 授权 scopes。
}

// DiscordUser 表示 Discord 用户信息接口返回的用户对象。
type DiscordUser struct {
	UID  string `json:"id"`          // Discord 用户唯一 ID。
	ID   string `json:"username"`    // Discord 用户名。
	Name string `json:"global_name"` // Discord 全局显示名。
}

// getDiscordUserInfoByCode 使用授权码从 Discord 获取用户信息。
// 参数：
//   - code：OAuth 回调里返回的授权码。
//
// 返回：
//   - *DiscordUser：解析出的 Discord 用户信息。
//   - error：授权码无效、网络请求失败或用户信息异常时返回错误。
func getDiscordUserInfoByCode(code string) (*DiscordUser, error) {
	// 空授权码直接视为无效参数。
	if code == "" {
		return nil, errors.New("无效的参数")
	}

	// 先构造 token 交换表单，向 Discord token 接口换取 access token。
	values := url.Values{}
	values.Set("client_id", system_setting.GetDiscordSettings().ClientId)
	values.Set("client_secret", system_setting.GetDiscordSettings().ClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", fmt.Sprintf("%s/oauth/discord", system_setting.ServerAddress))
	formData := values.Encode()
	req, err := http.NewRequest("POST", "https://discord.com/api/v10/oauth2/token", strings.NewReader(formData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	// 使用带短超时的 HTTP 客户端请求 Discord，避免长时间阻塞登录流程。
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 Discord 服务器，请稍后重试！")
	}
	defer res.Body.Close()
	// 解析 token 响应，后续用 access token 继续获取用户详情。
	var discordResponse DiscordResponse
	err = json.NewDecoder(res.Body).Decode(&discordResponse)
	if err != nil {
		return nil, err
	}

	if discordResponse.AccessToken == "" {
		common.SysError("Discord 获取 Token 失败，请检查设置！")
		return nil, errors.New("Discord 获取 Token 失败，请检查设置！")
	}

	// 使用 access token 调 Discord 用户信息接口，读取当前授权用户资料。
	req, err = http.NewRequest("GET", "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+discordResponse.AccessToken)
	res2, err := client.Do(req)
	if err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 Discord 服务器，请稍后重试！")
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		common.SysError("Discord 获取用户信息失败！请检查设置！")
		return nil, errors.New("Discord 获取用户信息失败！请检查设置！")
	}

	// 解析用户信息，并校验关键标识字段不能为空。
	var discordUser DiscordUser
	err = json.NewDecoder(res2.Body).Decode(&discordUser)
	if err != nil {
		return nil, err
	}
	if discordUser.UID == "" || discordUser.ID == "" {
		common.SysError("Discord 获取用户信息为空！请检查设置！")
		return nil, errors.New("Discord 获取用户信息为空！请检查设置！")
	}
	return &discordUser, nil
}

// DiscordOAuth 处理 Discord OAuth 登录/注册回调。
// 参数：
//   - c：当前请求上下文，用于校验 state、获取用户信息并完成登录。
func DiscordOAuth(c *gin.Context) {
	// 先校验 OAuth state，防止 CSRF 或伪造回调。
	session := sessions.Default(c)
	state := c.Query("state")
	if state == "" || session.Get("oauth_state") == nil || state != session.Get("oauth_state").(string) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "state is empty or not same",
		})
		return
	}
	// 已登录用户进入绑定流程，未登录用户进入登录/注册流程。
	username := session.Get("username")
	if username != nil {
		DiscordBind(c)
		return
	}
	// 如果管理员未启用 Discord 登录，则直接拒绝回调处理。
	if !system_setting.GetDiscordSettings().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Discord 登录以及注册",
		})
		return
	}
	// 使用授权码换取 Discord 用户信息。
	code := c.Query("code")
	discordUser, err := getDiscordUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 先按 DiscordId 查找已绑定用户；不存在时按注册开关决定是否自动创建账户。
	user := model.User{
		DiscordId: discordUser.UID,
	}
	if model.IsDiscordIdAlreadyTaken(user.DiscordId) {
		err := user.FillUserByDiscordId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	} else {
		if common.RegisterEnabled {
			if discordUser.ID != "" {
				user.Username = discordUser.ID
			} else {
				user.Username = "discord_" + strconv.Itoa(model.GetMaxUserId()+1)
			}
			if discordUser.Name != "" {
				user.DisplayName = discordUser.Name
			} else {
				user.DisplayName = "Discord User"
			}
			err := user.Insert(0)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		} else {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "管理员关闭了新用户注册",
			})
			return
		}
	}

	// 新建或读取出来的用户如果被禁用，则禁止继续登录。
	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "用户已被封禁",
			"success": false,
		})
		return
	}
	// 进入统一登录完成流程，写入 session 并返回登录结果。
	setupLogin(&user, c)
}

// DiscordBind 把当前登录用户与一个 Discord 账号进行绑定。
// 参数：
//   - c：当前请求上下文，用于读取 OAuth 回调 code 并更新用户绑定关系。
func DiscordBind(c *gin.Context) {
	// 只有启用了 Discord 登录/绑定时，才允许执行绑定流程。
	if !system_setting.GetDiscordSettings().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Discord 登录以及注册",
		})
		return
	}
	// 使用授权码获取 Discord 用户信息，作为绑定目标。
	code := c.Query("code")
	discordUser, err := getDiscordUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 如果该 Discord 账号已被其他用户绑定，则直接拒绝重复绑定。
	user := model.User{
		DiscordId: discordUser.UID,
	}
	if model.IsDiscordIdAlreadyTaken(user.DiscordId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该 Discord 账户已被绑定",
		})
		return
	}
	// 取出当前 session 用户，更新其 DiscordId 并保存。
	session := sessions.Default(c)
	id := session.Get("id")
	user.Id = id.(int)
	err = user.FillUserById()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user.DiscordId = discordUser.UID
	err = user.Update(false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回绑定成功结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "bind",
	})
}

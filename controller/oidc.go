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

// OidcResponse 表示 OIDC token 交换接口的响应结构。
type OidcResponse struct {
	AccessToken  string `json:"access_token"`  // 访问令牌。
	IDToken      string `json:"id_token"`      // OIDC ID Token。
	RefreshToken string `json:"refresh_token"` // 刷新令牌。
	TokenType    string `json:"token_type"`    // 令牌类型。
	ExpiresIn    int    `json:"expires_in"`    // 令牌有效期秒数。
	Scope        string `json:"scope"`         // 授权 scopes。
}

// OidcUser 表示 OIDC UserInfo 接口返回的用户资料。
type OidcUser struct {
	OpenID            string `json:"sub"`                // OIDC subject，作为唯一用户标识。
	Email             string `json:"email"`              // 用户邮箱。
	Name              string `json:"name"`               // 用户展示名。
	PreferredUsername string `json:"preferred_username"` // 用户偏好用户名。
	Picture           string `json:"picture"`            // 头像地址。
}

// getOidcUserInfoByCode 使用 OIDC 授权码换取用户信息。
// 参数：
//   - code：OIDC 回调中的授权码。
//
// 返回：
//   - *OidcUser：成功获取到的 OIDC 用户资料。
//   - error：授权码无效、网络失败或配置错误时返回错误。
func getOidcUserInfoByCode(code string) (*OidcUser, error) {
	// 授权码为空时直接返回参数错误。
	if code == "" {
		return nil, errors.New("无效的参数")
	}

	// 先用授权码向 OIDC token endpoint 换取 access token。
	values := url.Values{}
	values.Set("client_id", system_setting.GetOIDCSettings().ClientId)
	values.Set("client_secret", system_setting.GetOIDCSettings().ClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", fmt.Sprintf("%s/oauth/oidc", system_setting.ServerAddress))
	formData := values.Encode()
	req, err := http.NewRequest("POST", system_setting.GetOIDCSettings().TokenEndpoint, strings.NewReader(formData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	// 调用 token 接口并解析 token 响应。
	res, err := client.Do(req)
	if err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 OIDC 服务器，请稍后重试！")
	}
	defer res.Body.Close()
	var oidcResponse OidcResponse
	err = json.NewDecoder(res.Body).Decode(&oidcResponse)
	if err != nil {
		return nil, err
	}

	if oidcResponse.AccessToken == "" {
		common.SysLog("OIDC 获取 Token 失败，请检查设置！")
		return nil, errors.New("OIDC 获取 Token 失败，请检查设置！")
	}

	// 再使用 access token 调用 UserInfo 接口。
	req, err = http.NewRequest("GET", system_setting.GetOIDCSettings().UserInfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+oidcResponse.AccessToken)
	res2, err := client.Do(req)
	if err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 OIDC 服务器，请稍后重试！")
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		common.SysLog("OIDC 获取用户信息失败！请检查设置！")
		return nil, errors.New("OIDC 获取用户信息失败！请检查设置！")
	}

	// 解析 UserInfo 响应，并确保主键字段与邮箱存在。
	var oidcUser OidcUser
	err = json.NewDecoder(res2.Body).Decode(&oidcUser)
	if err != nil {
		return nil, err
	}
	if oidcUser.OpenID == "" || oidcUser.Email == "" {
		common.SysLog("OIDC 获取用户信息为空！请检查设置！")
		return nil, errors.New("OIDC 获取用户信息为空！请检查设置！")
	}
	return &oidcUser, nil
}

// OidcAuth 处理 OIDC 登录或注册回调。
// 参数：
//   - c：当前请求上下文，用于读取回调参数、session 并完成登录。
func OidcAuth(c *gin.Context) {
	// 先校验 state，防止非法回调。
	session := sessions.Default(c)
	state := c.Query("state")
	if state == "" || session.Get("oauth_state") == nil || state != session.Get("oauth_state").(string) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "state is empty or not same",
		})
		return
	}
	// 当前 session 已登录时，本次回调视为绑定流程。
	username := session.Get("username")
	if username != nil {
		OidcBind(c)
		return
	}
	// OIDC 未启用时，不允许登录或注册。
	if !system_setting.GetOIDCSettings().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 OIDC 登录以及注册",
		})
		return
	}
	// 通过授权码拉取 OIDC 用户资料。
	code := c.Query("code")
	oidcUser, err := getOidcUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user := model.User{
		OidcId: oidcUser.OpenID,
	}
	// 若 OIDC 账户已存在，则读取现有用户；否则按注册逻辑创建新用户。
	if model.IsOidcIdAlreadyTaken(user.OidcId) {
		err := user.FillUserByOidcId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	} else {
		// 根据 OIDC 返回资料创建新用户。
		if common.RegisterEnabled {
			user.Email = oidcUser.Email
			if oidcUser.PreferredUsername != "" {
				user.Username = oidcUser.PreferredUsername
			} else {
				user.Username = "oidc_" + strconv.Itoa(model.GetMaxUserId()+1)
			}
			if oidcUser.Name != "" {
				user.DisplayName = oidcUser.Name
			} else {
				user.DisplayName = "OIDC User"
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

	// 被禁用用户不能通过 OIDC 登录。
	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "用户已被封禁",
			"success": false,
		})
		return
	}
	// 进入统一登录收尾逻辑。
	setupLogin(&user, c)
}

// OidcBind 将当前登录用户与 OIDC 账户绑定。
// 参数：
//   - c：当前请求上下文，用于读取 OIDC code 和当前登录用户 session。
func OidcBind(c *gin.Context) {
	// 未启用 OIDC 时，不允许执行绑定。
	if !system_setting.GetOIDCSettings().Enabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 OIDC 登录以及注册",
		})
		return
	}
	// 通过授权码获取待绑定的 OIDC 账户信息。
	code := c.Query("code")
	oidcUser, err := getOidcUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user := model.User{
		OidcId: oidcUser.OpenID,
	}
	// 已被其他用户绑定的 OIDC 账户不可重复绑定。
	if model.IsOidcIdAlreadyTaken(user.OidcId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该 OIDC 账户已被绑定",
		})
		return
	}
	// 从 session 获取当前登录用户并加载用户记录。
	session := sessions.Default(c)
	id := session.Get("id")
	// id := c.GetInt("id")  // critical bug!
	user.Id = id.(int)
	err = user.FillUserById()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 写入 OIDC 标识，完成绑定。
	user.OidcId = oidcUser.OpenID
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
	return
}

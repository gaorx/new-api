package controller

import (
	"encoding/base64"
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

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// LinuxdoUser 表示从 Linux DO 用户信息接口读取到的用户资料。
type LinuxdoUser struct {
	Id         int    `json:"id"`          // Linux DO 用户 ID。
	Username   string `json:"username"`    // Linux DO 用户名。
	Name       string `json:"name"`        // Linux DO 显示名称。
	Active     bool   `json:"active"`      // 用户是否激活。
	TrustLevel int    `json:"trust_level"` // 用户信任等级。
	Silenced   bool   `json:"silenced"`    // 用户是否被禁言。
}

// LinuxDoBind 将当前登录用户与 Linux DO 账户绑定。
// 参数：
//   - c：当前请求上下文，用于读取 OAuth code 并完成绑定。
func LinuxDoBind(c *gin.Context) {
	// 未启用 Linux DO OAuth 时，不允许进行账户绑定。
	if !common.LinuxDOOAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Linux DO 登录以及注册",
		})
		return
	}

	// 使用授权码换取 Linux DO 用户资料。
	code := c.Query("code")
	linuxdoUser, err := getLinuxdoUserInfoByCode(code, c)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	user := model.User{
		LinuxDOId: strconv.Itoa(linuxdoUser.Id),
	}

	// 已被其他账号绑定的 Linux DO 账户不可重复绑定。
	if model.IsLinuxDOIdAlreadyTaken(user.LinuxDOId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该 Linux DO 账户已被绑定",
		})
		return
	}

	// 从 session 中取出当前登录用户 ID，并加载用户记录。
	session := sessions.Default(c)
	id := session.Get("id")
	user.Id = id.(int)

	err = user.FillUserById()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 把 Linux DO 账户 ID 写入当前用户，完成绑定。
	user.LinuxDOId = strconv.Itoa(linuxdoUser.Id)
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

// getLinuxdoUserInfoByCode 使用 Linux DO OAuth 授权码换取用户信息。
// 参数：
//   - code：Linux DO OAuth 回调中的授权码。
//   - c：当前请求上下文，用于推导当前站点回调地址。
//
// 返回：
//   - *LinuxdoUser：成功获取到的 Linux DO 用户资料。
//   - error：授权码无效、网络失败或解析失败时返回错误。
func getLinuxdoUserInfoByCode(code string, c *gin.Context) (*LinuxdoUser, error) {
	// 授权码为空时直接拒绝。
	if code == "" {
		return nil, errors.New("invalid code")
	}

	// Get access token using Basic auth
	// 先构造 token 交换请求所需的 Basic 鉴权头和回调地址。
	tokenEndpoint := common.GetEnvOrDefaultString("LINUX_DO_TOKEN_ENDPOINT", "https://connect.linux.do/oauth2/token")
	credentials := common.LinuxDOClientId + ":" + common.LinuxDOClientSecret
	basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))

	// Get redirect URI from request
	// 根据当前请求自动推导回调地址，兼容 http/https 环境。
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	redirectURI := fmt.Sprintf("%s://%s/api/oauth/linuxdo", scheme, c.Request.Host)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", basicAuth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := http.Client{Timeout: 5 * time.Second}
	// 用授权码换取 access token。
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("failed to connect to Linux DO server")
	}
	defer res.Body.Close()

	var tokenRes struct {
		AccessToken string `json:"access_token"`
		Message     string `json:"message"`
	}
	if err := json.NewDecoder(res.Body).Decode(&tokenRes); err != nil {
		return nil, err
	}

	if tokenRes.AccessToken == "" {
		return nil, fmt.Errorf("failed to get access token: %s", tokenRes.Message)
	}

	// Get user info
	// 再用 access token 调用 Linux DO 用户资料接口。
	userEndpoint := common.GetEnvOrDefaultString("LINUX_DO_USER_ENDPOINT", "https://connect.linux.do/api/user")
	req, err = http.NewRequest("GET", userEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tokenRes.AccessToken)
	req.Header.Set("Accept", "application/json")

	res2, err := client.Do(req)
	if err != nil {
		return nil, errors.New("failed to get user info from Linux DO")
	}
	defer res2.Body.Close()

	var linuxdoUser LinuxdoUser
	if err := json.NewDecoder(res2.Body).Decode(&linuxdoUser); err != nil {
		return nil, err
	}

	if linuxdoUser.Id == 0 {
		return nil, errors.New("invalid user info returned")
	}

	// 返回解析成功的 Linux DO 用户信息。
	return &linuxdoUser, nil
}

// LinuxdoOAuth 处理 Linux DO OAuth 登录或注册回调。
// 参数：
//   - c：当前请求上下文，用于读取回调参数、session 并完成登录。
func LinuxdoOAuth(c *gin.Context) {
	// 先读取 session，并处理上游显式返回的 OAuth 错误。
	session := sessions.Default(c)

	errorCode := c.Query("error")
	if errorCode != "" {
		errorDescription := c.Query("error_description")
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": errorDescription,
		})
		return
	}

	// 校验 state，防止非法回调或 CSRF。
	state := c.Query("state")
	if state == "" || session.Get("oauth_state") == nil || state != session.Get("oauth_state").(string) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "state is empty or not same",
		})
		return
	}

	// 若当前 session 已登录，则本次回调走账号绑定逻辑。
	username := session.Get("username")
	if username != nil {
		LinuxDoBind(c)
		return
	}

	// 管理员未开启 Linux DO OAuth 时，不允许登录或注册。
	if !common.LinuxDOOAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 Linux DO 登录以及注册",
		})
		return
	}

	// 通过授权码换取 Linux DO 用户资料。
	code := c.Query("code")
	linuxdoUser, err := getLinuxdoUserInfoByCode(code, c)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	user := model.User{
		LinuxDOId: strconv.Itoa(linuxdoUser.Id),
	}

	// Check if user exists
	// 若该 Linux DO 账户已经存在，则读取对应用户；否则尝试注册新用户。
	if model.IsLinuxDOIdAlreadyTaken(user.LinuxDOId) {
		err := user.FillUserByLinuxDOId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
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
	} else {
		// 仅当允许注册且信任等级满足要求时，才创建新用户。
		if common.RegisterEnabled {
			if linuxdoUser.TrustLevel >= common.LinuxDOMinimumTrustLevel {
				user.Username = "linuxdo_" + strconv.Itoa(model.GetMaxUserId()+1)
				user.DisplayName = linuxdoUser.Name
				user.Role = common.RoleCommonUser
				user.Status = common.UserStatusEnabled

				affCode := session.Get("aff")
				inviterId := 0
				if affCode != nil {
					inviterId, _ = model.GetUserIdByAffCode(affCode.(string))
				}

				if err := user.Insert(inviterId); err != nil {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": err.Error(),
					})
					return
				}
			} else {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Linux DO 信任等级未达到管理员设置的最低信任等级",
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

	// 被禁用用户不能通过 Linux DO 完成登录。
	if user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "用户已被封禁",
			"success": false,
		})
		return
	}

	// 进入统一登录收尾逻辑，写入 session 和登录态。
	setupLogin(&user, c)
}

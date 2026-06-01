package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// GitHubOAuthResponse 表示 GitHub OAuth token 交换接口的响应结构。
type GitHubOAuthResponse struct {
	AccessToken string `json:"access_token"` // GitHub 返回的访问令牌。
	Scope       string `json:"scope"`        // 授权 scopes。
	TokenType   string `json:"token_type"`   // 令牌类型。
}

// GitHubUser 表示从 GitHub 用户信息接口读取到的用户资料。
type GitHubUser struct {
	Login string `json:"login"` // GitHub 用户登录名。
	Name  string `json:"name"`  // GitHub 展示名称。
	Email string `json:"email"` // GitHub 邮箱。
}

// getGitHubUserInfoByCode 使用 OAuth 授权码换取 GitHub 用户信息。
// 参数：
//   - code：GitHub OAuth 回调中的授权码。
//
// 返回：
//   - *GitHubUser：成功获取到的 GitHub 用户信息。
//   - error：授权码无效、网络失败或解析失败时返回错误。
func getGitHubUserInfoByCode(code string) (*GitHubUser, error) {
	// 授权码为空时无需继续请求 GitHub。
	if code == "" {
		return nil, errors.New("无效的参数")
	}
	// 先用授权码向 GitHub 换取 access token。
	values := map[string]string{"client_id": common.GitHubClientId, "client_secret": common.GitHubClientSecret, "code": code}
	jsonData, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := http.Client{
		Timeout: 20 * time.Second,
	}
	// 调用 GitHub token 接口并解析访问令牌。
	res, err := client.Do(req)
	if err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 GitHub 服务器，请稍后重试！")
	}
	defer res.Body.Close()
	var oAuthResponse GitHubOAuthResponse
	err = json.NewDecoder(res.Body).Decode(&oAuthResponse)
	if err != nil {
		return nil, err
	}
	// 拿到 access token 后再请求 GitHub 用户资料接口。
	req, err = http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", oAuthResponse.AccessToken))
	res2, err := client.Do(req)
	if err != nil {
		common.SysLog(err.Error())
		return nil, errors.New("无法连接至 GitHub 服务器，请稍后重试！")
	}
	defer res2.Body.Close()
	var githubUser GitHubUser
	err = json.NewDecoder(res2.Body).Decode(&githubUser)
	if err != nil {
		return nil, err
	}
	// 登录名为空时视为 GitHub 返回了不完整的用户信息。
	if githubUser.Login == "" {
		return nil, errors.New("返回值非法，用户字段为空，请稍后重试！")
	}
	return &githubUser, nil
}

// GitHubOAuth 处理 GitHub OAuth 登录或注册回调。
// 参数：
//   - c：当前请求上下文，用于读取回调参数、session 并完成登录流程。
func GitHubOAuth(c *gin.Context) {
	// 先校验 session 中的 state，防止 CSRF 或非法回调。
	session := sessions.Default(c)
	state := c.Query("state")
	if state == "" || session.Get("oauth_state") == nil || state != session.Get("oauth_state").(string) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "state is empty or not same",
		})
		return
	}
	// 若当前 session 已绑定用户名，则本次回调走绑定流程而非登录流程。
	username := session.Get("username")
	if username != nil {
		GitHubBind(c)
		return
	}

	// 管理员未开启 GitHub OAuth 时，直接拒绝本次登录。
	if !common.GitHubOAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 GitHub 登录以及注册",
		})
		return
	}
	// 使用 code 拉取 GitHub 用户资料。
	code := c.Query("code")
	githubUser, err := getGitHubUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user := model.User{
		GitHubId: githubUser.Login,
	}
	// IsGitHubIdAlreadyTaken is unscoped
	// 若 GitHub 账户已存在，则读取对应用户；否则按注册逻辑创建新用户。
	if model.IsGitHubIdAlreadyTaken(user.GitHubId) {
		// FillUserByGitHubId is scoped
		err := user.FillUserByGitHubId()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		// if user.Id == 0 , user has been deleted
		if user.Id == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "用户已注销",
			})
			return
		}
	} else {
		// 当允许注册时，根据 GitHub 用户信息创建新的普通用户。
		if common.RegisterEnabled {
			user.Username = "github_" + strconv.Itoa(model.GetMaxUserId()+1)
			if githubUser.Name != "" {
				user.DisplayName = githubUser.Name
			} else {
				user.DisplayName = "GitHub User"
			}
			user.Email = githubUser.Email
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
				"message": "管理员关闭了新用户注册",
			})
			return
		}
	}

	// 被禁用的用户不能通过 GitHub 完成登录。
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

// GitHubBind 将当前登录用户与 GitHub 账户绑定。
// 参数：
//   - c：当前请求上下文，用于读取回调 code 和当前登录用户 session。
func GitHubBind(c *gin.Context) {
	// 绑定前同样要求 GitHub OAuth 已启用。
	if !common.GitHubOAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "管理员未开启通过 GitHub 登录以及注册",
		})
		return
	}
	// 先通过 code 拉取待绑定的 GitHub 账户信息。
	code := c.Query("code")
	githubUser, err := getGitHubUserInfoByCode(code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user := model.User{
		GitHubId: githubUser.Login,
	}
	// 已被其他用户绑定的 GitHub 账户不可重复绑定。
	if model.IsGitHubIdAlreadyTaken(user.GitHubId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该 GitHub 账户已被绑定",
		})
		return
	}
	// 从 session 中获取当前登录用户 ID，并回填用户记录。
	session := sessions.Default(c)
	id := session.Get("id")
	// id := c.GetInt("id")  // critical bug!
	user.Id = id.(int)
	err = user.FillUserById()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 把 GitHubId 写回当前用户，完成账户绑定。
	user.GitHubId = githubUser.Login
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

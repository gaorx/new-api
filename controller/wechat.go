package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// wechatLoginResponse 表示微信登录中转服务的响应结构。
type wechatLoginResponse struct {
	Success bool   `json:"success"` // 请求是否成功。
	Message string `json:"message"` // 失败时的错误消息。
	Data    string `json:"data"`    // 成功时返回的微信用户标识。
}

// getWeChatIdByCode 使用微信授权码向中转服务换取微信用户 ID。
// 参数：
//   - code：微信登录回调中的授权码。
//
// 返回：
//   - string：成功获取到的微信用户 ID。
//   - error：授权码无效、网络失败或中转服务返回错误时返回错误。
func getWeChatIdByCode(code string) (string, error) {
	// code 为空时无需继续请求中转服务。
	if code == "" {
		return "", errors.New("无效的参数")
	}
	// 构造请求并附带服务端令牌，调用微信登录中转服务。
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/wechat/user?code=%s", common.WeChatServerAddress, url.QueryEscape(code)), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", common.WeChatServerToken)
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	// 请求中转服务并解析响应。
	httpResponse, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer httpResponse.Body.Close()
	var res wechatLoginResponse
	err = json.NewDecoder(httpResponse.Body).Decode(&res)
	if err != nil {
		return "", err
	}
	if !res.Success {
		return "", errors.New(res.Message)
	}
	if res.Data == "" {
		return "", errors.New("验证码错误或已过期")
	}
	// 返回有效的微信用户标识。
	return res.Data, nil
}

// WeChatAuth 处理微信登录或自动注册流程。
// 参数：
//   - c：当前请求上下文，用于读取 code 并完成登录。
func WeChatAuth(c *gin.Context) {
	// 未启用微信登录时，直接拒绝请求。
	if !common.WeChatAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员未开启通过微信登录以及注册",
			"success": false,
		})
		return
	}
	// 用授权码换取微信用户 ID。
	code := c.Query("code")
	wechatId, err := getWeChatIdByCode(code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	user := model.User{
		WeChatId: wechatId,
	}
	// 若微信账号已存在，则读取对应用户；否则在允许注册时创建新用户。
	if model.IsWeChatIdAlreadyTaken(wechatId) {
		err := user.FillUserByWeChatId()
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
		// 按微信登录资料创建默认普通用户。
		if common.RegisterEnabled {
			user.Username = "wechat_" + strconv.Itoa(model.GetMaxUserId()+1)
			user.DisplayName = "WeChat User"
			user.Role = common.RoleCommonUser
			user.Status = common.UserStatusEnabled

			if err := user.Insert(0); err != nil {
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

	// 被禁用用户不能通过微信登录。
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

// wechatBindRequest 表示微信绑定接口的请求体。
type wechatBindRequest struct {
	Code string `json:"code"` // 前端提交的微信授权码。
}

// WeChatBind 将当前登录用户与微信账号绑定。
// 参数：
//   - c：当前请求上下文，用于读取绑定请求体和 session。
func WeChatBind(c *gin.Context) {
	// 未启用微信登录时，不允许绑定微信账号。
	if !common.WeChatAuthEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "管理员未开启通过微信登录以及注册",
			"success": false,
		})
		return
	}
	// 解析绑定请求体。
	var req wechatBindRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的请求",
		})
		return
	}
	// 用 code 换取待绑定的微信用户 ID。
	code := req.Code
	wechatId, err := getWeChatIdByCode(code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	// 已被其他账号绑定的微信号不可重复绑定。
	if model.IsWeChatIdAlreadyTaken(wechatId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该微信账号已被绑定",
		})
		return
	}
	// 从 session 读取当前登录用户并加载用户记录。
	session := sessions.Default(c)
	id := session.Get("id")
	user := model.User{
		Id: id.(int),
	}
	err = user.FillUserById()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 回写 WeChatId，完成账号绑定。
	user.WeChatId = wechatId
	err = user.Update(false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回绑定成功结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

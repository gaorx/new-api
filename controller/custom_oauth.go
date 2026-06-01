package controller

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-gonic/gin"
)

// CustomOAuthProviderResponse is the response structure for custom OAuth providers
// It excludes sensitive fields like client_secret
type CustomOAuthProviderResponse struct {
	Id                    int    `json:"id"`                     // 提供商主键 ID。
	Name                  string `json:"name"`                   // 提供商显示名称。
	Slug                  string `json:"slug"`                   // 提供商唯一 slug。
	Icon                  string `json:"icon"`                   // 前端展示图标。
	Enabled               bool   `json:"enabled"`                // 当前提供商是否启用。
	ClientId              string `json:"client_id"`              // OAuth Client ID。
	AuthorizationEndpoint string `json:"authorization_endpoint"` // 授权端点地址。
	TokenEndpoint         string `json:"token_endpoint"`         // Token 交换端点地址。
	UserInfoEndpoint      string `json:"user_info_endpoint"`     // 用户信息端点地址。
	Scopes                string `json:"scopes"`                 // 请求的 scopes 配置。
	UserIdField           string `json:"user_id_field"`          // 用户 ID 字段路径。
	UsernameField         string `json:"username_field"`         // 用户名字段路径。
	DisplayNameField      string `json:"display_name_field"`     // 显示名字段路径。
	EmailField            string `json:"email_field"`            // 邮箱字段路径。
	WellKnown             string `json:"well_known"`             // OIDC discovery 地址。
	AuthStyle             int    `json:"auth_style"`             // 认证风格配置。
	AccessPolicy          string `json:"access_policy"`          // 访问策略配置。
	AccessDeniedMessage   string `json:"access_denied_message"`  // 访问被拒绝时的提示文案。
}

// UserOAuthBindingResponse 表示用户与自定义 OAuth 提供商的绑定信息。
type UserOAuthBindingResponse struct {
	ProviderId     int    `json:"provider_id"`      // 提供商 ID。
	ProviderName   string `json:"provider_name"`    // 提供商名称。
	ProviderSlug   string `json:"provider_slug"`    // 提供商 slug。
	ProviderIcon   string `json:"provider_icon"`    // 提供商图标。
	ProviderUserId string `json:"provider_user_id"` // 上游提供商中的用户 ID。
}

// toCustomOAuthProviderResponse 将数据库模型转换为对外响应结构，并去掉敏感字段。
// 参数：
//   - p：数据库中的自定义 OAuth 提供商模型。
//
// 返回：
//   - *CustomOAuthProviderResponse：可安全返回给前端的响应对象。
func toCustomOAuthProviderResponse(p *model.CustomOAuthProvider) *CustomOAuthProviderResponse {
	// 逐字段复制允许暴露的信息，显式排除 client_secret 等敏感数据。
	return &CustomOAuthProviderResponse{
		Id:                    p.Id,
		Name:                  p.Name,
		Slug:                  p.Slug,
		Icon:                  p.Icon,
		Enabled:               p.Enabled,
		ClientId:              p.ClientId,
		AuthorizationEndpoint: p.AuthorizationEndpoint,
		TokenEndpoint:         p.TokenEndpoint,
		UserInfoEndpoint:      p.UserInfoEndpoint,
		Scopes:                p.Scopes,
		UserIdField:           p.UserIdField,
		UsernameField:         p.UsernameField,
		DisplayNameField:      p.DisplayNameField,
		EmailField:            p.EmailField,
		WellKnown:             p.WellKnown,
		AuthStyle:             p.AuthStyle,
		AccessPolicy:          p.AccessPolicy,
		AccessDeniedMessage:   p.AccessDeniedMessage,
	}
}

// GetCustomOAuthProviders returns all custom OAuth providers
// 参数：
//   - c：当前请求上下文，用于返回全部自定义 OAuth 提供商列表。
func GetCustomOAuthProviders(c *gin.Context) {
	// 先从数据库读取全部自定义 OAuth 提供商。
	providers, err := model.GetAllCustomOAuthProviders()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 把模型对象转换成安全响应结构，避免把敏感字段暴露给前端。
	response := make([]*CustomOAuthProviderResponse, len(providers))
	for i, p := range providers {
		response[i] = toCustomOAuthProviderResponse(p)
	}

	// 返回全部提供商列表。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// GetCustomOAuthProvider returns a single custom OAuth provider by ID
// 参数：
//   - c：当前请求上下文，用于读取 provider ID 并返回详情。
func GetCustomOAuthProvider(c *gin.Context) {
	// 先解析路径中的提供商 ID。
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiErrorMsg(c, "无效的 ID")
		return
	}

	// 按 ID 读取单个提供商；不存在时返回业务错误。
	provider, err := model.GetCustomOAuthProviderById(id)
	if err != nil {
		common.ApiErrorMsg(c, "未找到该 OAuth 提供商")
		return
	}

	// 返回单个提供商的脱敏详情。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    toCustomOAuthProviderResponse(provider),
	})
}

// CreateCustomOAuthProviderRequest is the request structure for creating a custom OAuth provider
type CreateCustomOAuthProviderRequest struct {
	Name                  string `json:"name" binding:"required"`                   // 提供商显示名称。
	Slug                  string `json:"slug" binding:"required"`                   // 提供商唯一 slug。
	Icon                  string `json:"icon"`                                      // 图标地址或标识。
	Enabled               bool   `json:"enabled"`                                   // 是否启用。
	ClientId              string `json:"client_id" binding:"required"`              // OAuth Client ID。
	ClientSecret          string `json:"client_secret" binding:"required"`          // OAuth Client Secret。
	AuthorizationEndpoint string `json:"authorization_endpoint" binding:"required"` // 授权端点。
	TokenEndpoint         string `json:"token_endpoint" binding:"required"`         // Token 端点。
	UserInfoEndpoint      string `json:"user_info_endpoint" binding:"required"`     // 用户信息端点。
	Scopes                string `json:"scopes"`                                    // scopes 配置。
	UserIdField           string `json:"user_id_field"`                             // 用户 ID 字段路径。
	UsernameField         string `json:"username_field"`                            // 用户名字段路径。
	DisplayNameField      string `json:"display_name_field"`                        // 显示名字段路径。
	EmailField            string `json:"email_field"`                               // 邮箱字段路径。
	WellKnown             string `json:"well_known"`                                // OIDC discovery 地址。
	AuthStyle             int    `json:"auth_style"`                                // 认证风格。
	AccessPolicy          string `json:"access_policy"`                             // 访问策略。
	AccessDeniedMessage   string `json:"access_denied_message"`                     // 访问拒绝提示。
}

// FetchCustomOAuthDiscoveryRequest 表示后端代理获取 OIDC discovery 文档的请求体。
type FetchCustomOAuthDiscoveryRequest struct {
	WellKnownURL string `json:"well_known_url"` // 明确指定的 discovery URL。
	IssuerURL    string `json:"issuer_url"`     // issuer 基础地址，会自动拼接 .well-known 路径。
}

// FetchCustomOAuthDiscovery fetches OIDC discovery document via backend (root-only route)
// 参数：
//   - c：当前请求上下文，用于读取 discovery 地址并返回 discovery 文档。
func FetchCustomOAuthDiscovery(c *gin.Context) {
	// 解析请求体，支持直接传 well-known URL 或仅传 issuer URL。
	var req FetchCustomOAuthDiscoveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}

	// 规范化输入参数，并根据 issuer URL 推导标准 discovery 地址。
	wellKnownURL := strings.TrimSpace(req.WellKnownURL)
	issuerURL := strings.TrimSpace(req.IssuerURL)

	if wellKnownURL == "" && issuerURL == "" {
		common.ApiErrorMsg(c, "请先填写 Discovery URL 或 Issuer URL")
		return
	}

	targetURL := wellKnownURL
	if targetURL == "" {
		targetURL = strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
	}
	targetURL = strings.TrimSpace(targetURL)

	// 做基础 URL 校验，只允许 http/https。
	parsedURL, err := url.Parse(targetURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		common.ApiErrorMsg(c, "Discovery URL 无效，仅支持 http/https")
		return
	}

	// 以带超时的后端请求方式拉取 discovery 文档，避免前端直连的跨域和安全问题。
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		common.ApiErrorMsg(c, "创建 Discovery 请求失败: "+err.Error())
		return
	}
	httpReq.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		common.ApiErrorMsg(c, "获取 Discovery 配置失败: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		common.ApiErrorMsg(c, "获取 Discovery 配置失败: "+message)
		return
	}

	var discovery map[string]any
	if err = common.DecodeJson(resp.Body, &discovery); err != nil {
		common.ApiErrorMsg(c, "解析 Discovery 配置失败: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"well_known_url": targetURL,
			"discovery":      discovery,
		},
	})
}

// CreateCustomOAuthProvider 创建新的自定义 OAuth 提供商。
// 参数：
//   - c：当前请求上下文，用于读取创建参数并返回创建结果。
func CreateCustomOAuthProvider(c *gin.Context) {
	// 解析创建请求参数，确保必填项完整。
	var req CreateCustomOAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}

	// 校验 slug 是否已被其他自定义提供商占用。
	if model.IsSlugTaken(req.Slug, 0) {
		common.ApiErrorMsg(c, "该 Slug 已被使用")
		return
	}

	// 校验 slug 是否与系统内置 OAuth 提供商冲突。
	if oauth.IsProviderRegistered(req.Slug) && !oauth.IsCustomProvider(req.Slug) {
		common.ApiErrorMsg(c, "该 Slug 与内置 OAuth 提供商冲突")
		return
	}

	// 根据请求体组装数据库模型，准备写入新提供商记录。
	provider := &model.CustomOAuthProvider{
		Name:                  req.Name,
		Slug:                  req.Slug,
		Icon:                  req.Icon,
		Enabled:               req.Enabled,
		ClientId:              req.ClientId,
		ClientSecret:          req.ClientSecret,
		AuthorizationEndpoint: req.AuthorizationEndpoint,
		TokenEndpoint:         req.TokenEndpoint,
		UserInfoEndpoint:      req.UserInfoEndpoint,
		Scopes:                req.Scopes,
		UserIdField:           req.UserIdField,
		UsernameField:         req.UsernameField,
		DisplayNameField:      req.DisplayNameField,
		EmailField:            req.EmailField,
		WellKnown:             req.WellKnown,
		AuthStyle:             req.AuthStyle,
		AccessPolicy:          req.AccessPolicy,
		AccessDeniedMessage:   req.AccessDeniedMessage,
	}

	// 将新提供商持久化到数据库。
	if err := model.CreateCustomOAuthProvider(provider); err != nil {
		common.ApiError(c, err)
		return
	}

	// 把新提供商注册到运行时 OAuth 注册表，使其立即可被授权流程识别。
	oauth.RegisterOrUpdateCustomProvider(provider)

	// 返回创建后的脱敏提供商信息。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "创建成功",
		"data":    toCustomOAuthProviderResponse(provider),
	})
}

// UpdateCustomOAuthProviderRequest 表示更新自定义 OAuth 提供商的请求体。
type UpdateCustomOAuthProviderRequest struct {
	Name                  string  `json:"name"`                  // 提供商名称；非空时覆盖原值。
	Slug                  string  `json:"slug"`                  // 提供商 slug；非空时覆盖原值。
	Icon                  *string `json:"icon"`                  // 可选图标；为 nil 时保留原值。
	Enabled               *bool   `json:"enabled"`               // 可选启用状态；为 nil 时保留原值。
	ClientId              string  `json:"client_id"`             // OAuth Client ID；非空时覆盖原值。
	ClientSecret          string  `json:"client_secret"`         // OAuth Client Secret；为空时保留原值。
	AuthorizationEndpoint string  `json:"authorization_endpoint"` // 授权端点；非空时覆盖原值。
	TokenEndpoint         string  `json:"token_endpoint"`        // Token 端点；非空时覆盖原值。
	UserInfoEndpoint      string  `json:"user_info_endpoint"`    // 用户信息端点；非空时覆盖原值。
	Scopes                string  `json:"scopes"`                // scopes 配置；非空时覆盖原值。
	UserIdField           string  `json:"user_id_field"`         // 用户 ID 字段路径；非空时覆盖原值。
	UsernameField         string  `json:"username_field"`        // 用户名字段路径；非空时覆盖原值。
	DisplayNameField      string  `json:"display_name_field"`    // 显示名字段路径；非空时覆盖原值。
	EmailField            string  `json:"email_field"`           // 邮箱字段路径；非空时覆盖原值。
	WellKnown             *string `json:"well_known"`            // 可选 discovery 地址；为 nil 时保留原值。
	AuthStyle             *int    `json:"auth_style"`            // 可选认证风格；为 nil 时保留原值。
	AccessPolicy          *string `json:"access_policy"`         // 可选访问策略；为 nil 时保留原值。
	AccessDeniedMessage   *string `json:"access_denied_message"` // 可选访问拒绝提示；为 nil 时保留原值。
}

// UpdateCustomOAuthProvider 更新指定的自定义 OAuth 提供商。
// 参数：
//   - c：当前请求上下文，用于读取 provider ID、更新参数并返回更新结果。
func UpdateCustomOAuthProvider(c *gin.Context) {
	// 先解析路径中的提供商 ID。
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiErrorMsg(c, "无效的 ID")
		return
	}

	// 绑定更新请求体，收集可选变更字段。
	var req UpdateCustomOAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "无效的请求参数: "+err.Error())
		return
	}

	// 读取现有提供商记录，后续在其基础上做增量更新。
	provider, err := model.GetCustomOAuthProviderById(id)
	if err != nil {
		common.ApiErrorMsg(c, "未找到该 OAuth 提供商")
		return
	}

	oldSlug := provider.Slug

	// 当 slug 被修改时，校验它没有被其他自定义提供商占用，也不与内置提供商冲突。
	if req.Slug != "" && req.Slug != provider.Slug {
		if model.IsSlugTaken(req.Slug, id) {
			common.ApiErrorMsg(c, "该 Slug 已被使用")
			return
		}
		// 同时避免与系统内置 OAuth 提供商的标识发生冲突。
		if oauth.IsProviderRegistered(req.Slug) && !oauth.IsCustomProvider(req.Slug) {
			common.ApiErrorMsg(c, "该 Slug 与内置 OAuth 提供商冲突")
			return
		}
	}

	// 按“非空或非 nil 即覆盖”的规则，把请求中的变更应用到现有模型。
	if req.Name != "" {
		provider.Name = req.Name
	}
	if req.Slug != "" {
		provider.Slug = req.Slug
	}
	if req.Icon != nil {
		provider.Icon = *req.Icon
	}
	if req.Enabled != nil {
		provider.Enabled = *req.Enabled
	}
	if req.ClientId != "" {
		provider.ClientId = req.ClientId
	}
	if req.ClientSecret != "" {
		provider.ClientSecret = req.ClientSecret
	}
	if req.AuthorizationEndpoint != "" {
		provider.AuthorizationEndpoint = req.AuthorizationEndpoint
	}
	if req.TokenEndpoint != "" {
		provider.TokenEndpoint = req.TokenEndpoint
	}
	if req.UserInfoEndpoint != "" {
		provider.UserInfoEndpoint = req.UserInfoEndpoint
	}
	if req.Scopes != "" {
		provider.Scopes = req.Scopes
	}
	if req.UserIdField != "" {
		provider.UserIdField = req.UserIdField
	}
	if req.UsernameField != "" {
		provider.UsernameField = req.UsernameField
	}
	if req.DisplayNameField != "" {
		provider.DisplayNameField = req.DisplayNameField
	}
	if req.EmailField != "" {
		provider.EmailField = req.EmailField
	}
	if req.WellKnown != nil {
		provider.WellKnown = *req.WellKnown
	}
	if req.AuthStyle != nil {
		provider.AuthStyle = *req.AuthStyle
	}
	if req.AccessPolicy != nil {
		provider.AccessPolicy = *req.AccessPolicy
	}
	if req.AccessDeniedMessage != nil {
		provider.AccessDeniedMessage = *req.AccessDeniedMessage
	}

	// 将更新后的提供商配置写回数据库。
	if err := model.UpdateCustomOAuthProvider(provider); err != nil {
		common.ApiError(c, err)
		return
	}

	// 同步更新运行时 OAuth 注册表；若 slug 发生变化，还需要移除旧注册项。
	if oldSlug != provider.Slug {
		oauth.UnregisterCustomProvider(oldSlug)
	}
	oauth.RegisterOrUpdateCustomProvider(provider)

	// 返回更新后的脱敏提供商信息。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "更新成功",
		"data":    toCustomOAuthProviderResponse(provider),
	})
}

// DeleteCustomOAuthProvider 删除指定的自定义 OAuth 提供商。
// 参数：
//   - c：当前请求上下文，用于读取 provider ID 并执行删除。
func DeleteCustomOAuthProvider(c *gin.Context) {
	// 先解析路径中的提供商 ID。
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiErrorMsg(c, "无效的 ID")
		return
	}

	// 先读取现有提供商，删除后还需要用它的 slug 清理运行时注册表。
	provider, err := model.GetCustomOAuthProviderById(id)
	if err != nil {
		common.ApiErrorMsg(c, "未找到该 OAuth 提供商")
		return
	}

	// 删除前先确认没有任何用户仍然绑定该提供商，避免留下失效外键关系。
	count, err := model.GetBindingCountByProviderId(id)
	if err != nil {
		common.SysError("Failed to get binding count for provider " + strconv.Itoa(id) + ": " + err.Error())
		common.ApiErrorMsg(c, "检查用户绑定时发生错误，请稍后重试")
		return
	}
	if count > 0 {
		common.ApiErrorMsg(c, "该 OAuth 提供商还有用户绑定，无法删除。请先解除所有用户绑定。")
		return
	}

	// 真正删除数据库中的提供商记录。
	if err := model.DeleteCustomOAuthProvider(id); err != nil {
		common.ApiError(c, err)
		return
	}

	// 同步从运行时 OAuth 注册表中注销该自定义提供商。
	oauth.UnregisterCustomProvider(provider.Slug)

	// 返回删除成功结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}

// buildUserOAuthBindingsResponse 构造指定用户的 OAuth 绑定响应列表。
// 参数：
//   - userId：目标用户 ID。
//
// 返回：
//   - []UserOAuthBindingResponse：可直接返回前端的绑定信息列表。
//   - error：查询绑定或提供商信息时发生的错误。
func buildUserOAuthBindingsResponse(userId int) ([]UserOAuthBindingResponse, error) {
	// 先取出用户在数据库中的全部 OAuth 绑定记录。
	bindings, err := model.GetUserOAuthBindingsByUserId(userId)
	if err != nil {
		return nil, err
	}

	// 逐条补齐提供商名称、图标等展示信息，生成前端友好的响应结构。
	response := make([]UserOAuthBindingResponse, 0, len(bindings))
	for _, binding := range bindings {
		provider, err := model.GetCustomOAuthProviderById(binding.ProviderId)
		if err != nil {
			continue
		}
		response = append(response, UserOAuthBindingResponse{
			ProviderId:     binding.ProviderId,
			ProviderName:   provider.Name,
			ProviderSlug:   provider.Slug,
			ProviderIcon:   provider.Icon,
			ProviderUserId: binding.ProviderUserId,
		})
	}

	return response, nil
}

// GetUserOAuthBindings 返回当前登录用户的全部 OAuth 绑定信息。
// 参数：
//   - c：当前请求上下文，用于读取当前登录用户并返回绑定列表。
func GetUserOAuthBindings(c *gin.Context) {
	// 从上下文中获取当前登录用户 ID；未登录则直接返回。
	userId := c.GetInt("id")
	if userId == 0 {
		common.ApiErrorMsg(c, "未登录")
		return
	}

	// 构造当前用户的绑定响应列表。
	response, err := buildUserOAuthBindingsResponse(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回当前用户的绑定列表。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// GetUserOAuthBindingsByAdmin 由管理员查询指定用户的 OAuth 绑定信息。
// 参数：
//   - c：当前请求上下文，用于读取目标用户 ID 并校验管理权限。
func GetUserOAuthBindingsByAdmin(c *gin.Context) {
	// 解析路径中的目标用户 ID。
	userIdStr := c.Param("id")
	userId, err := strconv.Atoi(userIdStr)
	if err != nil {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}

	// 读取目标用户并进行角色级别权限校验。
	targetUser, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	myRole := c.GetInt("role")
	if !canManageTargetRole(myRole, targetUser.Role) {
		common.ApiErrorMsg(c, "no permission")
		return
	}

	// 构造目标用户的绑定响应列表。
	response, err := buildUserOAuthBindingsResponse(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回目标用户的绑定信息。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// UnbindCustomOAuth 解除当前登录用户与指定自定义 OAuth 提供商的绑定。
// 参数：
//   - c：当前请求上下文，用于读取当前用户和目标提供商 ID。
func UnbindCustomOAuth(c *gin.Context) {
	// 从上下文中读取当前登录用户；未登录时不可解绑。
	userId := c.GetInt("id")
	if userId == 0 {
		common.ApiErrorMsg(c, "未登录")
		return
	}

	// 解析路径中的提供商 ID。
	providerIdStr := c.Param("provider_id")
	providerId, err := strconv.Atoi(providerIdStr)
	if err != nil {
		common.ApiErrorMsg(c, "无效的提供商 ID")
		return
	}

	// 删除当前用户与该提供商之间的绑定关系。
	if err := model.DeleteUserOAuthBinding(userId, providerId); err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回解绑成功结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "解绑成功",
	})
}

// UnbindCustomOAuthByAdmin 由管理员解除指定用户与自定义 OAuth 提供商的绑定。
// 参数：
//   - c：当前请求上下文，用于读取目标用户 ID、提供商 ID 并校验权限。
func UnbindCustomOAuthByAdmin(c *gin.Context) {
	// 先解析目标用户 ID。
	userIdStr := c.Param("id")
	userId, err := strconv.Atoi(userIdStr)
	if err != nil {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}

	// 读取目标用户并检查当前操作者是否有权限管理该用户。
	targetUser, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	myRole := c.GetInt("role")
	if !canManageTargetRole(myRole, targetUser.Role) {
		common.ApiErrorMsg(c, "no permission")
		return
	}

	// 再解析待解绑的提供商 ID。
	providerIdStr := c.Param("provider_id")
	providerId, err := strconv.Atoi(providerIdStr)
	if err != nil {
		common.ApiErrorMsg(c, "invalid provider id")
		return
	}

	// 删除目标用户与指定提供商之间的绑定关系。
	if err := model.DeleteUserOAuthBinding(userId, providerId); err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回管理员解绑成功结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "success",
	})
}

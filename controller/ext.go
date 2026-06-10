package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type ratiosRequest struct {
	Model string `json:"model"`
}

type ratiosChannelInfo struct {
	Id       int    `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Group    string `json:"group"`
	Models   string `json:"models"`
	Priority int64  `json:"priority"`
	Weight   int    `json:"weight"`
}

func GetRequestRatios(c *gin.Context) {
	var req ratiosRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid request body",
		})
		return
	}
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	tokenID := c.GetInt("token_id")
	if tokenID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "invalid token context",
		})
		return
	}

	token, err := model.GetTokenById(tokenID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userCache, err := model.GetUserCache(token.UserId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	userCache.WriteContext(c)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, token.Group)
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, token.CrossGroupRetry)

	userGroup := userCache.Group
	tokenGroup := token.Group
	usingGroup := userGroup
	autoGroups := []string{}
	if tokenGroup == "auto" {
		autoGroups = service.GetUserAutoGroup(userGroup)
		usingGroup = tokenGroup
	} else if tokenGroup != "" {
		if _, ok := service.GetUserUsableGroups(userGroup)[tokenGroup]; !ok {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "token group is not allowed for current user group",
			})
			return
		}
		if !ratio_setting.ContainsGroupRatio(tokenGroup) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "token group is deprecated",
			})
			return
		}
		usingGroup = tokenGroup
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)

	normalizedModel := ratio_setting.FormatMatchingModelName(req.Model)
	modelAllowed := isModelAllowedForToken(token, req.Model, normalizedModel)
	channel, resolvedGroup, channelAvailable, unavailableReason := resolveRatiosChannel(c, token, req.Model, userGroup)
	if resolvedGroup != "" {
		usingGroup = resolvedGroup
		common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
	}

	relayInfo := &relaycommon.RelayInfo{
		TokenId:         token.Id,
		TokenKey:        token.Key,
		TokenGroup:      token.Group,
		UserId:          token.UserId,
		UserGroup:       userGroup,
		UsingGroup:      usingGroup,
		OriginModelName: req.Model,
		UserSetting:     userCache.GetSetting(),
	}

	actualModel := req.Model
	if channel != nil {
		c.Set("model_mapping", channel.GetModelMapping())
		if err := helper.ModelMappedHelper(c, relayInfo, nil); err == nil && relayInfo.UpstreamModelName != "" {
			actualModel = relayInfo.UpstreamModelName
		}
	}

	priceData, priceErr := helper.ModelPriceHelper(c, relayInfo, 0, &types.TokenCountMeta{})
	billingMode := billing_setting.GetBillingMode(req.Model)
	_, hasBillingExpr := billing_setting.GetBillingExpr(req.Model)
	hasModelBillingConfig := helper.HasModelBillingConfig(req.Model)
	acceptUnsetRatioModel := operation_setting.SelfUseModeEnabled || relayInfo.UserSetting.AcceptUnsetRatioModel

	if priceErr != nil && !channelAvailable {
		unavailableReason = firstNonEmpty(unavailableReason, priceErr.Error())
	}

	available := modelAllowed && channelAvailable && priceErr == nil
	if !modelAllowed {
		unavailableReason = firstNonEmpty(unavailableReason, "model is not allowed by token model limits")
	}
	if priceErr != nil && unavailableReason == "" {
		unavailableReason = priceErr.Error()
	}

	resp := gin.H{
		"model":                    req.Model,
		"normalized_model":         normalizedModel,
		"actual_model":             actualModel,
		"token_group":              tokenGroup,
		"user_group":               userGroup,
		"using_group":              usingGroup,
		"auto_groups":              autoGroups,
		"model_limit_allowed":      modelAllowed,
		"accept_unset_ratio_model": acceptUnsetRatioModel,
		"has_model_billing_config": hasModelBillingConfig,
		"billing_mode":             billingMode,
		"billing_expr_configured":  hasBillingExpr,
		"channel_available":        channelAvailable,
		"available":                available,
		"unavailable_reason":       unavailableReason,
	}

	if channel != nil {
		resp["channel_id"] = channel.Id
		resp["channel_name"] = channel.Name
		resp["channel"] = ratiosChannelInfo{
			Id:       channel.Id,
			Name:     channel.Name,
			Type:     channel.Type,
			Group:    channel.Group,
			Models:   channel.Models,
			Priority: channel.GetPriority(),
			Weight:   channel.GetWeight(),
		}
	}

	if priceErr == nil {
		resp["use_price"] = priceData.UsePrice
		resp["free_model"] = priceData.FreeModel
		resp["model_price"] = priceData.ModelPrice
		resp["model_ratio"] = priceData.ModelRatio
		resp["completion_ratio"] = priceData.CompletionRatio
		resp["cache_ratio"] = priceData.CacheRatio
		resp["create_cache_ratio"] = priceData.CacheCreationRatio
		resp["cache_creation_5m_ratio"] = priceData.CacheCreation5mRatio
		resp["cache_creation_1h_ratio"] = priceData.CacheCreation1hRatio
		resp["image_ratio"] = priceData.ImageRatio
		resp["audio_ratio"] = priceData.AudioRatio
		resp["audio_completion_ratio"] = priceData.AudioCompletionRatio
		resp["group_ratio"] = priceData.GroupRatioInfo.GroupRatio
		resp["user_group_ratio"] = priceData.GroupRatioInfo.GroupSpecialRatio
		resp["has_special_group_ratio"] = priceData.GroupRatioInfo.HasSpecialRatio
		resp["quota_to_pre_consume"] = priceData.QuotaToPreConsume
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    resp,
	})
}

func resolveRatiosChannel(c *gin.Context, token *model.Token, modelName string, userGroup string) (*model.Channel, string, bool, string) {
	if token == nil {
		return nil, "", false, "token is nil"
	}

	if token.Group == "auto" {
		autoGroups := service.GetUserAutoGroup(userGroup)
		if len(autoGroups) == 0 {
			return nil, "", false, "auto groups is not enabled or no usable auto group"
		}
		retry := 0
		param := &service.RetryParam{
			Ctx:        c,
			TokenGroup: token.Group,
			ModelName:  modelName,
			Retry:      &retry,
		}
		channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(param)
		if err != nil {
			return nil, selectGroup, false, err.Error()
		}
		if channel == nil {
			return nil, selectGroup, false, "no available channel for model in auto groups"
		}
		return channel, selectGroup, true, ""
	}

	usingGroup := userGroup
	if token.Group != "" {
		usingGroup = token.Group
	}
	channel, err := model.GetRandomSatisfiedChannel(usingGroup, modelName, 0)
	if err != nil {
		return nil, usingGroup, false, err.Error()
	}
	if channel == nil {
		return nil, usingGroup, false, "no available channel for model in current group"
	}
	return channel, usingGroup, true, ""
}

func isModelAllowedForToken(token *model.Token, modelName string, normalizedModel string) bool {
	if token == nil || !token.IsModelLimitsEnabled() {
		return true
	}
	limits := token.GetModelLimitsMap()
	if limits[modelName] {
		return true
	}
	if normalizedModel != modelName && limits[normalizedModel] {
		return true
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

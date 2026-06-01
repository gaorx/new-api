package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// filterPricingByUsableGroups 根据用户可用分组过滤可见的定价项。
// 参数：
//   - pricing：原始定价配置列表。
//   - usableGroup：当前用户可使用的分组集合。
//
// 返回：
//   - []model.Pricing：按可用分组过滤后的定价列表。
func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	// 没有定价项时直接返回原始结果。
	if len(pricing) == 0 {
		return pricing
	}
	// 没有任何可用分组时，直接返回空列表。
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	// 保留 enable_group 包含 all 或与用户可用分组有交集的定价项。
	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

// GetPricing 获取当前用户可见的定价配置、供应商和分组倍率信息。
// 参数：
//   - c：当前请求上下文，用于读取登录用户并返回定价数据。
func GetPricing(c *gin.Context) {
	// 先读取系统全量定价配置。
	pricing := model.GetPricing()
	// 尝试获取当前用户 ID；未登录时按匿名场景处理。
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	// 先复制一份基础分组倍率配置，后续可能按用户主分组做覆盖。
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	if exists {
		// 若用户已登录，则读取用户缓存并应用组到组倍率覆盖。
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	// 计算当前用户可使用的分组，并据此过滤可见定价项。
	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	// 清理掉用户不可使用分组的倍率项，避免前端拿到无效配置。
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	// 返回定价列表、供应商、分组倍率、可用分组和自动分组信息。
	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

// ResetModelRatio 将模型倍率重置为系统默认值。
// 参数：
//   - c：当前请求上下文，用于返回重置结果。
func ResetModelRatio(c *gin.Context) {
	// 先生成默认模型倍率配置的 JSON 表达。
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	// 将默认倍率写回系统配置存储。
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	// 同步刷新内存中的模型倍率配置。
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	// 返回模型倍率重置成功结果。
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}

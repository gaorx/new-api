package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// GetGroups 返回当前系统已配置的全部分组名称列表。
// 参数：
//   - c：当前请求上下文，用于输出统一的成功响应。
func GetGroups(c *gin.Context) {
	// 遍历分组倍率配置，提取现有分组名称。
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}

	// 将分组名称列表直接返回给调用方。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

// GetUserGroups 返回当前用户可使用的分组及其展示说明。
// 参数：
//   - c：当前请求上下文，用于读取登录用户 ID 并输出可用分组信息。
func GetUserGroups(c *gin.Context) {
	// 准备结果容器，并读取当前用户的所属分组作为权限计算基础。
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)

	// 遍历系统中的所有分组，只收集当前用户真正有权限使用的那些条目。
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			usableGroups[groupName] = map[string]interface{}{
				"ratio": service.GetUserGroupRatio(userGroup, groupName),
				"desc":  desc,
			}
		}
	}

	// auto 分组是一个特殊入口，需要单独补充展示说明和固定文案。
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}

	// 返回用户可用分组的聚合结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}

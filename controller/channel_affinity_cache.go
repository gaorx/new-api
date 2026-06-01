package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetChannelAffinityCacheStats 返回渠道 Affinity 缓存的总体统计信息。
// 参数：
//   - c：当前请求上下文，用于输出标准 JSON 响应。
func GetChannelAffinityCacheStats(c *gin.Context) {
	// 从 service 层读取当前缓存命中、条目数量等统计信息。
	stats := service.GetChannelAffinityCacheStats()

	// 将统计结果按统一成功响应格式返回给前端。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}

// ClearChannelAffinityCache 按规则名或全量方式清空渠道 Affinity 缓存。
// 参数：
//   - c：当前请求上下文，用于读取查询参数并返回清理结果。
func ClearChannelAffinityCache(c *gin.Context) {
	// 读取清理范围相关参数，支持 all=true 全清或按 rule_name 定向清理。
	all := strings.TrimSpace(c.Query("all"))
	ruleName := strings.TrimSpace(c.Query("rule_name"))

	// 当显式指定 all=true 时，直接清空全部缓存并返回删除数量。
	if all == "true" {
		deleted := service.ClearChannelAffinityCacheAll()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"deleted": deleted,
			},
		})
		return
	}

	// 定向清理模式下必须提供规则名，否则无法确定目标缓存集合。
	if ruleName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少参数：rule_name，或使用 all=true 清空全部",
		})
		return
	}

	// 按指定规则名清理对应缓存，并把 service 层错误透传给调用方。
	deleted, err := service.ClearChannelAffinityCacheByRuleName(ruleName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 清理成功后返回本次删除的缓存条目数量。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"deleted": deleted,
		},
	})
}

// GetChannelAffinityUsageCacheStats 返回某个 Affinity 使用量缓存条目的统计信息。
// 参数：
//   - c：当前请求上下文，用于读取 rule_name、using_group、key_fp 等查询参数。
func GetChannelAffinityUsageCacheStats(c *gin.Context) {
	// 提取定位使用量缓存所需的关键参数。
	ruleName := strings.TrimSpace(c.Query("rule_name"))
	usingGroup := strings.TrimSpace(c.Query("using_group"))
	keyFp := strings.TrimSpace(c.Query("key_fp"))

	// rule_name 是定位规则缓存的主键之一，缺失时直接返回参数错误。
	if ruleName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "missing param: rule_name",
		})
		return
	}

	// key_fp 是区分密钥使用缓存的必要参数，缺失时无法查询准确结果。
	if keyFp == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "missing param: key_fp",
		})
		return
	}

	// 调用 service 层获取指定规则和密钥维度下的使用量缓存统计。
	stats := service.GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFp)

	// 将查询结果按统一成功响应格式返回。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}

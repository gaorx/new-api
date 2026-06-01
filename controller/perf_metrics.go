package controller

import (
	"net/http"
	"strconv"

	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// GetPerfMetricsSummary 获取近一段时间内所有模型与分组的性能指标汇总。
// 参数：
//   - c：当前请求上下文，用于读取小时数筛选参数并返回汇总结果。
func GetPerfMetricsSummary(c *gin.Context) {
	// 默认查询最近 24 小时的数据，允许通过查询参数覆盖。
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	// 基于当前启用的分组配置，加上 auto 分组，一并查询汇总数据。
	activeGroups := append(lo.Keys(ratio_setting.GetGroupRatioCopy()), "auto")
	result, err := perfmetrics.QuerySummaryAll(hours, activeGroups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 返回性能指标汇总结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetPerfMetrics 获取指定模型在一段时间内的性能指标明细。
// 参数：
//   - c：当前请求上下文，用于读取模型名、分组和小时数筛选参数。
func GetPerfMetrics(c *gin.Context) {
	// model 是必填参数，没有模型名就无法查询指标。
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	// 默认查询最近 24 小时的数据。
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	// 按模型、分组和时间范围查询性能指标。
	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: c.Query("group"),
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 过滤掉当前已不在启用分组配置中的历史分组数据。
	result.Groups = filterActiveGroups(result.Groups)

	// 返回模型性能指标明细。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// filterActiveGroups 过滤出当前仍然有效的分组指标结果。
// 参数：
//   - groups：原始分组指标结果列表。
//
// 返回：
//   - []perfmetrics.GroupResult：仅保留当前活跃分组和 auto 分组的结果。
func filterActiveGroups(groups []perfmetrics.GroupResult) []perfmetrics.GroupResult {
	// 读取当前有效分组配置，并基于该配置过滤结果。
	activeRatios := ratio_setting.GetGroupRatioCopy()
	return lo.Filter(groups, func(g perfmetrics.GroupResult, _ int) bool {
		_, ok := activeRatios[g.Group]
		return ok || g.Group == "auto"
	})
}

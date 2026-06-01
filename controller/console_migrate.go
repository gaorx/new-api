// 用于迁移检测的旧键，该文件下个版本会删除

package controller

import (
	"encoding/json"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// MigrateConsoleSetting 迁移旧的控制台相关配置到 console_setting.*
// 参数：
//   - c：当前请求上下文，用于触发一次性迁移并返回结果。
func MigrateConsoleSetting(c *gin.Context) {
	// 读取全部 option
	// 先加载所有旧 option，后续统一放入 map 便于按键名迁移。
	opts, err := model.AllOption()
	if err != nil {
		common.SysError("failed to get all options: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取配置失败，请稍后重试"})
		return
	}
	// 建立 map
	// 把 option 列表拍平成 key-value map，方便直接按旧键读取。
	valMap := map[string]string{}
	for _, o := range opts {
		valMap[o.Key] = o.Value
	}

	// 处理 APIInfo
	// 旧版 ApiInfo 迁移到 console_setting.api_info，并限制最大条目数。
	if v := valMap["ApiInfo"]; v != "" {
		var arr []map[string]interface{}
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			if len(arr) > 50 {
				arr = arr[:50]
			}
			bytes, _ := json.Marshal(arr)
			model.UpdateOption("console_setting.api_info", string(bytes))
		}
		model.UpdateOption("ApiInfo", "")
	}
	// Announcements 直接搬
	// 公告配置无需结构转换，直接迁移到新键并清空旧键。
	if v := valMap["Announcements"]; v != "" {
		model.UpdateOption("console_setting.announcements", v)
		model.UpdateOption("Announcements", "")
	}
	// FAQ 转换
	// FAQ 需要兼容旧字段名，并统一映射为 question/answer 结构。
	if v := valMap["FAQ"]; v != "" {
		var arr []map[string]interface{}
		if err := json.Unmarshal([]byte(v), &arr); err == nil {
			out := []map[string]interface{}{}
			for _, item := range arr {
				q, _ := item["question"].(string)
				if q == "" {
					q, _ = item["title"].(string)
				}
				a, _ := item["answer"].(string)
				if a == "" {
					a, _ = item["content"].(string)
				}
				if q != "" && a != "" {
					out = append(out, map[string]interface{}{"question": q, "answer": a})
				}
			}
			if len(out) > 50 {
				out = out[:50]
			}
			bytes, _ := json.Marshal(out)
			model.UpdateOption("console_setting.faq", string(bytes))
		}
		model.UpdateOption("FAQ", "")
	}
	// Uptime Kuma 迁移到新的 groups 结构（console_setting.uptime_kuma_groups）
	// 只有同时存在旧 URL 与 Slug 时，才构造一条默认分组记录迁移到新结构。
	url := valMap["UptimeKumaUrl"]
	slug := valMap["UptimeKumaSlug"]
	if url != "" && slug != "" {
		// 仅当同时存在 URL 与 Slug 时才进行迁移
		groups := []map[string]interface{}{
			{
				"id":           1,
				"categoryName": "old",
				"url":          url,
				"slug":         slug,
				"description":  "",
			},
		}
		bytes, _ := json.Marshal(groups)
		model.UpdateOption("console_setting.uptime_kuma_groups", string(bytes))
	}
	// 清空旧键内容
	// 迁移完成后，把旧键值置空，避免新旧配置并存导致读取歧义。
	if url != "" {
		model.UpdateOption("UptimeKumaUrl", "")
	}
	if slug != "" {
		model.UpdateOption("UptimeKumaSlug", "")
	}

	// 删除旧键记录
	// 从数据库中物理删除旧键，确保后续只保留新的 console_setting.* 配置。
	oldKeys := []string{"ApiInfo", "Announcements", "FAQ", "UptimeKumaUrl", "UptimeKumaSlug"}
	model.DB.Where("key IN ?", oldKeys).Delete(&model.Option{})

	// 重新加载 OptionMap
	// 重建内存 OptionMap，让新配置立即对运行时生效。
	model.InitOptionMap()
	common.SysLog("console setting migrated")
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "migrated"})
}

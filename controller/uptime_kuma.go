package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/setting/console_setting"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

const (
	requestTimeout   = 30 * time.Second
	httpTimeout      = 10 * time.Second
	uptimeKeySuffix  = "_24"
	apiStatusPath    = "/api/status-page/"
	apiHeartbeatPath = "/api/status-page/heartbeat/"
)

// Monitor 表示 Uptime Kuma 中单个监控项的公开状态。
type Monitor struct {
	Name   string  `json:"name"`           // 监控项名称。
	Uptime float64 `json:"uptime"`         // 24 小时可用率。
	Status int     `json:"status"`         // 最新心跳状态码。
	Group  string  `json:"group,omitempty"` // 所属分组名。
}

// UptimeGroupResult 表示单个 Uptime Kuma 分组的聚合结果。
type UptimeGroupResult struct {
	CategoryName string    `json:"categoryName"` // 前端展示的分类名称。
	Monitors     []Monitor `json:"monitors"`     // 该分类下的监控项列表。
}

// getAndDecode 发起 GET 请求并把 JSON 响应解码到目标对象中。
// 参数：
//   - ctx：请求上下文。
//   - client：HTTP 客户端。
//   - url：目标地址。
//   - dest：响应解码目标。
//
// 返回：
//   - error：请求失败、状态码非 200 或解码失败时返回错误。
func getAndDecode(ctx context.Context, client *http.Client, url string, dest interface{}) error {
	// 构造带上下文的 GET 请求。
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	// 发起请求并确保响应体最终被关闭。
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("non-200 status")
	}

	// 把 JSON 响应体解码到目标对象。
	return json.NewDecoder(resp.Body).Decode(dest)
}

// fetchGroupData 拉取单个 Uptime Kuma 分组的状态页和心跳数据。
// 参数：
//   - ctx：请求上下文。
//   - client：HTTP 客户端。
//   - groupConfig：单个 Uptime Kuma 分组配置。
//
// 返回：
//   - UptimeGroupResult：聚合后的监控分组结果。
func fetchGroupData(ctx context.Context, client *http.Client, groupConfig map[string]interface{}) UptimeGroupResult {
	// 从配置中提取地址、slug 和分类名，并先构造默认返回值。
	url, _ := groupConfig["url"].(string)
	slug, _ := groupConfig["slug"].(string)
	categoryName, _ := groupConfig["categoryName"].(string)

	result := UptimeGroupResult{
		CategoryName: categoryName,
		Monitors:     []Monitor{},
	}

	if url == "" || slug == "" {
		return result
	}

	// 规范化 base URL，并准备状态页和心跳页的数据结构。
	baseURL := strings.TrimSuffix(url, "/")

	var statusData struct {
		PublicGroupList []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			MonitorList []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"monitorList"`
		} `json:"publicGroupList"`
	}

	var heartbeatData struct {
		HeartbeatList map[string][]struct {
			Status int `json:"status"`
		} `json:"heartbeatList"`
		UptimeList map[string]float64 `json:"uptimeList"`
	}

	// 并发请求状态页与心跳数据，减少整体等待时间。
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return getAndDecode(gCtx, client, baseURL+apiStatusPath+slug, &statusData)
	})
	g.Go(func() error {
		return getAndDecode(gCtx, client, baseURL+apiHeartbeatPath+slug, &heartbeatData)
	})

	if g.Wait() != nil {
		return result
	}

	// 把状态页和心跳数据合并成前端需要的统一监控结构。
	for _, pg := range statusData.PublicGroupList {
		if len(pg.MonitorList) == 0 {
			continue
		}

		for _, m := range pg.MonitorList {
			monitor := Monitor{
				Name:  m.Name,
				Group: pg.Name,
			}

			monitorID := strconv.Itoa(m.ID)

			if uptime, exists := heartbeatData.UptimeList[monitorID+uptimeKeySuffix]; exists {
				monitor.Uptime = uptime
			}

			if heartbeats, exists := heartbeatData.HeartbeatList[monitorID]; exists && len(heartbeats) > 0 {
				monitor.Status = heartbeats[0].Status
			}

			result.Monitors = append(result.Monitors, monitor)
		}
	}

	return result
}

// GetUptimeKumaStatus 获取所有配置分组的 Uptime Kuma 公共状态数据。
// 参数：
//   - c：当前请求上下文，用于返回 Uptime Kuma 分组状态。
func GetUptimeKumaStatus(c *gin.Context) {
	// 读取配置好的 Uptime Kuma 分组；若没有配置则直接返回空列表。
	groups := console_setting.GetUptimeKumaGroups()
	if len(groups) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": []UptimeGroupResult{}})
		return
	}

	// 为整批查询设置统一超时，并创建短超时 HTTP 客户端。
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	client := &http.Client{Timeout: httpTimeout}
	results := make([]UptimeGroupResult, len(groups))

	// 并发拉取每个分组的状态数据。
	g, gCtx := errgroup.WithContext(ctx)
	for i, group := range groups {
		i, group := i, group
		g.Go(func() error {
			results[i] = fetchGroupData(gCtx, client, group)
			return nil
		})
	}

	// 返回所有分组的监控状态结果。
	g.Wait()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": results})
}

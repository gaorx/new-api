package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// UpdateMidjourneyTaskBulk 持续轮询所有未完成的 Midjourney 任务，并同步上游状态到本地数据库。
func UpdateMidjourneyTaskBulk() {
	//imageModel := "midjourney"

	// 创建后台轮询使用的基础上下文，供日志输出复用。
	ctx := context.TODO()

	// 进入常驻轮询循环，按固定周期扫描并更新任务状态。
	for {
		time.Sleep(time.Duration(15) * time.Second)

		// 读取当前所有未完成任务；没有待处理任务时直接进入下一轮。
		tasks := model.GetAllUnFinishTasks()
		if len(tasks) == 0 {
			continue
		}

		// 按渠道归并任务，同时收集异常的空任务 ID 记录，便于后续批量处理。
		logger.LogInfo(ctx, fmt.Sprintf("检测到未完成的任务数有: %v", len(tasks)))
		taskChannelM := make(map[int][]string)
		taskM := make(map[string]*model.Midjourney)
		nullTaskIds := make([]int, 0)
		for _, task := range tasks {
			if task.MjId == "" {
				// 统计失败的未完成任务
				nullTaskIds = append(nullTaskIds, task.Id)
				continue
			}
			taskM[task.MjId] = task
			taskChannelM[task.ChannelId] = append(taskChannelM[task.ChannelId], task.MjId)
		}

		// 对缺失 mj_id 的异常任务做失败收口，避免它们永久停留在未完成状态。
		if len(nullTaskIds) > 0 {
			err := model.MjBulkUpdateByTaskIds(nullTaskIds, map[string]any{
				"status":   "FAILURE",
				"progress": "100%",
			})
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Fix null mj_id task error: %v", err))
			} else {
				logger.LogInfo(ctx, fmt.Sprintf("Fix null mj_id task success: %v", nullTaskIds))
			}
		}
		if len(taskChannelM) == 0 {
			continue
		}

		// 按渠道批量查询上游状态，减少请求次数并复用同一渠道配置。
		for channelId, taskIds := range taskChannelM {
			logger.LogInfo(ctx, fmt.Sprintf("渠道 #%d 未完成的任务有: %d", channelId, len(taskIds)))
			if len(taskIds) == 0 {
				continue
			}

			// 先读取渠道配置；如果渠道信息不可用，则把对应任务统一标记为失败。
			midjourneyChannel, err := model.CacheGetChannel(channelId)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("CacheGetChannel: %v", err))
				err := model.MjBulkUpdate(taskIds, map[string]any{
					"fail_reason": fmt.Sprintf("获取渠道信息失败，请联系管理员，渠道ID：%d", channelId),
					"status":      "FAILURE",
					"progress":    "100%",
				})
				if err != nil {
					logger.LogInfo(ctx, fmt.Sprintf("UpdateMidjourneyTask error: %v", err))
				}
				continue
			}

			// 构造上游批量查询地址，后续一次请求拉回这一批任务的最新状态。
			requestUrl := fmt.Sprintf("%s/mj/task/list-by-condition", *midjourneyChannel.BaseURL)

			// 组装本轮任务查询请求体，把所有待查任务 ID 一并传给上游。
			body, _ := json.Marshal(map[string]any{
				"ids": taskIds,
			})
			req, err := http.NewRequest("POST", requestUrl, bytes.NewBuffer(body))
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Get Task error: %v", err))
				continue
			}

			// 设置超时时间
			timeout := time.Second * 15
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			// 使用带有超时的 context 创建新的请求
			req = req.WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("mj-api-secret", midjourneyChannel.Key)

			// 发起上游批量查询请求；网络失败或非 200 响应时记录日志并等待下一轮重试。
			resp, err := service.GetHttpClient().Do(req)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Get Task Do req error: %v", err))
				continue
			}
			if resp.StatusCode != http.StatusOK {
				logger.LogError(ctx, fmt.Sprintf("Get Task status code: %d", resp.StatusCode))
				continue
			}

			// 读取并解析上游返回的任务列表，为逐条同步本地状态做准备。
			responseBody, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Get Mjp Task parse body error: %v", err))
				continue
			}
			var responseItems []dto.MidjourneyDto
			err = json.Unmarshal(responseBody, &responseItems)
			if err != nil {
				logger.LogError(ctx, fmt.Sprintf("Get Mjp Task parse body error2: %v, body: %s", err, string(responseBody)))
				continue
			}
			resp.Body.Close()
			req.Body.Close()
			cancel()

			// 遍历每条上游任务结果，判断是否需要更新本地记录。
			for _, responseItem := range responseItems {
				task := taskM[responseItem.MjId]

				// 对超过一小时仍未完成的任务主动判定为超时失败。
				useTime := (time.Now().UnixNano() / int64(time.Millisecond)) - task.SubmitTime
				// 如果时间超过一小时，且进度不是100%，则认为任务失败
				if useTime > 3600000 && task.Progress != "100%" {
					responseItem.FailReason = "上游任务超时（超过1小时）"
					responseItem.Status = "FAILURE"
				}
				if !checkMjTaskNeedUpdate(task, responseItem) {
					continue
				}

				// 将上游返回的最新状态字段映射回本地任务对象。
				preStatus := task.Status
				task.Code = 1
				task.Progress = responseItem.Progress
				task.PromptEn = responseItem.PromptEn
				task.State = responseItem.State
				task.SubmitTime = responseItem.SubmitTime
				task.StartTime = responseItem.StartTime
				task.FinishTime = responseItem.FinishTime
				task.ImageUrl = responseItem.ImageUrl
				task.Status = responseItem.Status
				task.FailReason = responseItem.FailReason
				if responseItem.Properties != nil {
					propertiesStr, _ := json.Marshal(responseItem.Properties)
					task.Properties = string(propertiesStr)
				}
				if responseItem.Buttons != nil {
					buttonStr, _ := json.Marshal(responseItem.Buttons)
					task.Buttons = string(buttonStr)
				}
				// 映射 VideoUrl
				task.VideoUrl = responseItem.VideoUrl

				// 映射 VideoUrls - 将数组序列化为 JSON 字符串
				if responseItem.VideoUrls != nil && len(responseItem.VideoUrls) > 0 {
					videoUrlsStr, err := json.Marshal(responseItem.VideoUrls)
					if err != nil {
						logger.LogError(ctx, fmt.Sprintf("序列化 VideoUrls 失败: %v", err))
						task.VideoUrls = "[]" // 失败时设置为空数组
					} else {
						task.VideoUrls = string(videoUrlsStr)
					}
				} else {
					task.VideoUrls = "" // 空值时清空字段
				}

				// 当任务被明确判定为失败时，标记后续需要退还用户额度。
				shouldReturnQuota := false
				if (task.Progress != "100%" && responseItem.FailReason != "") || (task.Progress == "100%" && task.Status == "FAILURE") {
					logger.LogInfo(ctx, task.MjId+" 构建失败，"+task.FailReason)
					task.Progress = "100%"
					if task.Quota != 0 {
						shouldReturnQuota = true
					}
				}

				// 使用带旧状态校验的更新方式，避免并发轮询覆盖较新的状态。
				won, err := task.UpdateWithStatus(preStatus)
				if err != nil {
					logger.LogError(ctx, "UpdateMidjourneyTask task error: "+err.Error())
				} else if won && shouldReturnQuota {
					// 在成功推进到失败态后，退回额度并记录退款账单日志。
					err = model.IncreaseUserQuota(task.UserId, task.Quota, false)
					if err != nil {
						logger.LogError(ctx, "fail to increase user quota: "+err.Error())
					}
					model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
						UserId:    task.UserId,
						LogType:   model.LogTypeRefund,
						Content:   "",
						ChannelId: task.ChannelId,
						ModelName: service.CovertMjpActionToModelName(task.Action),
						Quota:     task.Quota,
						Other: map[string]interface{}{
							"task_id": task.MjId,
							"reason":  "构图失败",
						},
					})
				}
			}
		}
	}
}

// checkMjTaskNeedUpdate 判断上游返回的 Midjourney 任务是否值得写回本地数据库。
// 参数：
//   - oldTask：数据库中当前保存的旧任务记录。
//   - newTask：本轮从上游获取到的新任务快照。
//
// 返回：
//   - bool：true 表示本地任务需要更新，false 表示本轮可以跳过写入。
func checkMjTaskNeedUpdate(oldTask *model.Midjourney, newTask dto.MidjourneyDto) bool {
	// 未进入正常同步态的本地任务，直接允许覆盖更新。
	if oldTask.Code != 1 {
		return true
	}

	// 逐项比较核心状态字段，只要任何关键字段变化就需要刷新本地数据。
	if oldTask.Progress != newTask.Progress {
		return true
	}
	if oldTask.PromptEn != newTask.PromptEn {
		return true
	}
	if oldTask.State != newTask.State {
		return true
	}
	if oldTask.SubmitTime != newTask.SubmitTime {
		return true
	}
	if oldTask.StartTime != newTask.StartTime {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if oldTask.ImageUrl != newTask.ImageUrl {
		return true
	}
	if oldTask.Status != newTask.Status {
		return true
	}
	if oldTask.FailReason != newTask.FailReason {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if oldTask.Progress != "100%" && newTask.FailReason != "" {
		return true
	}
	// 检查 VideoUrl 是否需要更新
	if oldTask.VideoUrl != newTask.VideoUrl {
		return true
	}
	// 检查 VideoUrls 是否需要更新
	if newTask.VideoUrls != nil && len(newTask.VideoUrls) > 0 {
		newVideoUrlsStr, _ := json.Marshal(newTask.VideoUrls)
		if oldTask.VideoUrls != string(newVideoUrlsStr) {
			return true
		}
	} else if oldTask.VideoUrls != "" {
		// 如果新数据没有 VideoUrls 但旧数据有，需要更新（清空）
		return true
	}

	return false
}

// GetAllMidjourney 返回管理员视角的 Midjourney 任务分页列表。
// 参数：
//   - c：当前请求上下文，用于读取分页参数和查询条件。
func GetAllMidjourney(c *gin.Context) {
	// 先解析分页参数，供后续任务列表查询和响应封装使用。
	pageInfo := common.GetPageQuery(c)

	// 解析其他查询参数
	queryParams := model.TaskQueryParams{
		ChannelID:      c.Query("channel_id"),
		MjID:           c.Query("mj_id"),
		StartTimestamp: c.Query("start_timestamp"),
		EndTimestamp:   c.Query("end_timestamp"),
	}

	// 按筛选条件查询任务列表及总数，用于后台分页展示。
	items := model.GetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.CountAllTasks(queryParams)

	// 如果启用了图片转发地址，则把图片 URL 改写为本站代理访问路径。
	if setting.MjForwardUrlEnabled {
		for i, midjourney := range items {
			midjourney.ImageUrl = system_setting.ServerAddress + "/mj/image/" + midjourney.MjId
			items[i] = midjourney
		}
	}

	// 回填分页元信息并通过统一成功响应返回。
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// GetUserMidjourney 返回当前登录用户自己的 Midjourney 任务分页列表。
// 参数：
//   - c：当前请求上下文，用于读取用户身份和筛选条件。
func GetUserMidjourney(c *gin.Context) {
	// 解析分页参数，并准备按当前用户维度查询任务记录。
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	// 组装用户任务查询条件，支持按任务 ID 和时间范围过滤。
	queryParams := model.TaskQueryParams{
		MjID:           c.Query("mj_id"),
		StartTimestamp: c.Query("start_timestamp"),
		EndTimestamp:   c.Query("end_timestamp"),
	}

	// 查询当前用户的 Midjourney 任务列表及总数。
	items := model.GetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.CountAllUserTask(userId, queryParams)

	// 如已启用图片转发，则把返回图片地址统一改写为本站代理路径。
	if setting.MjForwardUrlEnabled {
		for i, midjourney := range items {
			midjourney.ImageUrl = system_setting.ServerAddress + "/mj/image/" + midjourney.MjId
			items[i] = midjourney
		}
	}

	// 组装分页结果并返回给前端。
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

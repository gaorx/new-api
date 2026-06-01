package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/ionet"
	"github.com/gin-gonic/gin"
)

// getIoAPIKey 从系统配置中读取 io.net 部署功能是否启用以及 API Key。
// 参数：
//   - c：当前请求上下文；当配置缺失时用于直接返回错误提示。
//
// 返回：
//   - string：可用的 io.net API Key。
//   - bool：true 表示成功获取，false 表示已向前端返回错误。
func getIoAPIKey(c *gin.Context) (string, bool) {
	// 从 OptionMap 读取部署开关和 API Key，并在读锁保护下保证并发安全。
	common.OptionMapRWMutex.RLock()
	enabled := common.OptionMap["model_deployment.ionet.enabled"] == "true"
	apiKey := common.OptionMap["model_deployment.ionet.api_key"]
	common.OptionMapRWMutex.RUnlock()
	if !enabled || strings.TrimSpace(apiKey) == "" {
		common.ApiErrorMsg(c, "io.net model deployment is not enabled or api key missing")
		return "", false
	}
	return apiKey, true
}

// GetModelDeploymentSettings 返回当前 io.net 部署能力的启用和配置状态。
// 参数：
//   - c：当前请求上下文，用于输出部署设置概览。
func GetModelDeploymentSettings(c *gin.Context) {
	// 读取部署功能是否开启，以及 API Key 是否已经配置。
	common.OptionMapRWMutex.RLock()
	enabled := common.OptionMap["model_deployment.ionet.enabled"] == "true"
	hasAPIKey := strings.TrimSpace(common.OptionMap["model_deployment.ionet.api_key"]) != ""
	common.OptionMapRWMutex.RUnlock()

	common.ApiSuccess(c, gin.H{
		"provider":    "io.net",
		"enabled":     enabled,
		"configured":  hasAPIKey,
		"can_connect": enabled && hasAPIKey,
	})
}

// getIoClient 构造普通 io.net API 客户端。
func getIoClient(c *gin.Context) (*ionet.Client, bool) {
	// 先获取 API Key，成功后创建普通 io.net 客户端。
	apiKey, ok := getIoAPIKey(c)
	if !ok {
		return nil, false
	}
	return ionet.NewClient(apiKey), true
}

// getIoEnterpriseClient 构造 io.net Enterprise API 客户端。
func getIoEnterpriseClient(c *gin.Context) (*ionet.Client, bool) {
	// 先获取 API Key，成功后创建 Enterprise 客户端。
	apiKey, ok := getIoAPIKey(c)
	if !ok {
		return nil, false
	}
	return ionet.NewEnterpriseClient(apiKey), true
}

// TestIoNetConnection 校验 io.net API Key 是否可用，并返回基础硬件可用信息。
// 参数：
//   - c：当前请求上下文，用于读取可选 api_key 并返回探测结果。
func TestIoNetConnection(c *gin.Context) {
	var req struct {
		APIKey string `json:"api_key"` // 可选的临时 API Key；为空时回退到系统已配置 key。
	}

	// 读取原始请求体；允许空请求体，表示直接使用系统配置的 API Key。
	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(bytes.TrimSpace(rawBody)) > 0 {
		if err := json.Unmarshal(rawBody, &req); err != nil {
			common.ApiErrorMsg(c, "invalid request payload")
			return
		}
	}

	// 优先使用请求里传入的 key；如果没传，则回退到系统配置。
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		common.OptionMapRWMutex.RLock()
		storedKey := strings.TrimSpace(common.OptionMap["model_deployment.ionet.api_key"])
		common.OptionMapRWMutex.RUnlock()
		if storedKey == "" {
			common.ApiErrorMsg(c, "api_key is required")
			return
		}
		apiKey = storedKey
	}

	// 调用 Enterprise API 做一次最小能力探测，用于验证 key 是否有效。
	client := ionet.NewEnterpriseClient(apiKey)
	result, err := client.GetMaxGPUsPerContainer()
	if err != nil {
		if apiErr, ok := err.(*ionet.APIError); ok {
			message := strings.TrimSpace(apiErr.Message)
			if message == "" {
				message = "failed to validate api key"
			}
			common.ApiErrorMsg(c, message)
			return
		}
		common.ApiError(c, err)
		return
	}

	// 汇总硬件总类目数和可用总量，兼容不同返回字段形式。
	totalHardware := 0
	totalAvailable := 0
	if result != nil {
		totalHardware = len(result.Hardware)
		totalAvailable = result.Total
		if totalAvailable == 0 {
			for _, hw := range result.Hardware {
				totalAvailable += hw.Available
			}
		}
	}

	// 返回连接测试成功结果。
	common.ApiSuccess(c, gin.H{
		"hardware_count":  totalHardware,
		"total_available": totalAvailable,
	})
}

// requireDeploymentID 从路径参数中提取 deployment ID，并在缺失时直接返回错误。
func requireDeploymentID(c *gin.Context) (string, bool) {
	// deployment ID 为空时无需继续执行后续请求。
	deploymentID := strings.TrimSpace(c.Param("id"))
	if deploymentID == "" {
		common.ApiErrorMsg(c, "deployment ID is required")
		return "", false
	}
	return deploymentID, true
}

// requireContainerID 从路径参数中提取 container ID，并在缺失时直接返回错误。
func requireContainerID(c *gin.Context) (string, bool) {
	// container ID 为空时直接向前端返回错误。
	containerID := strings.TrimSpace(c.Param("container_id"))
	if containerID == "" {
		common.ApiErrorMsg(c, "container ID is required")
		return "", false
	}
	return containerID, true
}

// mapIoNetDeployment 把 io.net 的部署对象映射成前端统一消费的部署展示结构。
func mapIoNetDeployment(d ionet.Deployment) map[string]interface{} {
	// 若上游未返回创建时间，则退化为当前时间，避免前端拿到零值时间戳。
	var created int64
	if d.CreatedAt.IsZero() {
		created = time.Now().Unix()
	} else {
		created = d.CreatedAt.Unix()
	}

	// 将剩余计算时长格式化为人类可读文本，兼顾小时和分钟展示。
	timeRemainingHours := d.ComputeMinutesRemaining / 60
	timeRemainingMins := d.ComputeMinutesRemaining % 60
	var timeRemaining string
	if timeRemainingHours > 0 {
		timeRemaining = fmt.Sprintf("%d hour %d minutes", timeRemainingHours, timeRemainingMins)
	} else if timeRemainingMins > 0 {
		timeRemaining = fmt.Sprintf("%d minutes", timeRemainingMins)
	} else {
		timeRemaining = "completed"
	}

	// 拼接硬件品牌、型号和数量，生成前端表格可直接展示的硬件摘要。
	hardwareInfo := fmt.Sprintf("%s %s x%d", d.BrandName, d.HardwareName, d.HardwareQuantity)

	// 映射成当前前端所需的统一字段结构。
	return map[string]interface{}{
		"id":                        d.ID,
		"deployment_name":           d.Name,
		"container_name":            d.Name,
		"status":                    strings.ToLower(d.Status),
		"type":                      "Container",
		"time_remaining":            timeRemaining,
		"time_remaining_minutes":    d.ComputeMinutesRemaining,
		"hardware_info":             hardwareInfo,
		"hardware_name":             d.HardwareName,
		"brand_name":                d.BrandName,
		"hardware_quantity":         d.HardwareQuantity,
		"completed_percent":         d.CompletedPercent,
		"compute_minutes_served":    d.ComputeMinutesServed,
		"compute_minutes_remaining": d.ComputeMinutesRemaining,
		"created_at":                created,
		"updated_at":                created,
		"model_name":                "",
		"model_version":             "",
		"instance_count":            d.HardwareQuantity,
		"resource_config": map[string]interface{}{
			"cpu":    "",
			"memory": "",
			"gpu":    strconv.Itoa(d.HardwareQuantity),
		},
		"description": "",
		"provider":    "io.net",
	}
}

// computeStatusCounts 统计部署列表中各状态的数量，供前端概览展示使用。
// 参数：
//   - total：总部署数量。
//   - deployments：当前页返回的部署列表。
//
// 返回：
//   - map[string]int64：按状态聚合后的数量统计。
func computeStatusCounts(total int, deployments []ionet.Deployment) map[string]int64 {
	// 先初始化 all 总数，以及前端常用状态的默认计数。
	counts := map[string]int64{
		"all": int64(total),
	}

	for _, status := range []string{"running", "completed", "failed", "deployment requested", "termination requested", "destroyed"} {
		counts[status] = 0
	}

	// 遍历部署列表，按标准化后的状态累加数量。
	for _, d := range deployments {
		status := strings.ToLower(strings.TrimSpace(d.Status))
		counts[status] = counts[status] + 1
	}

	return counts
}

// GetAllDeployments 分页获取全部 io.net 部署列表。
// 参数：
//   - c：当前请求上下文，用于读取分页和状态筛选参数并返回部署列表。
func GetAllDeployments(c *gin.Context) {
	// 先解析分页参数，并创建企业版 io.net 客户端。
	pageInfo := common.GetPageQuery(c)
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	// 根据请求中的状态筛选条件组装上游列表查询参数。
	status := c.Query("status")
	opts := &ionet.ListDeploymentsOptions{
		Status:    strings.ToLower(strings.TrimSpace(status)),
		Page:      pageInfo.GetPage(),
		PageSize:  pageInfo.GetPageSize(),
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	// 调用 io.net 获取部署分页数据。
	dl, err := client.ListDeployments(opts)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 把上游部署对象统一映射成前端使用的数据结构。
	items := make([]map[string]interface{}, 0, len(dl.Deployments))
	for _, d := range dl.Deployments {
		items = append(items, mapIoNetDeployment(d))
	}

	// 附带分页信息和状态统计一并返回。
	data := gin.H{
		"page":          pageInfo.GetPage(),
		"page_size":     pageInfo.GetPageSize(),
		"total":         dl.Total,
		"items":         items,
		"status_counts": computeStatusCounts(dl.Total, dl.Deployments),
	}
	common.ApiSuccess(c, data)
}

// SearchDeployments 按状态和关键字搜索部署列表。
// 参数：
//   - c：当前请求上下文，用于读取分页、状态和关键字筛选参数。
func SearchDeployments(c *gin.Context) {
	// 先解析分页信息，并初始化企业版客户端。
	pageInfo := common.GetPageQuery(c)
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	// 读取搜索条件，其中关键字在本地对返回结果做二次过滤。
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	keyword := strings.TrimSpace(c.Query("keyword"))

	// 先从上游按状态拉取一页部署列表。
	dl, err := client.ListDeployments(&ionet.ListDeploymentsOptions{
		Status:    status,
		Page:      pageInfo.GetPage(),
		PageSize:  pageInfo.GetPageSize(),
		SortBy:    "created_at",
		SortOrder: "desc",
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 如果携带关键字，则按部署名称做不区分大小写的本地过滤。
	filtered := make([]ionet.Deployment, 0, len(dl.Deployments))
	if keyword == "" {
		filtered = dl.Deployments
	} else {
		kw := strings.ToLower(keyword)
		for _, d := range dl.Deployments {
			if strings.Contains(strings.ToLower(d.Name), kw) {
				filtered = append(filtered, d)
			}
		}
	}

	// 把过滤后的结果映射成前端统一结构。
	items := make([]map[string]interface{}, 0, len(filtered))
	for _, d := range filtered {
		items = append(items, mapIoNetDeployment(d))
	}

	// 关键字过滤后总数以过滤结果为准，否则保持上游分页总数。
	total := dl.Total
	if keyword != "" {
		total = len(filtered)
	}

	// 返回搜索结果分页数据。
	data := gin.H{
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
		"total":     total,
		"items":     items,
	}
	common.ApiSuccess(c, data)
}

// GetDeployment 获取指定部署的详细信息。
// 参数：
//   - c：当前请求上下文，用于读取 deployment ID 并返回详情。
func GetDeployment(c *gin.Context) {
	// 初始化企业版客户端并提取部署 ID。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	// 读取上游部署详情。
	details, err := client.GetDeployment(deploymentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 整理成前端详情页可直接消费的统一字段结构。
	data := map[string]interface{}{
		"id":              details.ID,
		"deployment_name": details.ID,
		"model_name":      "",
		"model_version":   "",
		"status":          strings.ToLower(details.Status),
		"instance_count":  details.TotalContainers,
		"hardware_id":     details.HardwareID,
		"resource_config": map[string]interface{}{
			"cpu":    "",
			"memory": "",
			"gpu":    strconv.Itoa(details.TotalGPUs),
		},
		"created_at":                details.CreatedAt.Unix(),
		"updated_at":                details.CreatedAt.Unix(),
		"description":               "",
		"amount_paid":               details.AmountPaid,
		"completed_percent":         details.CompletedPercent,
		"gpus_per_container":        details.GPUsPerContainer,
		"total_gpus":                details.TotalGPUs,
		"total_containers":          details.TotalContainers,
		"hardware_name":             details.HardwareName,
		"brand_name":                details.BrandName,
		"compute_minutes_served":    details.ComputeMinutesServed,
		"compute_minutes_remaining": details.ComputeMinutesRemaining,
		"locations":                 details.Locations,
		"container_config":          details.ContainerConfig,
	}

	common.ApiSuccess(c, data)
}

// UpdateDeploymentName 修改部署名称。
// 参数：
//   - c：当前请求上下文，用于读取 deployment ID 和新的名称。
func UpdateDeploymentName(c *gin.Context) {
	// 初始化企业版客户端并读取部署 ID。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	// 绑定请求体，读取新的部署名称。
	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// 构造更新名称请求，并先做本地非空校验。
	updateReq := &ionet.UpdateClusterNameRequest{
		Name: strings.TrimSpace(req.Name),
	}

	if updateReq.Name == "" {
		common.ApiErrorMsg(c, "deployment name cannot be empty")
		return
	}

	// 修改前先向上游校验名称是否可用。
	available, err := client.CheckClusterNameAvailability(updateReq.Name)
	if err != nil {
		common.ApiError(c, fmt.Errorf("failed to check name availability: %w", err))
		return
	}

	if !available {
		common.ApiErrorMsg(c, "deployment name is not available, please choose a different name")
		return
	}

	// 向 io.net 提交名称更新请求。
	resp, err := client.UpdateClusterName(deploymentID, updateReq)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回名称更新结果。
	data := gin.H{
		"status":  resp.Status,
		"message": resp.Message,
		"id":      deploymentID,
		"name":    updateReq.Name,
	}
	common.ApiSuccess(c, data)
}

// UpdateDeployment 更新部署配置。
// 参数：
//   - c：当前请求上下文，用于读取 deployment ID 和更新请求体。
func UpdateDeployment(c *gin.Context) {
	// 初始化企业版客户端并读取部署 ID。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	// 绑定上游定义的部署更新请求结构。
	var req ionet.UpdateDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// 将更新请求转发给 io.net。
	resp, err := client.UpdateDeployment(deploymentID, &req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回部署更新结果。
	data := gin.H{
		"status":        resp.Status,
		"deployment_id": resp.DeploymentID,
	}
	common.ApiSuccess(c, data)
}

// ExtendDeployment 延长部署的运行时长。
// 参数：
//   - c：当前请求上下文，用于读取 deployment ID 和延长时长请求。
func ExtendDeployment(c *gin.Context) {
	// 初始化企业版客户端并读取部署 ID。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	// 绑定延长时长请求结构。
	var req ionet.ExtendDurationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// 调用上游接口延长部署时长，并拿回最新详情。
	details, err := client.ExtendDeployment(deploymentID, &req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 把延长后的详情重新映射为统一部署结构返回前端。
	data := mapIoNetDeployment(ionet.Deployment{
		ID:                      details.ID,
		Status:                  details.Status,
		Name:                    deploymentID,
		CompletedPercent:        float64(details.CompletedPercent),
		HardwareQuantity:        details.TotalGPUs,
		BrandName:               details.BrandName,
		HardwareName:            details.HardwareName,
		ComputeMinutesServed:    details.ComputeMinutesServed,
		ComputeMinutesRemaining: details.ComputeMinutesRemaining,
		CreatedAt:               details.CreatedAt,
	})

	common.ApiSuccess(c, data)
}

// DeleteDeployment 删除指定部署。
// 参数：
//   - c：当前请求上下文，用于读取 deployment ID 并提交删除请求。
func DeleteDeployment(c *gin.Context) {
	// 初始化企业版客户端并读取部署 ID。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	// 向 io.net 提交部署终止/删除请求。
	resp, err := client.DeleteDeployment(deploymentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回删除请求结果。
	data := gin.H{
		"status":        resp.Status,
		"deployment_id": resp.DeploymentID,
		"message":       "Deployment termination requested successfully",
	}
	common.ApiSuccess(c, data)
}

// CreateDeployment 创建新的 io.net 部署。
// 参数：
//   - c：当前请求上下文，用于读取部署请求体并返回创建结果。
func CreateDeployment(c *gin.Context) {
	// 初始化企业版客户端。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	// 绑定部署创建请求结构。
	var req ionet.DeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// 调用 io.net 创建部署。
	resp, err := client.DeployContainer(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回新部署的 ID 和创建状态。
	data := gin.H{
		"deployment_id": resp.DeploymentID,
		"status":        resp.Status,
		"message":       "Deployment created successfully",
	}
	common.ApiSuccess(c, data)
}

// GetHardwareTypes 获取可部署的硬件类型列表。
// 参数：
//   - c：当前请求上下文，用于返回硬件类型及可用量汇总。
func GetHardwareTypes(c *gin.Context) {
	// 初始化企业版客户端并查询硬件类型列表。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	hardwareTypes, totalAvailable, err := client.ListHardwareTypes()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回硬件类型列表、总数和总可用量。
	data := gin.H{
		"hardware_types":  hardwareTypes,
		"total":           len(hardwareTypes),
		"total_available": totalAvailable,
	}
	common.ApiSuccess(c, data)
}

// GetLocations 获取可用部署区域列表。
// 参数：
//   - c：当前请求上下文，用于返回区域列表信息。
func GetLocations(c *gin.Context) {
	// 初始化普通 io.net 客户端并查询区域列表。
	client, ok := getIoClient(c)
	if !ok {
		return
	}

	locationsResp, err := client.ListLocations()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 兼容 total 缺失时，退化为按返回数组长度计算总数。
	total := locationsResp.Total
	if total == 0 {
		total = len(locationsResp.Locations)
	}

	// 返回区域列表和总数。
	data := gin.H{
		"locations": locationsResp.Locations,
		"total":     total,
	}
	common.ApiSuccess(c, data)
}

// GetAvailableReplicas 获取指定硬件配置下可用的副本数量。
// 参数：
//   - c：当前请求上下文，用于读取 hardware_id 和 gpu_count 查询参数。
func GetAvailableReplicas(c *gin.Context) {
	// 初始化企业版客户端。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	// 读取硬件 ID 和 GPU 数量查询参数。
	hardwareIDStr := c.Query("hardware_id")
	gpuCountStr := c.Query("gpu_count")

	// hardware_id 是必填参数。
	if hardwareIDStr == "" {
		common.ApiErrorMsg(c, "hardware_id parameter is required")
		return
	}

	// 将 hardware_id 转为正整数。
	hardwareID, err := strconv.Atoi(hardwareIDStr)
	if err != nil || hardwareID <= 0 {
		common.ApiErrorMsg(c, "invalid hardware_id parameter")
		return
	}

	// GPU 数量默认为 1，若传入有效正整数则使用用户值。
	gpuCount := 1
	if gpuCountStr != "" {
		if parsed, err := strconv.Atoi(gpuCountStr); err == nil && parsed > 0 {
			gpuCount = parsed
		}
	}

	// 查询该硬件条件下当前可用副本数。
	replicas, err := client.GetAvailableReplicas(hardwareID, gpuCount)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, replicas)
}

// GetPriceEstimation 获取部署价格预估。
// 参数：
//   - c：当前请求上下文，用于读取价格预估请求体并返回结果。
func GetPriceEstimation(c *gin.Context) {
	// 初始化企业版客户端。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	// 绑定价格预估请求参数。
	var req ionet.PriceEstimationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	// 调用上游接口获取价格预估结果。
	priceResp, err := client.GetPriceEstimation(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, priceResp)
}

// CheckClusterNameAvailability 检查部署名称是否可用。
// 参数：
//   - c：当前请求上下文，用于读取 name 查询参数并返回可用性。
func CheckClusterNameAvailability(c *gin.Context) {
	// 初始化企业版客户端。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	// 读取并规范化待校验的部署名称。
	clusterName := strings.TrimSpace(c.Query("name"))
	if clusterName == "" {
		common.ApiErrorMsg(c, "name parameter is required")
		return
	}

	// 调用上游接口检查名称是否可用。
	available, err := client.CheckClusterNameAvailability(clusterName)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回名称可用性结果。
	data := gin.H{
		"available": available,
		"name":      clusterName,
	}
	common.ApiSuccess(c, data)
}

// GetDeploymentLogs 获取指定部署容器的日志内容。
// 参数：
//   - c：当前请求上下文，用于读取 deployment ID、container_id 以及日志筛选参数。
func GetDeploymentLogs(c *gin.Context) {
	// 初始化普通 io.net 客户端并提取部署 ID。
	client, ok := getIoClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	// container_id 是必填参数，其余参数用于控制日志级别、流、分页和跟随模式。
	containerID := c.Query("container_id")
	if containerID == "" {
		common.ApiErrorMsg(c, "container_id parameter is required")
		return
	}
	level := c.Query("level")
	stream := c.Query("stream")
	cursor := c.Query("cursor")
	limitStr := c.Query("limit")
	follow := c.Query("follow") == "true"

	// limit 默认 100，并设置最大上限防止单次拉取过大。
	var limit int = 100
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 1000 {
				limit = 1000
			}
		}
	}

	// 组装日志查询参数对象。
	opts := &ionet.GetLogsOptions{
		Level:  level,
		Stream: stream,
		Limit:  limit,
		Cursor: cursor,
		Follow: follow,
	}

	// 如果传入了开始或结束时间，则解析 RFC3339 时间后附加到请求中。
	if startTime := c.Query("start_time"); startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			opts.StartTime = &t
		}
	}
	if endTime := c.Query("end_time"); endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			opts.EndTime = &t
		}
	}

	// 从上游拉取原始容器日志数据。
	rawLogs, err := client.GetContainerLogsRaw(deploymentID, containerID, opts)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, rawLogs)
}

// ListDeploymentContainers 获取指定部署下的容器列表。
// 参数：
//   - c：当前请求上下文，用于读取 deployment ID 并返回容器列表。
func ListDeploymentContainers(c *gin.Context) {
	// 初始化企业版客户端并提取部署 ID。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	// 读取部署下的全部容器信息。
	containers, err := client.ListContainers(deploymentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 将容器及其事件列表映射成前端使用的统一响应结构。
	items := make([]map[string]interface{}, 0)
	if containers != nil {
		items = make([]map[string]interface{}, 0, len(containers.Workers))
		for _, ctr := range containers.Workers {
			// 先整理单个容器的事件时间线。
			events := make([]map[string]interface{}, 0, len(ctr.ContainerEvents))
			for _, event := range ctr.ContainerEvents {
				events = append(events, map[string]interface{}{
					"time":    event.Time.Unix(),
					"message": event.Message,
				})
			}

			items = append(items, map[string]interface{}{
				"container_id":       ctr.ContainerID,
				"device_id":          ctr.DeviceID,
				"status":             strings.ToLower(strings.TrimSpace(ctr.Status)),
				"hardware":           ctr.Hardware,
				"brand_name":         ctr.BrandName,
				"created_at":         ctr.CreatedAt.Unix(),
				"uptime_percent":     ctr.UptimePercent,
				"gpus_per_container": ctr.GPUsPerContainer,
				"public_url":         ctr.PublicURL,
				"events":             events,
			})
		}
	}

	// 返回容器总数以及容器明细列表。
	response := gin.H{
		"total":      0,
		"containers": items,
	}
	if containers != nil {
		response["total"] = containers.Total
	}

	common.ApiSuccess(c, response)
}

// GetContainerDetails 获取指定部署中某个容器的详细信息。
// 参数：
//   - c：当前请求上下文，用于读取 deployment ID 和 container ID 并返回详情。
func GetContainerDetails(c *gin.Context) {
	// 初始化企业版客户端并提取部署与容器 ID。
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	containerID, ok := requireContainerID(c)
	if !ok {
		return
	}

	// 读取指定容器详情；若上游返回空结果则按未找到处理。
	details, err := client.GetContainerDetails(deploymentID, containerID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if details == nil {
		common.ApiErrorMsg(c, "container details not found")
		return
	}

	// 整理容器事件时间线，便于前端直接展示。
	events := make([]map[string]interface{}, 0, len(details.ContainerEvents))
	for _, event := range details.ContainerEvents {
		events = append(events, map[string]interface{}{
			"time":    event.Time.Unix(),
			"message": event.Message,
		})
	}

	// 返回容器详情的统一响应结构。
	data := gin.H{
		"deployment_id":      deploymentID,
		"container_id":       details.ContainerID,
		"device_id":          details.DeviceID,
		"status":             strings.ToLower(strings.TrimSpace(details.Status)),
		"hardware":           details.Hardware,
		"brand_name":         details.BrandName,
		"created_at":         details.CreatedAt.Unix(),
		"uptime_percent":     details.UptimePercent,
		"gpus_per_container": details.GPUsPerContainer,
		"public_url":         details.PublicURL,
		"events":             events,
	}

	common.ApiSuccess(c, data)
}

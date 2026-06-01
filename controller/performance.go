package controller

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

// PerformanceStats 性能统计信息
type PerformanceStats struct {
	// 缓存统计
	CacheStats common.DiskCacheStats `json:"cache_stats"`
	// 系统内存统计
	MemoryStats MemoryStats `json:"memory_stats"`
	// 磁盘缓存目录信息
	DiskCacheInfo DiskCacheInfo `json:"disk_cache_info"`
	// 磁盘空间信息
	DiskSpaceInfo common.DiskSpaceInfo `json:"disk_space_info"`
	// 配置信息
	Config PerformanceConfig `json:"config"`
}

// MemoryStats 内存统计
type MemoryStats struct {
	// 已分配内存（字节）
	Alloc uint64 `json:"alloc"`
	// 总分配内存（字节）
	TotalAlloc uint64 `json:"total_alloc"`
	// 系统内存（字节）
	Sys uint64 `json:"sys"`
	// GC 次数
	NumGC uint32 `json:"num_gc"`
	// Goroutine 数量
	NumGoroutine int `json:"num_goroutine"`
}

// DiskCacheInfo 磁盘缓存目录信息
type DiskCacheInfo struct {
	// 缓存目录路径
	Path string `json:"path"`
	// 目录是否存在
	Exists bool `json:"exists"`
	// 文件数量
	FileCount int `json:"file_count"`
	// 总大小（字节）
	TotalSize int64 `json:"total_size"`
}

// PerformanceConfig 性能配置
type PerformanceConfig struct {
	// 是否启用磁盘缓存
	DiskCacheEnabled bool `json:"disk_cache_enabled"`
	// 磁盘缓存阈值（MB）
	DiskCacheThresholdMB int `json:"disk_cache_threshold_mb"`
	// 磁盘缓存最大大小（MB）
	DiskCacheMaxSizeMB int `json:"disk_cache_max_size_mb"`
	// 磁盘缓存路径
	DiskCachePath string `json:"disk_cache_path"`
	// 是否在容器中运行
	IsRunningInContainer bool `json:"is_running_in_container"`

	// MonitorEnabled 是否启用性能监控
	MonitorEnabled bool `json:"monitor_enabled"`
	// MonitorCPUThreshold CPU 使用率阈值（%）
	MonitorCPUThreshold int `json:"monitor_cpu_threshold"`
	// MonitorMemoryThreshold 内存使用率阈值（%）
	MonitorMemoryThreshold int `json:"monitor_memory_threshold"`
	// MonitorDiskThreshold 磁盘使用率阈值（%）
	MonitorDiskThreshold int `json:"monitor_disk_threshold"`
}

// GetPerformanceStats 获取系统性能与缓存统计信息。
// 参数：
//   - c：当前请求上下文，用于返回缓存、内存、磁盘和配置统计。
func GetPerformanceStats(c *gin.Context) {
	// 不再每次获取统计都全量扫描磁盘，依赖原子计数器保证性能
	// 仅在系统启动或显式清理时同步
	cacheStats := common.GetDiskCacheStats()

	// 读取 Go 运行时内存统计信息。
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 获取磁盘缓存目录的文件数量和空间占用。
	diskCacheInfo := getDiskCacheInfo()

	// 汇总磁盘缓存与性能监控相关配置，便于前端统一展示。
	diskConfig := common.GetDiskCacheConfig()
	monitorConfig := common.GetPerformanceMonitorConfig()
	config := PerformanceConfig{
		DiskCacheEnabled:       diskConfig.Enabled,
		DiskCacheThresholdMB:   diskConfig.ThresholdMB,
		DiskCacheMaxSizeMB:     diskConfig.MaxSizeMB,
		DiskCachePath:          diskConfig.Path,
		IsRunningInContainer:   common.IsRunningInContainer(),
		MonitorEnabled:         monitorConfig.Enabled,
		MonitorCPUThreshold:    monitorConfig.CPUThreshold,
		MonitorMemoryThreshold: monitorConfig.MemoryThreshold,
		MonitorDiskThreshold:   monitorConfig.DiskThreshold,
	}

	// 先读取缓存中的系统状态，作为磁盘使用率的轻量来源。
	systemStatus := common.GetSystemStatus()
	diskSpaceInfo := common.DiskSpaceInfo{
		UsedPercent: systemStatus.DiskUsage,
	}
	// 如果需要详细信息，可以按需获取，或者扩展 SystemStatus
	// 这里为了保持接口兼容性，我们仍然调用 GetDiskSpaceInfo，但注意这可能会有性能开销
	// 考虑到 GetPerformanceStats 是管理接口，频率较低，直接调用是可以接受的
	// 但为了一致性，我们也可以考虑从 SystemStatus 中获取部分信息
	diskSpaceInfo = common.GetDiskSpaceInfo()

	// 组装最终返回的性能统计对象。
	stats := PerformanceStats{
		CacheStats: cacheStats,
		MemoryStats: MemoryStats{
			Alloc:        memStats.Alloc,
			TotalAlloc:   memStats.TotalAlloc,
			Sys:          memStats.Sys,
			NumGC:        memStats.NumGC,
			NumGoroutine: runtime.NumGoroutine(),
		},
		DiskCacheInfo: diskCacheInfo,
		DiskSpaceInfo: diskSpaceInfo,
		Config:        config,
	}

	// 返回当前性能统计快照。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// ClearDiskCache 清理长时间未使用的磁盘缓存文件。
// 参数：
//   - c：当前请求上下文，用于返回清理结果。
func ClearDiskCache(c *gin.Context) {
	// 清理超过 10 分钟未使用的缓存文件
	// 10 分钟是一个安全的阈值，确保正在进行的请求不会被误删
	err := common.CleanupOldDiskCacheFiles(10 * time.Minute)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 返回磁盘缓存清理成功结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "不活跃的磁盘缓存已清理",
	})
}

// ResetPerformanceStats 重置磁盘缓存相关的性能统计计数器。
// 参数：
//   - c：当前请求上下文，用于返回重置结果。
func ResetPerformanceStats(c *gin.Context) {
	// 清空磁盘缓存统计累计值。
	common.ResetDiskCacheStats()

	// 返回统计重置成功结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "统计信息已重置",
	})
}

// ForceGC 强制触发一次 Go 垃圾回收。
// 参数：
//   - c：当前请求上下文，用于返回执行结果。
func ForceGC(c *gin.Context) {
	// 直接调用运行时 GC。
	runtime.GC()

	// 返回 GC 执行成功结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "GC 已执行",
	})
}

// LogFileInfo 表示单个日志文件的基础信息。
type LogFileInfo struct {
	Name    string    `json:"name"`     // 日志文件名。
	Size    int64     `json:"size"`     // 文件大小，单位字节。
	ModTime time.Time `json:"mod_time"` // 最后修改时间。
}

// LogFilesResponse 表示日志文件列表接口的响应结构。
type LogFilesResponse struct {
	LogDir     string        `json:"log_dir"`               // 日志目录路径。
	Enabled    bool          `json:"enabled"`               // 日志目录功能是否启用。
	FileCount  int           `json:"file_count"`            // 日志文件数量。
	TotalSize  int64         `json:"total_size"`            // 日志文件总大小。
	OldestTime *time.Time    `json:"oldest_time,omitempty"` // 最旧日志时间。
	NewestTime *time.Time    `json:"newest_time,omitempty"` // 最新日志时间。
	Files      []LogFileInfo `json:"files"`                 // 日志文件列表。
}

// getLogFiles 读取日志目录中的日志文件列表。
//
// 返回：
//   - []LogFileInfo：符合命名规则的日志文件列表。
//   - error：读取目录失败时返回错误。
func getLogFiles() ([]LogFileInfo, error) {
	// 未配置日志目录时视为日志功能未启用。
	if *common.LogDir == "" {
		return nil, nil
	}
	// 枚举日志目录中的全部文件项。
	entries, err := os.ReadDir(*common.LogDir)
	if err != nil {
		return nil, err
	}
	var files []LogFileInfo
	for _, entry := range entries {
		// 仅处理符合 oneapi-*.log 命名约定的普通文件。
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "oneapi-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, LogFileInfo{
			Name:    name,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	// 按文件名降序排列（最新在前）
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name > files[j].Name
	})
	return files, nil
}

// GetLogFiles 获取日志文件列表及统计信息。
// 参数：
//   - c：当前请求上下文，用于返回日志目录状态和文件清单。
func GetLogFiles(c *gin.Context) {
	// 未配置日志目录时直接返回未启用状态。
	if *common.LogDir == "" {
		common.ApiSuccess(c, LogFilesResponse{Enabled: false})
		return
	}
	// 读取日志文件列表。
	files, err := getLogFiles()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 统计日志文件总大小、最早时间和最新时间。
	var totalSize int64
	var oldest, newest time.Time
	for i, f := range files {
		totalSize += f.Size
		if i == 0 || f.ModTime.Before(oldest) {
			oldest = f.ModTime
		}
		if i == 0 || f.ModTime.After(newest) {
			newest = f.ModTime
		}
	}
	// 组装基础响应结构。
	resp := LogFilesResponse{
		LogDir:    *common.LogDir,
		Enabled:   true,
		FileCount: len(files),
		TotalSize: totalSize,
		Files:     files,
	}
	// 仅在存在日志文件时填充时间边界信息。
	if len(files) > 0 {
		resp.OldestTime = &oldest
		resp.NewestTime = &newest
	}
	// 返回日志文件清单和统计信息。
	common.ApiSuccess(c, resp)
}

// CleanupLogFiles 按保留数量或保留天数清理历史日志文件。
// 参数：
//   - c：当前请求上下文，用于读取清理模式和阈值。
func CleanupLogFiles(c *gin.Context) {
	// 读取清理模式和阈值参数。
	mode := c.Query("mode")
	valueStr := c.Query("value")
	// 仅支持按数量或按天数两种清理模式。
	if mode != "by_count" && mode != "by_days" {
		common.ApiErrorMsg(c, "invalid mode, must be by_count or by_days")
		return
	}
	// 清理阈值必须是正整数。
	value, err := strconv.Atoi(valueStr)
	if err != nil || value < 1 {
		common.ApiErrorMsg(c, "invalid value, must be a positive integer")
		return
	}
	// 没有配置日志目录时无法执行清理。
	if *common.LogDir == "" {
		common.ApiErrorMsg(c, "log directory not configured")
		return
	}

	// 获取当前日志文件列表。
	files, err := getLogFiles()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 当前正在写入的日志文件需要始终保留，避免误删活动日志。
	activeLogPath := logger.GetCurrentLogPath()
	var toDelete []LogFileInfo

	switch mode {
	case "by_count":
		// files 已按名称降序（最新在前），保留前 value 个
		// 超出保留数量的历史文件加入删除列表，但跳过当前活动日志。
		for i, f := range files {
			if i < value {
				continue
			}
			fullPath := filepath.Join(*common.LogDir, f.Name)
			if fullPath == activeLogPath {
				continue
			}
			toDelete = append(toDelete, f)
		}
	case "by_days":
		// 计算保留天数截止时间，并筛出更早的日志文件。
		cutoff := time.Now().AddDate(0, 0, -value)
		for _, f := range files {
			if f.ModTime.Before(cutoff) {
				fullPath := filepath.Join(*common.LogDir, f.Name)
				if fullPath == activeLogPath {
					continue
				}
				toDelete = append(toDelete, f)
			}
		}
	}

	// 逐个删除目标日志文件，并统计释放空间与失败项。
	var deletedCount int
	var freedBytes int64
	var failedFiles []string
	for _, f := range toDelete {
		fullPath := filepath.Join(*common.LogDir, f.Name)
		if err := os.Remove(fullPath); err != nil {
			failedFiles = append(failedFiles, f.Name)
			continue
		}
		deletedCount++
		freedBytes += f.Size
	}

	// 组织清理结果数据。
	result := gin.H{
		"deleted_count": deletedCount,
		"freed_bytes":   freedBytes,
		"failed_files":  failedFiles,
	}

	// 若存在删除失败的文件，则以 success=false 返回部分成功结果。
	if len(failedFiles) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("部分文件删除失败（%d/%d）", len(failedFiles), len(toDelete)),
			"data":    result,
		})
		return
	}

	// 全部删除成功时返回成功结果。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

// getDiskCacheInfo 获取磁盘缓存目录中的文件数量和空间占用信息。
//
// 返回：
//   - DiskCacheInfo：缓存目录状态与体积统计信息。
func getDiskCacheInfo() DiskCacheInfo {
	// 使用统一的缓存目录
	dir := common.GetDiskCacheDir()

	// 先构造默认返回值；目录不存在时直接返回。
	info := DiskCacheInfo{
		Path:   dir,
		Exists: false,
	}

	// 尝试读取缓存目录内容；失败时返回默认结构。
	entries, err := os.ReadDir(dir)
	if err != nil {
		return info
	}

	// 目录存在时开始统计文件数量与总大小。
	info.Exists = true
	info.FileCount = 0
	info.TotalSize = 0

	for _, entry := range entries {
		// 仅统计普通文件，忽略子目录。
		if entry.IsDir() {
			continue
		}
		info.FileCount++
		if fileInfo, err := entry.Info(); err == nil {
			info.TotalSize += fileInfo.Size()
		}
	}

	return info
}

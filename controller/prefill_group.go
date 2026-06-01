package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetPrefillGroups 获取预填组列表，可通过 `?type=xxx` 进行类型过滤。
// 参数：
//   - c：当前请求上下文，用于读取类型筛选参数并返回预填组列表。
func GetPrefillGroups(c *gin.Context) {
	// 读取可选的类型筛选参数，并查询对应预填组列表。
	groupType := c.Query("type")
	groups, err := model.GetAllPrefillGroups(groupType)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回预填组查询结果。
	common.ApiSuccess(c, groups)
}

// CreatePrefillGroup 创建新的预填组。
// 参数：
//   - c：当前请求上下文，用于读取预填组参数并返回创建结果。
func CreatePrefillGroup(c *gin.Context) {
	// 绑定请求体到预填组模型结构。
	var g model.PrefillGroup
	if err := c.ShouldBindJSON(&g); err != nil {
		common.ApiError(c, err)
		return
	}
	// 组名称和类型都是创建时的必填字段。
	if g.Name == "" || g.Type == "" {
		common.ApiErrorMsg(c, "组名称和类型不能为空")
		return
	}
	// 创建前检查名称
	// 在写入前检查同名预填组是否已存在，避免重复创建。
	if dup, err := model.IsPrefillGroupNameDuplicated(0, g.Name); err != nil {
		common.ApiError(c, err)
		return
	} else if dup {
		common.ApiErrorMsg(c, "组名称已存在")
		return
	}

	// 插入新预填组记录。
	if err := g.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回创建后的预填组数据。
	common.ApiSuccess(c, &g)
}

// UpdatePrefillGroup 更新已有预填组。
// 参数：
//   - c：当前请求上下文，用于读取预填组更新内容并返回结果。
func UpdatePrefillGroup(c *gin.Context) {
	// 绑定请求体到预填组模型结构。
	var g model.PrefillGroup
	if err := c.ShouldBindJSON(&g); err != nil {
		common.ApiError(c, err)
		return
	}
	// 更新时必须携带目标组 ID。
	if g.Id == 0 {
		common.ApiErrorMsg(c, "缺少组 ID")
		return
	}
	// 名称冲突检查
	// 更新前检查目标名称是否与其他预填组冲突。
	if dup, err := model.IsPrefillGroupNameDuplicated(g.Id, g.Name); err != nil {
		common.ApiError(c, err)
		return
	} else if dup {
		common.ApiErrorMsg(c, "组名称已存在")
		return
	}

	// 执行预填组更新操作。
	if err := g.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回更新后的预填组数据。
	common.ApiSuccess(c, &g)
}

// DeletePrefillGroup 删除指定的预填组。
// 参数：
//   - c：当前请求上下文，用于读取路径中的组 ID 并执行删除。
func DeletePrefillGroup(c *gin.Context) {
	// 解析路径中的预填组 ID。
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 按 ID 删除预填组记录。
	if err := model.DeletePrefillGroupByID(id); err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回删除成功结果。
	common.ApiSuccess(c, nil)
}

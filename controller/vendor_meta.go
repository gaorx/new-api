package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetAllVendors 分页获取供应商列表。
// 参数：
//   - c：当前请求上下文，用于读取分页参数并返回供应商列表。
func GetAllVendors(c *gin.Context) {
	// 查询供应商分页数据。
	pageInfo := common.GetPageQuery(c)
	vendors, err := model.GetAllVendors(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 统计总数并写入分页对象后返回。
	var total int64
	model.DB.Model(&model.Vendor{}).Count(&total)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(vendors)
	common.ApiSuccess(c, pageInfo)
}

// SearchVendors 按关键字搜索供应商。
// 参数：
//   - c：当前请求上下文，用于读取关键字和分页参数。
func SearchVendors(c *gin.Context) {
	// 读取关键字并执行分页搜索。
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)
	vendors, total, err := model.SearchVendors(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(vendors)
	common.ApiSuccess(c, pageInfo)
}

// GetVendorMeta 根据 ID 获取单个供应商详情。
// 参数：
//   - c：当前请求上下文，用于读取供应商 ID 并返回详情。
func GetVendorMeta(c *gin.Context) {
	// 解析路径中的供应商 ID。
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	v, err := model.GetVendorByID(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回供应商详情。
	common.ApiSuccess(c, v)
}

// CreateVendorMeta 新建供应商。
// 参数：
//   - c：当前请求上下文，用于读取供应商数据并返回创建结果。
func CreateVendorMeta(c *gin.Context) {
	// 绑定请求体到供应商模型结构。
	var v model.Vendor
	if err := c.ShouldBindJSON(&v); err != nil {
		common.ApiError(c, err)
		return
	}
	// 供应商名称是创建时的必填字段。
	if v.Name == "" {
		common.ApiErrorMsg(c, "供应商名称不能为空")
		return
	}
	// 创建前先检查名称
	// 写入前先检查名称是否与现有供应商冲突。
	if dup, err := model.IsVendorNameDuplicated(0, v.Name); err != nil {
		common.ApiError(c, err)
		return
	} else if dup {
		common.ApiErrorMsg(c, "供应商名称已存在")
		return
	}

	// 插入供应商记录。
	if err := v.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回创建后的供应商数据。
	common.ApiSuccess(c, &v)
}

// UpdateVendorMeta 更新已有供应商。
// 参数：
//   - c：当前请求上下文，用于读取供应商更新内容并返回结果。
func UpdateVendorMeta(c *gin.Context) {
	// 绑定请求体到供应商模型结构。
	var v model.Vendor
	if err := c.ShouldBindJSON(&v); err != nil {
		common.ApiError(c, err)
		return
	}
	// 更新时必须携带供应商 ID。
	if v.Id == 0 {
		common.ApiErrorMsg(c, "缺少供应商 ID")
		return
	}
	// 名称冲突检查
	// 更新前检查目标名称是否与其他供应商重复。
	if dup, err := model.IsVendorNameDuplicated(v.Id, v.Name); err != nil {
		common.ApiError(c, err)
		return
	} else if dup {
		common.ApiErrorMsg(c, "供应商名称已存在")
		return
	}

	// 保存更新后的供应商记录。
	if err := v.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回更新后的供应商数据。
	common.ApiSuccess(c, &v)
}

// DeleteVendorMeta 删除指定供应商。
// 参数：
//   - c：当前请求上下文，用于读取供应商 ID 并执行删除。
func DeleteVendorMeta(c *gin.Context) {
	// 解析路径中的供应商 ID。
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 直接按主键删除供应商记录。
	if err := model.DB.Delete(&model.Vendor{}, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	// 返回删除成功结果。
	common.ApiSuccess(c, nil)
}

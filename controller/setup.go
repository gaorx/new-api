package controller

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// Setup 表示系统初始化状态查询接口的响应结构。
type Setup struct {
	Status       bool   `json:"status"`        // 系统是否已经完成初始化。
	RootInit     bool   `json:"root_init"`     // Root 用户是否已经存在。
	DatabaseType string `json:"database_type"` // 当前使用的数据库类型。
}

// SetupRequest 表示系统初始化接口的请求体。
type SetupRequest struct {
	Username           string `json:"username"`           // 初始化时创建的管理员用户名。
	Password           string `json:"password"`           // 初始化时设置的管理员密码。
	ConfirmPassword    string `json:"confirmPassword"`    // 确认密码。
	SelfUseModeEnabled bool   `json:"SelfUseModeEnabled"` // 是否启用自用模式。
	DemoSiteEnabled    bool   `json:"DemoSiteEnabled"`    // 是否启用演示站模式。
}

// GetSetup 获取当前系统初始化状态。
// 参数：
//   - c：当前请求上下文，用于返回初始化状态和数据库类型。
func GetSetup(c *gin.Context) {
	// 先构造基础响应对象，包含系统全局初始化标志。
	setup := Setup{
		Status: constant.Setup,
	}
	// 若系统已经初始化完成，则直接返回当前状态。
	if constant.Setup {
		c.JSON(200, gin.H{
			"success": true,
			"data":    setup,
		})
		return
	}
	// 系统尚未初始化时，补充 root 用户存在性和当前数据库类型信息。
	setup.RootInit = model.RootUserExists()
	if common.UsingMySQL {
		setup.DatabaseType = "mysql"
	}
	if common.UsingPostgreSQL {
		setup.DatabaseType = "postgres"
	}
	if common.UsingSQLite {
		setup.DatabaseType = "sqlite"
	}
	// 返回初始化前的系统状态概览。
	c.JSON(200, gin.H{
		"success": true,
		"data":    setup,
	})
}

// PostSetup 执行系统初始化，必要时创建 root 用户并写入初始化配置。
// 参数：
//   - c：当前请求上下文，用于读取初始化参数并返回初始化结果。
func PostSetup(c *gin.Context) {
	// Check if setup is already completed
	// 如果系统已经初始化完成，则不允许再次执行初始化。
	if constant.Setup {
		c.JSON(200, gin.H{
			"success": false,
			"message": "系统已经初始化完成",
		})
		return
	}

	// Check if root user already exists
	// 检查 root 用户是否已经存在，存在时仅更新初始化配置而不重复建号。
	rootExists := model.RootUserExists()

	// 解析初始化请求体。
	var req SetupRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": "请求参数有误",
		})
		return
	}

	// If root doesn't exist, validate and create admin account
	// 当 root 用户不存在时，先校验用户名和密码，再创建管理员账号。
	if !rootExists {
		// Validate username length: max 12 characters to align with model.User validation
		if len(req.Username) > 12 {
			c.JSON(200, gin.H{
				"success": false,
				"message": "用户名长度不能超过12个字符",
			})
			return
		}
		// Validate password
		if req.Password != req.ConfirmPassword {
			c.JSON(200, gin.H{
				"success": false,
				"message": "两次输入的密码不一致",
			})
			return
		}

		if len(req.Password) < 8 {
			c.JSON(200, gin.H{
				"success": false,
				"message": "密码长度至少为8个字符",
			})
			return
		}

		// Create root user
		// 对密码做哈希处理后创建 root 用户记录。
		hashedPassword, err := common.Password2Hash(req.Password)
		if err != nil {
			c.JSON(200, gin.H{
				"success": false,
				"message": "系统错误: " + err.Error(),
			})
			return
		}
		rootUser := model.User{
			Username:    req.Username,
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		err = model.DB.Create(&rootUser).Error
		if err != nil {
			c.JSON(200, gin.H{
				"success": false,
				"message": "创建管理员账号失败: " + err.Error(),
			})
			return
		}
	}

	// Set operation modes
	// 将自用模式和演示站模式写入内存配置。
	operation_setting.SelfUseModeEnabled = req.SelfUseModeEnabled
	operation_setting.DemoSiteEnabled = req.DemoSiteEnabled

	// Save operation modes to database for persistence
	// 再把运行模式持久化到数据库，确保重启后仍然生效。
	err = model.UpdateOption("SelfUseModeEnabled", boolToString(req.SelfUseModeEnabled))
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": "保存自用模式设置失败: " + err.Error(),
		})
		return
	}

	err = model.UpdateOption("DemoSiteEnabled", boolToString(req.DemoSiteEnabled))
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": "保存演示站点模式设置失败: " + err.Error(),
		})
		return
	}

	// Update setup status
	// 标记系统初始化完成，并记录初始化版本与时间。
	constant.Setup = true

	setup := model.Setup{
		Version:       common.Version,
		InitializedAt: time.Now().Unix(),
	}
	err = model.DB.Create(&setup).Error
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": "系统初始化失败: " + err.Error(),
		})
		return
	}

	// 返回系统初始化成功结果。
	c.JSON(200, gin.H{
		"success": true,
		"message": "系统初始化成功",
	})
}

// boolToString 将布尔值转换为配置持久化使用的字符串表示。
// 参数：
//   - b：待转换布尔值。
//
// 返回：
//   - string：`"true"` 或 `"false"`。
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

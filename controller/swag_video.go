package controller

import (
	"github.com/gin-gonic/gin"
)

// VideoGenerations
// @Summary 生成视频
// @Description 调用视频生成接口生成视频
// @Description 支持多种视频生成服务：
// @Description - 可灵AI (Kling): https://app.klingai.com/cn/dev/document-api/apiReference/commonInfo
// @Description - 即梦 (Jimeng): https://www.volcengine.com/docs/85621/1538636
// @Tags Video
// @Accept json
// @Produce json
// @Param Authorization header string true "用户认证令牌 (Aeess-Token: sk-xxxx)"
// @Param request body dto.VideoRequest true "视频生成请求参数"
// @Failure 400 {object} dto.OpenAIError "请求参数错误"
// @Failure 401 {object} dto.OpenAIError "未授权"
// @Failure 403 {object} dto.OpenAIError "无权限"
// @Failure 500 {object} dto.OpenAIError "服务器内部错误"
// @Router /v1/video/generations [post]
func VideoGenerations(c *gin.Context) {
}

// VideoGenerationsTaskId
// @Summary 查询视频
// @Description 根据任务ID查询视频生成任务的状态和结果
// @Tags Video
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param task_id path string true "Task ID"
// @Success 200 {object} dto.VideoTaskResponse "任务状态和结果"
// @Failure 400 {object} dto.OpenAIError "请求参数错误"
// @Failure 401 {object} dto.OpenAIError "未授权"
// @Failure 403 {object} dto.OpenAIError "无权限"
// @Failure 500 {object} dto.OpenAIError "服务器内部错误"
// @Router /v1/video/generations/{task_id} [get]
func VideoGenerationsTaskId(c *gin.Context) {
}

// KlingText2VideoGenerations
// @Summary 可灵文生视频
// @Description 调用可灵AI文生视频接口，生成视频内容
// @Tags Video
// @Accept json
// @Produce json
// @Param Authorization header string true "用户认证令牌 (Aeess-Token: sk-xxxx)"
// @Param request body KlingText2VideoRequest true "视频生成请求参数"
// @Success 200 {object} dto.VideoTaskResponse "任务状态和结果"
// @Failure 400 {object} dto.OpenAIError "请求参数错误"
// @Failure 401 {object} dto.OpenAIError "未授权"
// @Failure 403 {object} dto.OpenAIError "无权限"
// @Failure 500 {object} dto.OpenAIError "服务器内部错误"
// @Router /kling/v1/videos/text2video [post]
func KlingText2VideoGenerations(c *gin.Context) {
}

// KlingText2VideoRequest 表示可灵文生视频请求体。
type KlingText2VideoRequest struct {
	ModelName      string              `json:"model_name,omitempty" example:"kling-v1"`                                  // 可灵模型名。
	Prompt         string              `json:"prompt" binding:"required" example:"A cat playing piano in the garden"`   // 正向提示词。
	NegativePrompt string              `json:"negative_prompt,omitempty" example:"blurry, low quality"`                  // 负向提示词。
	CfgScale       float64             `json:"cfg_scale,omitempty" example:"0.7"`                                        // 提示词引导强度。
	Mode           string              `json:"mode,omitempty" example:"std"`                                              // 生成模式。
	CameraControl  *KlingCameraControl `json:"camera_control,omitempty"`                                                  // 镜头控制配置。
	AspectRatio    string              `json:"aspect_ratio,omitempty" example:"16:9"`                                    // 画面比例。
	Duration       string              `json:"duration,omitempty" example:"5"`                                            // 视频时长。
	CallbackURL    string              `json:"callback_url,omitempty" example:"https://your.domain/callback"`            // 异步回调地址。
	ExternalTaskId string              `json:"external_task_id,omitempty" example:"custom-task-001"`                     // 外部任务 ID。
}

// KlingCameraControl 表示可灵视频生成中的镜头控制配置。
type KlingCameraControl struct {
	Type   string             `json:"type,omitempty" example:"simple"` // 镜头控制类型。
	Config *KlingCameraConfig `json:"config,omitempty"`                // 镜头控制参数。
}

// KlingCameraConfig 表示可灵镜头移动参数。
type KlingCameraConfig struct {
	Horizontal float64 `json:"horizontal,omitempty" example:"2.5"` // 水平移动量。
	Vertical   float64 `json:"vertical,omitempty" example:"0"`     // 垂直移动量。
	Pan        float64 `json:"pan,omitempty" example:"0"`          // 平移量。
	Tilt       float64 `json:"tilt,omitempty" example:"0"`         // 俯仰量。
	Roll       float64 `json:"roll,omitempty" example:"0"`         // 翻滚量。
	Zoom       float64 `json:"zoom,omitempty" example:"0"`         // 缩放量。
}

// KlingImage2VideoGenerations
// @Summary 可灵官方-图生视频
// @Description 调用可灵AI图生视频接口，生成视频内容
// @Tags Video
// @Accept json
// @Produce json
// @Param Authorization header string true "用户认证令牌 (Aeess-Token: sk-xxxx)"
// @Param request body KlingImage2VideoRequest true "图生视频请求参数"
// @Success 200 {object} dto.VideoTaskResponse "任务状态和结果"
// @Failure 400 {object} dto.OpenAIError "请求参数错误"
// @Failure 401 {object} dto.OpenAIError "未授权"
// @Failure 403 {object} dto.OpenAIError "无权限"
// @Failure 500 {object} dto.OpenAIError "服务器内部错误"
// @Router /kling/v1/videos/image2video [post]
func KlingImage2VideoGenerations(c *gin.Context) {
}

// KlingImage2VideoRequest 表示可灵图生视频请求体。
type KlingImage2VideoRequest struct {
	ModelName      string              `json:"model_name,omitempty" example:"kling-v2-master"`                                                                 // 可灵模型名。
	Image          string              `json:"image" binding:"required" example:"https://h2.inkwai.com/bs2/upload-ylab-stunt/se/ai_portal_queue_mmu_image_upscale_aiweb/3214b798-e1b4-4b00-b7af-72b5b0417420_raw_image_0.jpg"` // 输入图片。
	Prompt         string              `json:"prompt,omitempty" example:"A cat playing piano in the garden"`                                                    // 正向提示词。
	NegativePrompt string              `json:"negative_prompt,omitempty" example:"blurry, low quality"`                                                           // 负向提示词。
	CfgScale       float64             `json:"cfg_scale,omitempty" example:"0.7"`                                                                                 // 提示词引导强度。
	Mode           string              `json:"mode,omitempty" example:"std"`                                                                                       // 生成模式。
	CameraControl  *KlingCameraControl `json:"camera_control,omitempty"`                                                                                           // 镜头控制配置。
	AspectRatio    string              `json:"aspect_ratio,omitempty" example:"16:9"`                                                                             // 画面比例。
	Duration       string              `json:"duration,omitempty" example:"5"`                                                                                     // 视频时长。
	CallbackURL    string              `json:"callback_url,omitempty" example:"https://your.domain/callback"`                                                     // 异步回调地址。
	ExternalTaskId string              `json:"external_task_id,omitempty" example:"custom-task-002"`                                                              // 外部任务 ID。
}

// KlingImage2videoTaskId godoc
// @Summary 可灵任务查询--图生视频
// @Description Query the status and result of a Kling video generation task by task ID
// @Tags Origin
// @Accept json
// @Produce json
// @Param task_id path string true "Task ID"
// @Router /kling/v1/videos/image2video/{task_id} [get]
// KlingImage2videoTaskId 预留的可灵图生视频任务查询接口占位实现。
// 参数：
//   - c：当前请求上下文。
func KlingImage2videoTaskId(c *gin.Context) {}

// KlingText2videoTaskId godoc
// @Summary 可灵任务查询--文生视频
// @Description Query the status and result of a Kling text-to-video generation task by task ID
// @Tags Origin
// @Accept json
// @Produce json
// @Param task_id path string true "Task ID"
// @Router /kling/v1/videos/text2video/{task_id} [get]
// KlingText2videoTaskId 预留的可灵文生视频任务查询接口占位实现。
// 参数：
//   - c：当前请求上下文。
func KlingText2videoTaskId(c *gin.Context) {}

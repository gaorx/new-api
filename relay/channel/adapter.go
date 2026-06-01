package channel

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type Adaptor interface {
	// Init 在一次请求开始时初始化适配器及相关上下文状态。
	Init(info *relaycommon.RelayInfo)
	// GetRequestURL 根据当前渠道和 RelayInfo 生成最终上游请求地址。
	GetRequestURL(info *relaycommon.RelayInfo) (string, error)
	// SetupRequestHeader 为本次上游请求写入认证和协议相关请求头。
	SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error
	// ConvertOpenAIRequest 将 OpenAI 文本请求转换为当前渠道可接受的请求结构。
	ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error)
	// ConvertRerankRequest 将重排请求转换为当前渠道可接受的请求结构。
	ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error)
	// ConvertEmbeddingRequest 将 embedding 请求转换为当前渠道可接受的请求结构。
	ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error)
	// ConvertAudioRequest 将音频请求转换为当前渠道需要的请求体。
	ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error)
	// ConvertImageRequest 将图片请求转换为当前渠道可接受的请求结构。
	ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error)
	// ConvertOpenAIResponsesRequest 将 OpenAI Responses 请求转换为当前渠道可接受的请求结构。
	ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error)
	// DoRequest 发起上游请求并返回原始响应对象。
	DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error)
	// DoResponse 解析上游响应、回写客户端，并返回 usage 或统一错误。
	DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError)
	// GetModelList 返回当前渠道声明支持的模型列表。
	GetModelList() []string
	// GetChannelName 返回当前渠道的名称标识。
	GetChannelName() string
	// ConvertClaudeRequest 将 Claude 请求转换为当前渠道可接受的请求结构。
	ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error)
	// ConvertGeminiRequest 将 Gemini 请求转换为当前渠道可接受的请求结构。
	ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error)
}

type TaskAdaptor interface {
	// Init 在异步任务请求开始时初始化适配器及相关上下文状态。
	Init(info *relaycommon.RelayInfo)

	// ValidateRequestAndSetAction 校验任务请求并确定本次任务动作类型。
	ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError

	// ── Billing ──────────────────────────────────────────────────────

	// EstimateBilling 根据用户请求估算预扣费所需的额外倍率参数。
	// 它在 ValidateRequestAndSetAction 之后、价格计算之前调用。
	// 适配器应从已解析请求中提取时长、分辨率等信息，
	// 并以倍率形式返回，例如 {"seconds": 5, "size": 1.666}。
	// 返回 nil 表示只使用模型基础价格，不叠加额外倍率。
	EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64

	// AdjustBillingOnSubmit 根据上游提交响应返回修正后的倍率参数。
	// 它在 DoResponse 成功之后调用。
	// 如果上游返回的实际参数与预估值不同，例如实际时长发生变化，
	// 则返回新的倍率供调用方重新计算额度，并与预扣费进行差额结算。
	// 返回 nil 表示无需调整。
	AdjustBillingOnSubmit(info *relaycommon.RelayInfo, taskData []byte) map[string]float64

	// AdjustBillingOnComplete 在轮询到任务终态（成功或失败）时返回实际应结算额度。
	// 它由轮询流程在 ParseTaskResult 之后调用。
	// 返回正数会触发差额结算，例如补扣或退款。
	// 返回 0 表示保持预扣额度不变。
	AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int

	// ── Request / Response ───────────────────────────────────────────

	// BuildRequestURL 生成任务提交接口的上游地址。
	BuildRequestURL(info *relaycommon.RelayInfo) (string, error)
	// BuildRequestHeader 为任务提交请求构造请求头。
	BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error
	// BuildRequestBody 构造任务提交所需的请求体。
	BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error)

	// DoRequest 发起任务提交请求并返回上游原始响应。
	DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error)
	// DoResponse 解析任务提交响应并提取任务 ID、任务数据或错误。
	DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, err *dto.TaskError)

	// GetModelList 返回当前任务渠道支持的模型列表。
	GetModelList() []string
	// GetChannelName 返回当前任务渠道的名称标识。
	GetChannelName() string

	// ── Polling ──────────────────────────────────────────────────────

	// FetchTask 按任务 ID 或查询参数拉取任务最新状态。
	FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error)
	// ParseTaskResult 解析任务查询结果为统一任务状态结构。
	ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error)
}

type OpenAIVideoConverter interface {
	// ConvertToOpenAIVideo 将异步任务结果转换为 OpenAI 风格视频响应。
	ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error)
}

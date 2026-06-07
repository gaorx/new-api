# Relay Provider Error Shapes

本文记录 Relay 侧常见上游错误的原始表达形式，以及本项目如何将它们归一化。

重点覆盖：

- OpenAI / OpenAI-compatible
- Anthropic / Claude
- Gemini
- 若干项目中有独立错误 DTO 或专门错误处理逻辑的下游渠道

说明：

- 以下 JSON 例子主要依据仓库中的 DTO、handler 和错误提取逻辑整理，目的是帮助阅读代码和定位兼容差异。
- 这些例子是“结构示意”，不保证与各家线上文档逐字一致。

## 1. 项目内的统一错误模型

Relay 运行时最终会收敛到两种主要错误外观：

1. OpenAI 风格

```json
{
  "error": {
    "message": "xxx",
    "type": "xxx",
    "param": "",
    "code": "xxx"
  }
}
```

2. Claude 风格

```json
{
  "type": "error",
  "error": {
    "type": "xxx",
    "message": "xxx"
  }
}
```

核心类型定义见：

- [types/error.go](../types/error.go)

统一错误兜底入口见：

- [service/error.go](../service/error.go)
- [dto/error.go](../dto/error.go)

`service.RelayErrorHandler(...)` 会优先尝试解析：

- `error`
- `message`
- `msg`
- `err`
- `error_msg`
- `detail`
- `header.message`
- `response.error.message`

如果 `error` 是一个对象，项目会优先尝试把它当作 OpenAI 风格错误解析。

## 2. OpenAI / OpenAI-compatible

OpenAI 风格错误提取逻辑见：

- [dto/openai_response.go](../dto/openai_response.go)

核心错误结构：

```json
{
  "error": {
    "message": "The model `gpt-4o` does not exist",
    "type": "invalid_request_error",
    "param": "model",
    "code": "model_not_found"
  }
}
```

项目兼容的几种变体：

1. `error` 是标准对象
2. `error` 是字符串
3. `error` 是 map，但字段不完整

字符串错误示意：

```json
{
  "error": "rate limit exceeded"
}
```

项目内部会把它提取为：

```json
{
  "message": "rate limit exceeded",
  "type": "error"
}
```

## 3. Anthropic / Claude

Claude 风格错误提取逻辑见：

- [dto/claude.go](../dto/claude.go)
- [relay/channel/claude/relay-claude.go](../relay/channel/claude/relay-claude.go)

常见结构：

```json
{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "messages: field required"
  }
}
```

项目内部真正关注的是 `error` 对象里的：

- `type`
- `message`

提取后的内部形态约等于：

```json
{
  "type": "invalid_request_error",
  "message": "messages: field required"
}
```

如果上游只返回一个字符串错误，项目会兜底成：

```json
{
  "type": "upstream_error",
  "message": "some upstream error text"
}
```

## 4. Gemini

Gemini 在本项目里最特殊的一点是：

- 它不一定通过标准 `error` 字段表达失败
- 某些“被安全策略拦截”的情况会返回 200，但 `candidates` 为空

相关结构和处理逻辑见：

- [dto/gemini.go](../dto/gemini.go)
- [relay/channel/gemini/relay-gemini.go](../relay/channel/gemini/relay-gemini.go)
- [relay/channel/gemini/relay-gemini-native.go](../relay/channel/gemini/relay-gemini-native.go)

典型“被拦截”响应示意：

```json
{
  "candidates": [],
  "promptFeedback": {
    "blockReason": "SAFETY",
    "safetyRatings": [
      {
        "category": "HARM_CATEGORY_HATE_SPEECH",
        "probability": "MEDIUM"
      }
    ]
  },
  "usageMetadata": {
    "promptTokenCount": 12,
    "totalTokenCount": 12
  }
}
```

项目会将其映射为类似：

```json
{
  "error": {
    "message": "request blocked by Gemini API: SAFETY",
    "type": "upstream_error",
    "code": "prompt_blocked"
  }
}
```

另一种 Gemini 失败是：

- `candidates` 为空
- `promptFeedback.blockReason` 也为空

此时会被视为异常空响应，映射为：

```json
{
  "error": {
    "message": "empty response from Gemini API",
    "type": "upstream_error",
    "code": "empty_response"
  }
}
```

## 5. 项目中有独立错误表达的下游例子

### 5.1 Replicate

定义见：

- [relay/channel/replicate/dto.go](../relay/channel/replicate/dto.go)

结构示意：

```json
{
  "status": "failed",
  "error": {
    "code": "prediction_failed",
    "message": "model execution failed",
    "detail": "CUDA out of memory"
  }
}
```

特点：

- 错误挂在业务响应体中，而不一定完全依赖 HTTP 非 2xx
- `detail` 往往是更具体的执行失败原因

### 5.2 Tencent Hunyuan

定义见：

- [relay/channel/tencent/dto.go](../relay/channel/tencent/dto.go)

结构示意：

```json
{
  "Response": {
    "Error": {
      "Code": 1100,
      "Message": "Authentication failed"
    },
    "Req_id": "abc123"
  }
}
```

特点：

- 错误在 `Response.Error`
- 字段名是大写风格：`Code` / `Message`

### 5.3 Coze

定义见：

- [relay/channel/coze/dto.go](../relay/channel/coze/dto.go)

入口层常见错误示意：

```json
{
  "code": 4001,
  "msg": "invalid bot id"
}
```

任务明细层常见错误示意：

```json
{
  "data": {
    "status": "failed",
    "last_error": {
      "code": 5000,
      "message": "workflow execution failed"
    }
  }
}
```

特点：

- 有时是顶层 `code/msg`
- 有时是业务数据里的 `last_error`

### 5.4 Ali / DashScope

定义见：

- [relay/channel/ali/dto.go](../relay/channel/ali/dto.go)

结构示意：

```json
{
  "code": "InvalidApiKey",
  "message": "The API key is invalid",
  "request_id": "req_xxx"
}
```

特点：

- 没有标准 `error` 包装
- 更像“顶层 code + message”

### 5.5 PaLM

定义见：

- [relay/channel/palm/dto.go](../relay/channel/palm/dto.go)

结构示意：

```json
{
  "error": {
    "code": 400,
    "message": "Invalid argument",
    "status": "INVALID_ARGUMENT"
  }
}
```

特点：

- 接近 Google 风格
- `status` 是语义错误码字符串

### 5.6 Baidu Wenxin

定义见：

- [relay/channel/baidu/dto.go](../relay/channel/baidu/dto.go)

结构示意：

```json
{
  "error_code": 336000,
  "error_msg": "Invalid parameter"
}
```

特点：

- 顶层错误字段不是 `error`
- 项目兜底逻辑会从 `error_msg` 取 message

### 5.7 Zhipu 4V Image

定义和处理见：

- [relay/channel/zhipu_4v/image.go](../relay/channel/zhipu_4v/image.go)

上游结构示意：

```json
{
  "error": {
    "code": "invalid_request",
    "message": "image size too large"
  }
}
```

项目会将其包装为 OpenAI 风格错误：

```json
{
  "error": {
    "message": "image size too large",
    "type": "zhipu_image_error",
    "code": "invalid_request"
  }
}
```

### 5.8 Ollama

处理逻辑见：

- [relay/channel/ollama/relay-ollama.go](../relay/channel/ollama/relay-ollama.go)

常见结构示意：

```json
{
  "error": "model not found"
}
```

项目内部会进一步包装为：

```json
{
  "error": {
    "message": "ollama error: model not found",
    "type": "upstream_error",
    "code": "bad_response_body"
  }
}
```

## 6. 读取这些错误时的一个实用判断顺序

在读 relay 代码或排查兼容问题时，可以按这个顺序看：

1. 该 provider 是否有专门 DTO
2. 该 handler 是否有“业务成功但语义失败”的特殊判断
3. 如果没有，是否最终落到 `service.RelayErrorHandler(...)`
4. `RelayErrorHandler(...)` 是否能从 `error/message/msg/error_msg/detail` 等字段里兜底抽取

对 Gemini 尤其要注意第 2 点，因为它的失败不总是标准错误对象。

## 7. 当前文档对应的关键代码

- [types/error.go](../types/error.go)
- [service/error.go](../service/error.go)
- [dto/error.go](../dto/error.go)
- [dto/openai_response.go](../dto/openai_response.go)
- [dto/claude.go](../dto/claude.go)
- [dto/gemini.go](../dto/gemini.go)
- [relay/channel/gemini/relay-gemini.go](../relay/channel/gemini/relay-gemini.go)
- [relay/channel/replicate/dto.go](../relay/channel/replicate/dto.go)
- [relay/channel/tencent/dto.go](../relay/channel/tencent/dto.go)
- [relay/channel/coze/dto.go](../relay/channel/coze/dto.go)
- [relay/channel/ali/dto.go](../relay/channel/ali/dto.go)
- [relay/channel/palm/dto.go](../relay/channel/palm/dto.go)
- [relay/channel/baidu/dto.go](../relay/channel/baidu/dto.go)
- [relay/channel/zhipu_4v/image.go](../relay/channel/zhipu_4v/image.go)
- [relay/channel/ollama/relay-ollama.go](../relay/channel/ollama/relay-ollama.go)

# Relay 转发格式调研

## 这篇文档的范围

这篇文档只回答三个问题：

1. `relay/` 的协议互转到底是不是“统一内部格式”
2. 文本主链路里 OpenAI DTO 为什么会像“事实上的中间桥”
3. `relay` 调上游时，channel 中的凭证材料大致有哪些使用模式

以下内容已经有主文档，不在这里重复展开：

- Relay 主运行时、`RelayInfo`、`Adaptor`、重试、`service/relay/channel` 依赖链：见 [project-architecture-03-relay-runtime.md](/Users/gaorx/Works/my/new-api/research/project-architecture-03-relay-runtime.md)
- 渠道选路、Affinity、熔断恢复：见 [project-architecture-05-channel-routing.md](/Users/gaorx/Works/my/new-api/research/project-architecture-05-channel-routing.md)
- 计费、预扣、结算：见 [project-architecture-06-billing-system.md](/Users/gaorx/Works/my/new-api/research/project-architecture-06-billing-system.md)
- 任务型 adaptor 与轮询：见 [project-architecture-11-async-tasks-and-background-jobs.md](/Users/gaorx/Works/my/new-api/research/project-architecture-11-async-tasks-and-background-jobs.md)
- 中间件入口链路：见 [project-architecture-12-middleware-and-request-flow.md](/Users/gaorx/Works/my/new-api/research/project-architecture-12-middleware-and-request-flow.md)

## 调研结论

`relay/` 的总体原理不是“所有协议都先强制变成唯一 canonical DTO”，也不是“所有协议两两直连互转”。

更准确的说法是：

```text
统一的是 Relay 运行时框架
不完全统一的是协议 DTO
而 OpenAI DTO 在文本主链路里经常被当作中间桥
```

如果只用一句话概括：

> `relay/` 是“统一运行时 + 目标渠道 adaptor 转换”的组合；在文本/chat 主链路上，Claude、Gemini 等协议经常先归一化到 OpenAI 风格请求/响应，再桥接到目标 provider，但系统仍保留同协议直通、局部直接转换和 pass-through 旁路。

## 1. 为什么说运行时是统一的

同步请求统一从 `controller/relay.go` 的 `Relay()` 进入，先做：

- 请求校验
- `RelayInfo` 构建
- 敏感词检测
- token 预估
- 价格计算
- 预扣费
- 重试控制

然后再按入口协议和业务模式分发到：

- `relay.TextHelper`
- `relay.ImageHelper`
- `relay.AudioHelper`
- `relay.EmbeddingHelper`
- `relay.ResponsesHelper`
- `relay.ClaudeHelper`
- `relay.GeminiHelper`

所以系统首先统一的是“执行框架”，不是所有 payload 的单一内部模型。

关键代码：

- `controller/relay.go`
- `relay/common/relay_info.go`
- `relay/channel/adapter.go`

## 2. 为什么说 OpenAI DTO 是“事实上的中间桥”

`relay/channel/adapter.go` 同时保留了多种入口：

- `ConvertOpenAIRequest(...)`
- `ConvertClaudeRequest(...)`
- `ConvertGeminiRequest(...)`
- `ConvertOpenAIResponsesRequest(...)`

这说明设计上允许多条转换路径。但在文本主链路里，很多实现并没有直接写“协议 A -> 协议 B”的全套转换，而是借 `dto.GeneralOpenAIRequest` 和 OpenAI 风格响应做桥。

### 2.1 请求侧的典型桥接链路

最常见的几条链路是：

```text
Claude 请求 -> OpenAI 请求 -> 目标渠道请求
Gemini 请求 -> OpenAI 请求 -> 目标渠道请求
Claude 请求 -> OpenAI 请求 -> Gemini 请求
```

典型例子：

- `openai.Adaptor.ConvertClaudeRequest(...)`
  - 先调 `service.ClaudeToOpenAIRequest(...)`
  - 再调 `ConvertOpenAIRequest(...)`
- `openai.Adaptor.ConvertGeminiRequest(...)`
  - 先调 `service.GeminiToOpenAIRequest(...)`
  - 再调 `ConvertOpenAIRequest(...)`
- `gemini.Adaptor.ConvertClaudeRequest(...)`
  - 先借 `openai.Adaptor{}.ConvertClaudeRequest(...)`
  - 得到 `*dto.GeneralOpenAIRequest`
  - 再调 Gemini 自己的 `ConvertOpenAIRequest(...)`

相关位置：

- `relay/channel/openai/adaptor.go`
- `relay/channel/gemini/adaptor.go`
- `service/convert.go`

### 2.2 响应侧也有类似桥接

响应侧没有统一暴露 `Convert*Response(...)` 接口，统一入口是：

- `DoResponse(c, resp, info) (usage any, err *types.NewAPIError)`

但很多 provider 内部仍然会先把响应规整成 OpenAI 风格，再决定是否继续转成 Claude / Gemini。

典型链路：

```text
OpenAI 上游响应 -> Claude / Gemini 客户端响应
Gemini 上游响应 -> OpenAI 响应 -> Claude 客户端响应
```

常见转换函数：

- `service.ResponseOpenAI2Claude(...)`
- `service.ResponseOpenAI2Gemini(...)`
- `service.StreamResponseOpenAI2Claude(...)`
- `service.StreamResponseOpenAI2Gemini(...)`
- `responseGeminiChat2OpenAI(...)`
- `streamResponseGeminiChat2OpenAI(...)`

相关位置：

- `relay/channel/openai/helper.go`
- `relay/channel/openai/relay-openai.go`
- `relay/channel/gemini/relay-gemini.go`
- `service/convert.go`

### 2.3 为什么它还不算“严格统一内部格式”

因为系统仍然保留这些情况：

- 同协议直通
  - Claude 原生入口到 Claude 上游，`ConvertClaudeRequest(...)` 可以直接返回原请求
  - Gemini 原生入口到 Gemini 上游，可以基本保留 Gemini 结构，仅做少量修正
- Pass-through 旁路
  - 开启 `PassThroughRequestEnabled` 或 `PassThroughBodyEnabled` 时，直接复用原始 body
- 局部能力不共享同一桥接模型
  - image / audio / embedding / rerank / responses 并不完全共用同一套 OpenAI 中间对象

所以更准确的说法不是“系统有唯一统一内部格式”，而是：

> 在文本聊天这条最核心的 relay 主链路上，OpenAI 结构被广泛复用为桥接层。

## 3. RelayFormat 的数量，和“真正参与主互转的格式”不是一回事

按 `types.RelayFormat` 看，项目里声明了 12 种格式：

- `openai`
- `claude`
- `gemini`
- `openai_responses`
- `openai_responses_compaction`
- `openai_audio`
- `openai_image`
- `openai_realtime`
- `rerank`
- `embedding`
- `task`
- `mj_proxy`

但如果问题是“真正参与聊天/文本主协议互转的格式有多少种”，核心仍然是 3 种：

- `openai`
- `claude`
- `gemini`

原因是：

- adaptor 专门为这三种保留了独立入口方法
- 现有桥接函数也主要围绕这三种协议展开
- 其他很多 `RelayFormat` 更接近 OpenAI 生态变体、专项能力格式或任务入口

可以简单理解成：

```text
主文本互转协议：OpenAI / Claude / Gemini
其他 RelayFormat：OpenAI 家族变体、专项能力格式、任务型入口
```

## 4. `relay` 调上游时，channel 中的凭证材料有哪些模式

这部分只保留凭证使用模式本身；channel 的选路语义和业务含义见：

- [project-architecture-04-core-concepts-and-relationships.md](/Users/gaorx/Works/my/new-api/research/project-architecture-04-core-concepts-and-relationships.md)
- [project-architecture-05-channel-routing.md](/Users/gaorx/Works/my/new-api/research/project-architecture-05-channel-routing.md)

最准确的结论是：

> `relay` 调上游时，本质上总是依赖“预先配置在 channel 中的凭证材料”；但这些材料不一定是狭义 API key，也可能是 OAuth token、service account、`client_id|client_secret`，或本地签名所需密钥。

### 4.1 最常见模式：静态 API Key 直传

常见例子：

- OpenAI / 兼容 OpenAI：`Authorization: Bearer <ApiKey>`
- Azure OpenAI：`api-key: <ApiKey>`
- Claude：`x-api-key: <ApiKey>`
- Gemini / PaLM：`x-goog-api-key: <ApiKey>`

### 4.2 预存 OAuth token 后直接消费

典型是 `codex` 渠道。

channel `key` 实际保存的是 JSON，里面会有：

- `access_token`
- `refresh_token`
- `account_id`

转发时 adaptor 直接读取这些字段，构造：

- `Authorization: Bearer <access_token>`
- `chatgpt-account-id: <account_id>`

这里 relay 主链路不是每次都重新做 OAuth 登录，而是消费已经存下来的 token。

### 4.3 预配置原始凭证，动态向上游换临时 access token

典型有：

- Vertex AI
- 百度文心

Vertex AI 会把 Google service account JSON 作为 channel key，转发前：

1. 本地签 JWT
2. 调 token endpoint
3. 换取临时 `access_token`
4. 缓存后再调用真正模型接口

百度文心常见是：

```text
client_id|client_secret
```

先换 `access_token`，再带 token 调模型接口。

### 4.4 预配置原始密钥，本地签名后调用

典型是智谱 `zhipu`。

channel 中保存的是类似 `id.secret` 的原始密钥材料，请求前会：

1. 组装 claims
2. 本地签出 JWT / token
3. 把签名结果放进 `Authorization`

这和“静态 key 直传”以及“先向上游换 access token”都不一样。

### 4.5 `codex` 还带有后台刷新机制

`codex` 不只是“保存 access token 然后一直用”，还会：

- 读取 `refresh_token`
- 调刷新接口拿新 token
- 把更新后的凭证写回 channel `key`

所以更准确地说，它是：

- 请求路径消费已保存 token
- 后台或服务层按需刷新 token

不是“每次转发时现场申请一个新的永久 API key”。

## 5. 一页结论

如果只保留最关键的信息，这篇调研可以压缩成下面 5 条：

1. `relay/` 统一的是运行时框架，不是所有协议 DTO 的唯一 canonical model。
2. 文本主链路里，OpenAI DTO 经常充当事实上的中间桥。
3. Claude 和 Gemini 并不总是两两直转，很多时候会借 OpenAI 做桥。
4. 系统保留同协议直通和 pass-through 旁路，所以不存在“全部强制先转 OpenAI”的硬规则。
5. 调上游时总是依赖 channel 中预先配置的凭证材料，但这些材料既可能是静态 key，也可能是 OAuth token、动态换取 token 的原始凭证，或本地签名密钥。

## 关键参考文件

- `controller/relay.go`
- `relay/common/relay_info.go`
- `relay/channel/adapter.go`
- `relay/channel/openai/adaptor.go`
- `relay/channel/openai/helper.go`
- `relay/channel/openai/relay-openai.go`
- `relay/channel/gemini/adaptor.go`
- `relay/channel/gemini/relay-gemini.go`
- `relay/channel/api_request.go`
- `relay/channel/vertex/service_account.go`
- `relay/channel/baidu/adaptor.go`
- `relay/channel/codex/adaptor.go`
- `relay/channel/zhipu/relay-zhipu.go`
- `service/convert.go`
- `service/codex_credential_refresh.go`

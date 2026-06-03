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

这里要特别区分两类经常被混称为 “OAuth” 的东西：

- 用户登录 OAuth：让 GitHub、Discord、OIDC、LinuxDO 等外部网站账号登录本项目控制台
- 渠道接入 OAuth：让本项目作为 client 去上游平台换取调用模型接口所需的 token

本节只讨论第二类，也就是 channel / relay 侧的 OAuth，与平台用户登录态没有直接关系。

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

这类 token 的用途不是“证明某个用户登录了本项目”，而是“证明当前 channel 有权调用该上游平台接口”。

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

因此从渠道语义上看，这一类虽然也用了 OAuth token endpoint，但项目本地长期保存的往往不是最终 `access_token`，而是能换取它的原始凭证材料。

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

## 5. `codex relay` 和 `openai relay` 的差异

如果只看网关入口，`codex` 和 `openai` 都可以从统一的 `/v1/*` 路由进入，因此表面上都像“OpenAI 风格接口”。

但从实际 adaptor 与上游行为看，`codex` 不是普通 `openai` channel 的别名，而是一个独立渠道类型：

- `openai` 对应 `constant.ChannelTypeOpenAI`
- `codex` 对应 `constant.ChannelTypeCodex`
- `relay.GetAdaptor()` 会把两者分发到不同 adaptor

可以把它理解成：

```text
统一客户端入口
  -> 相同的 Relay 运行时
  -> 不同的 channel adaptor
  -> 不同的真实上游协议
```

### 5.1 路由入口看起来相似，但上游目标不同

项目对外暴露的 Relay 入口没有专门单独开一个 `/v1/codex/...`。

文本相关常见入口仍然是：

- `/v1/chat/completions`
- `/v1/responses`
- `/v1/responses/compact`

真正的区别发生在渠道分发之后：

- `openai` 默认上游 base URL 是 `https://api.openai.com`
- `codex` 默认上游 base URL 是 `https://chatgpt.com`

而且 `codex` adaptor 会把请求真正转发到：

- `/backend-api/codex/responses`
- `/backend-api/codex/responses/compact`

这说明它对接的不是标准 OpenAI 公共 API 路径，而是 ChatGPT/Codex 后台接口。

### 5.2 `openai` 支持的能力面更大，`codex` 被刻意收窄

`openai` adaptor 支持的能力非常多，包含：

- `/v1/chat/completions`
- `/v1/responses`
- `/v1/embeddings`
- image / audio / realtime
- Claude/Gemini 请求桥接到 OpenAI 风格请求

而 `codex` adaptor 明确只支持：

- `/v1/responses`
- `/v1/responses/compact`

以下入口在 `codex` adaptor 中会直接返回不支持：

- `/v1/chat/completions`
- `/v1/messages`
- `/v1/embeddings`
- `/v1/rerank`
- image / audio 相关入口

所以 `codex` 不是“兼容 OpenAI 全家桶”的上游，更像一个只接入 Responses 家族的专门实现。

### 5.3 `codex` 的鉴权材料不是普通 API key

`openai` channel 最常见的模式是：

- channel `key` 直接保存普通 API key
- 请求头使用 `Authorization: Bearer <api_key>`

`codex` channel 则不同。它要求 channel `key` 是一个 JSON，对象中至少包含：

- `access_token`
- `account_id`

通常还会带：

- `refresh_token`
- `expired`
- `last_refresh`
- `email`

因此从系统设计上看，`codex` 更接近“保存一份 OAuth 会话凭证”，而不是“保存一把静态 API key”。

### 5.4 `codex` 的请求头和请求体也有特殊约束

`codex` 转发时会额外设置一组专用头：

- `Authorization: Bearer <access_token>`
- `chatgpt-account-id: <account_id>`
- `originator: codex_cli_rs`
- `OpenAI-Beta: responses=experimental`

并且它对 `Content-Type` 更严格，会强制使用精确的 `application/json`。

请求体侧也有一些 Codex 特有兼容逻辑：

- `instructions` 字段必须存在；若客户端没传，会补成空字符串
- 非 compact 模式下强制 `store=false`
- 非 compact 模式下会移除 `max_output_tokens`
- 非 compact 模式下会移除 `temperature`

这说明 `codex` 虽然消费的是 OpenAI Responses 风格 DTO，但上游契约并不等价于普通 OpenAI Responses API。

### 5.5 响应处理层复用了 OpenAI Responses handler

`codex` 的特殊性主要在：

- 上游 URL
- 鉴权方式
- 特定请求头
- 请求体裁剪规则

但在响应解析阶段，它没有单独发明一套完全不同的输出模型，而是复用了：

- `openai.OaiResponsesHandler(...)`
- `openai.OaiResponsesStreamHandler(...)`
- `openai.OaiResponsesCompactionHandler(...)`

所以更准确的说法不是“Codex 对客户端暴露了一套完全新的返回格式”，而是：

> 客户端看到的仍然是 OpenAI Responses 风格输出；特殊之处主要体现在它连接的是不同的上游后台接口。

### 5.6 为什么说它“有自己的特殊 API”

从本项目的实现视角，可以明确认为：有。

这里的“特殊 API”不是指网关额外发明了一套客户端公开协议，而是指它对接了 OpenAI/Codex 体系里一套不同于标准公开 API 的后台接口组合，包括：

- `https://chatgpt.com/backend-api/codex/responses`
- `https://chatgpt.com/backend-api/codex/responses/compact`
- `https://chatgpt.com/backend-api/wham/usage`
- `https://auth.openai.com/oauth/authorize`
- `https://auth.openai.com/oauth/token`

同时项目里还专门为它实现了：

- Codex OAuth 启动
- Codex OAuth 完成
- Codex 凭证刷新
- Codex 用量查询

所以在系统建模上，`codex` 应该被理解成：

```text
OpenAI 风格客户端入口
  + Codex 专用凭证
  + ChatGPT/Codex 后台上游
  + Responses-only 能力面
```

## 6. 一页结论

如果只保留最关键的信息，这篇调研可以压缩成下面 5 条：

1. `relay/` 统一的是运行时框架，不是所有协议 DTO 的唯一 canonical model。
2. 文本主链路里，OpenAI DTO 经常充当事实上的中间桥。
3. Claude 和 Gemini 并不总是两两直转，很多时候会借 OpenAI 做桥。
4. 系统保留同协议直通和 pass-through 旁路，所以不存在“全部强制先转 OpenAI”的硬规则。
5. 调上游时总是依赖 channel 中预先配置的凭证材料，但这些材料既可能是静态 key，也可能是 OAuth token、动态换取 token 的原始凭证，或本地签名密钥。
6. `codex` 不是普通 `openai` channel 的模型别名，而是复用 OpenAI Responses 风格输入输出、但连接 ChatGPT/Codex 后台接口的一类独立 channel。

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

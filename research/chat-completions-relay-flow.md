# `/v1/chat/completions` 转发链路说明

## 这篇文档的范围

这篇文档只保留 `/v1/chat/completions` 这条链路中最有“接口个性”的部分。

以下通用内容已经有主文档，不在这里重复展开：

- Relay 总运行时、重试、预扣费、`service -> relay -> relay/channel`：见 [project-architecture-03-relay-runtime.md](/Users/gaorx/Works/my/new-api/research/project-architecture-03-relay-runtime.md)
- 中间件链、`TokenAuth()`、`Distribute()`、`ModelRequestRateLimit()`：见 [project-architecture-12-middleware-and-request-flow.md](/Users/gaorx/Works/my/new-api/research/project-architecture-12-middleware-and-request-flow.md)
- 计费体系、quota、正式结算：见 [project-architecture-06-billing-system.md](/Users/gaorx/Works/my/new-api/research/project-architecture-06-billing-system.md)
- 协议互转与 OpenAI 作为事实中间桥：见 [relay.md](/Users/gaorx/Works/my/new-api/research/relay.md)

本文重点回答：

1. `/v1/chat/completions` 在这个项目里进入了哪条 relay 分支
2. 哪些地方会对 chat 请求做特别处理
3. 什么情况下“表面上是 chat，实际上走的是 responses”
4. 非流式和流式在最后一段返回路径上有什么差异

## 1. 一页总览

客户端调用：

```text
POST /v1/chat/completions
```

在当前项目里，核心链路可以概括成：

```text
路由与中间件
  -> controller.Relay
  -> 生成 RelayInfo
  -> 预估 token + 价格 + 预扣费
  -> relay.TextHelper
  -> channel adaptor.ConvertOpenAIRequest
  -> adaptor.DoRequest
  -> adaptor.DoResponse
  -> PostTextConsumeQuota / SettleBilling
```

和很多其他入口相比，这个接口有 4 个最值得单独记住的特性：

1. 它默认走 `relay.TextHelper()` 这条文本主链。
2. 它会处理 `stream_options`，并受渠道是否支持 `StreamOptions` 影响。
3. 它可能被改写成“chat 入口，实际走 responses 上游”。
4. 它的流式与非流式最终都要回到统一的 usage 结算。

## 2. 这条链路真正独特的阶段

### 2.1 请求体会先进入 OpenAI 文本请求校验

`controller.Relay` 中，`helper.GetAndValidateRequest()` 对 `/v1/chat/completions` 会落到文本请求校验逻辑，核心对象是：

- `dto.GeneralOpenAIRequest`

这里做的是 chat 入口特有的检查，例如：

- `model` 必填
- `messages` 必填
- `max_tokens` 等字段约束
- 某些 OpenAI 风格扩展字段的合法性校验

所以从一开始，这个接口就是以 OpenAI chat DTO 作为主输入对象。

### 2.2 它默认进入 `relay.TextHelper()`

在 `controller/relay.go` 里，普通 OpenAI chat 入口不会去走 image / audio / embedding / responses helper，而是：

```text
controller.Relay
  -> relayHandler
  -> relay.TextHelper
```

这意味着它天然复用了项目里最成熟的一条文本主链：

- 模型映射
- 协议转换
- 上游错误处理
- usage 提取
- 流式 SSE 转发

### 2.3 `stream_options` 会在这里被统一裁剪

这是 `/v1/chat/completions` 很典型的兼容点。

在文本 helper / adaptor 处理中，会根据实际情况修正 `StreamOptions`：

- 如果 `stream != true`，则清掉 `StreamOptions`
- 如果当前渠道不支持 `StreamOptions`，则清掉
- 如果配置要求强制返回 usage，可能会自动补 `IncludeUsage`

这意味着客户端虽然传的是 OpenAI chat 兼容参数，但最终能否原样发到上游，取决于：

- 是否流式
- 渠道能力
- 渠道设置

### 2.4 模型名会在这里做“用户模型名 -> 上游模型名”映射

`helper.ModelMappedHelper()` 会把用户看到的逻辑模型名，映射成当前渠道真正要发给上游的模型名。

所以：

```text
客户端的 model
  -> OriginModelName
  -> UpstreamModelName
```

对 `/v1/chat/completions` 来说，这一步非常关键，因为后面的：

- adaptor 选协议细节
- URL 路径
- 计费快照
- 日志记录

都会依赖这个映射结果。

## 3. 这条链路最容易被忽略的分支：Chat 实际走 Responses

这是当前项目里 `/v1/chat/completions` 最“反直觉”的一段。

满足一定条件时，chat 请求并不会直接打上游 `/chat/completions`，而是进入：

```text
chatCompletionsViaResponses()
```

也就是：

```text
Chat Completions Request
  -> 转成 Responses Request
  -> 请求上游 /v1/responses
  -> 再转回 Chat Completions 风格响应
```

### 3.1 这条分支通常在什么条件下发生

典型条件包括：

- 当前仍处于 `RelayModeChatCompletions`
- 没有开启 pass-through
- 渠道没有强制透传原始 body
- `ShouldChatCompletionsUseResponsesGlobal(...) == true`

也就是说，项目是在“保证对客户端仍表现为 chat 接口”的前提下，内部把上游调用切成了 responses。

### 3.2 它具体做了哪些事

这条链路里，核心动作是：

1. 对原始 chat 请求先应用字段删除和参数覆盖
2. `ChatCompletionsRequestToResponsesRequest(...)`
3. 临时把 `RelayInfo.RelayMode` 改成 `Responses`
4. `ConvertOpenAIResponsesRequest(...)`
5. 发上游 `/v1/responses`
6. 非流式时把 responses 响应转回 chat JSON
7. 流式时把 responses SSE 转回 chat SSE chunk

所以客户端看到的是：

```text
/v1/chat/completions
```

但内部真实请求的可能是：

```text
/v1/responses
```

### 3.3 为什么这个分支重要

因为它解释了很多“看起来是 chat，但调试时像 responses”的现象：

- 请求参数可能先被改写成 responses 风格
- 最终 URL 可能不是 `/chat/completions`
- 上游返回语义先按 responses 解析，再转回 chat

如果后面要排查兼容问题，这通常是首要检查点。

## 4. 非流式和流式的真正差别在哪里

这两条分支前半段非常像，真正的差别主要发生在 `DoResponse(...)` 的最后阶段。

### 4.1 非流式：拿完整响应，再统一规整

非流式大致是：

```text
上游返回完整 JSON
  -> 读完整 body
  -> 解析 choices / usage / finish_reason
  -> 必要时补 usage
  -> 必要时转 Claude / Gemini 等下游格式
  -> 整体写回客户端
```

这里最关键的是：

- 如果上游不给 usage，项目会尝试本地补算
- 如果出口协议不是 OpenAI 原样，还会在最后一步做格式转换

### 4.2 流式：边读 chunk，边转发，最后统一收尾

流式大致是：

```text
上游返回 SSE
  -> 逐块扫描
  -> 解析 delta / tool call / reasoning / usage
  -> 每个 chunk 立即写给客户端
  -> 最后处理 last chunk
  -> 必要时补 stop / usage / [DONE]
```

这里最关键的是：

- 流式不是等全量响应，而是边读边发
- 最终 usage 可能来自最后一个 chunk，也可能来自本地兜底估算
- thinking / reasoning 之类的 provider 特有字段，也经常在这一步被规整

### 4.2.1 “边读边发”不等于“每个 chunk 都一定重写”

这个问题在排查性能和兼容性时很常见，准确说法要分两类：

#### 情况 A：上游和下游格式不同

如果是跨协议流式 relay，通常**每个 chunk 都要在线转换**。

典型模式是：

```text
收到一个上游 chunk
  -> 反序列化成当前 provider 的流式 DTO
  -> 转成目标协议 chunk
  -> 立刻写给客户端
```

常见例子：

- `Gemini SSE -> OpenAI SSE`
- `OpenAI SSE -> Claude SSE`
- `OpenAI SSE -> Gemini SSE`
- `Baidu SSE -> OpenAI SSE`

所以在跨协议流式场景里，可以近似理解成：

```text
一块进
一块转
一块出
```

#### 情况 B：上游和下游都是 OpenAI 风格流

如果本次 relay 输出格式本来就是 OpenAI，且没有开启额外改写开关，那么**未必需要把每个 chunk 重新改写后再发**。

更准确地说：

- 系统仍然会逐 chunk 读取
- 仍然会逐 chunk 解析一遍，用于累计文本、tool call、usage 估算等内部逻辑
- 但对客户端回写时，很多情况下可以基本按原 chunk 直接转发

也就是说，这一类更接近：

```text
每个 chunk 都会被消费和观察
但不一定都会被重写
```

#### 哪些开关会让 OpenAI -> OpenAI 也发生 chunk 改写

即使上下游都是 OpenAI 风格流，只要开启以下能力，chunk 仍然可能被逐块改写：

- `force_format`
- `thinking_to_content`

这类逻辑会在流式发送阶段把：

- reasoning / thinking 字段
- 内容片段形态
- 最终补发 usage / stop 的行为

做额外规整。

所以一句话总结这段差别：

> Chat 流式 relay 总是“逐 chunk 处理”，但只有跨协议转换或显式开启格式改写时，才基本等价于“每个 chunk 都要转换”。

### 4.3 两条分支最后都会回到统一结算

无论非流式还是流式，只要转发成功，最终都会进入：

- `PostTextConsumeQuota(...)`
- `SettleBilling(...)`

也就是说：

```text
前面返回形式不同
后面结算逻辑尽量统一
```

这也是为什么这个项目能同时兼容很多 provider，又还能把 quota 结算统一收口。

## 5. 排查 `/v1/chat/completions` 问题时，优先看什么

如果这个接口表现异常，最值得优先确认的是这 6 件事：

1. 请求是否真的按 OpenAI chat 入口成功解析成 `dto.GeneralOpenAIRequest`
2. 当前 `RelayMode` 最终是不是普通 chat，还是被改写成了 `Responses`
3. `ModelMappedHelper()` 后的 `UpstreamModelName` 是什么
4. 当前渠道是否支持 `StreamOptions`
5. adaptor 实际调用的是哪一种 `Convert*Request(...)`
6. `DoResponse(...)` 最终 usage 是上游给的，还是本地兜底补的

这 6 个点基本覆盖了 chat 入口最常见的兼容问题来源。

## 6. 一句话结论

`/v1/chat/completions` 在这个项目里不是“简单把 OpenAI chat body 原样转发出去”，而是：

> 以 OpenAI chat DTO 为主输入，经由统一文本 relay 主链完成模型映射、能力裁剪、协议转换、上游调用和最终结算；其中在部分渠道或配置下，它还会被内部改写成“chat 入口，responses 上游”的兼容路径。

## 关键参考文件

- `router/relay-router.go`
- `controller/relay.go`
- `relay/compatible_handler.go`
- `relay/chat_completions_via_responses.go`
- `relay/responses_handler.go`
- `relay/channel/openai/adaptor.go`
- `relay/channel/openai/relay-openai.go`
- `relay/channel/openai/helper.go`

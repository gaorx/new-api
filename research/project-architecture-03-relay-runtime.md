# 03 - Relay 运行时架构

## Relay 子系统定位

如果说管理后台是“控制面”，那么 `relay/` 就是“数据面”。

Relay 子系统负责把统一入口格式转换为具体上游厂商请求，并处理响应回写、usage 提取、错误映射、流式转发。

核心入口：

- `controller/relay.go`
- `relay/common/relay_info.go`
- `relay/channel/adapter.go`

## 核心抽象：`RelayInfo`

`RelayInfo` 是整条代理链路的上下文对象，几乎承载了一个请求从进入网关到完成结算的全部状态。

它包含的信息大致分为几类：

- 调用方信息：用户、令牌、分组、订阅来源
- 请求信息：模型名、relay format、relay mode、流式标记
- 渠道信息：渠道类型、base URL、上游 key、模型映射、header override
- 计费信息：预扣额度、Billing 会话、价格快照、tiered billing snapshot
- 流式/协议状态：WebSocket、stream status、request conversion chain
- 重试状态：retry index、last error

可以把它看成：

```text
RelayInfo = 请求上下文 + 渠道上下文 + 计费上下文 + 协议上下文 + 重试上下文
```

## 核心抽象：`Adaptor`

`relay/channel/adapter.go` 定义了统一适配器接口。

每个上游渠道都要实现类似能力：

- 构造上游 URL
- 设置请求头
- 把统一请求格式转换成上游格式
- 发请求
- 解析上游响应并回写给客户端

这意味着整个 Relay 层本质上是：

```text
统一入口协议
  -> 统一上下文 RelayInfo
  -> 具体渠道 Adaptor
  -> 具体上游实现
```

它避免了把每个上游写成“独立 controller”，而是统一挂在一个网关框架之下。

## `service`、`relay`、`relay/channel` 的链路关系

这三个目录从职责上看，最容易理解成下面这条主链：

```text
controller
  -> service
  -> relay
  -> relay/channel/<provider>
```

如果只看同步主请求链路，这个理解基本成立：

1. `controller/relay.go` 负责总入口，做请求解析、重试控制和错误收口。
2. `service/` 负责前置业务能力，例如敏感词检查、token 估算、预扣费、渠道选择、结算。
3. `relay/` 负责真正的转发编排，按 `relay format` 和 `relay mode` 分发到不同 helper。
4. `relay/channel/*` 负责具体上游适配，实现 URL 构造、header、请求转换、发请求、响应解析。

对应到关键代码位置：

- `controller/relay.go`
- `relay/relay_adaptor.go`
- `relay/channel/adapter.go`

### 1. 职责分层

可以把三者的角色简单记成：

- `service`：业务规则层
- `relay`：转发编排层
- `relay/channel`：上游适配层

其中 `relay.GetAdaptor()` 会把 `apiType` 映射成具体 adaptor，例如 OpenAI、Claude、Gemini、Ollama 等实现。

### 2. 真实代码依赖并不是严格单向

虽然职责上很像：

```text
service -> relay -> relay/channel
```

但实际代码并不是严格的单向分层。

更接近真实情况的是：

```text
controller -> service + relay
relay -> service + relay/channel
relay/channel -> service
service -> relay/common + relay/constant
```

原因主要有三类：

1. `relay` 会调用 `service` 做错误处理、usage 结算、quota 后处理。
2. `relay/channel/*` 会调用 `service` 做协议转换或通用工具复用，例如 `ClaudeToOpenAIRequest`、`GeminiToOpenAIRequest`。
3. `service` 虽然不直接依赖 `relay` 的主 handler，但会依赖 `relay/common`、`relay/constant`，少数地方还会依赖具体 `relay/channel` 子包的结构或类型。

例如：

- `relay/responses_handler.go` 会调用 `service.RelayErrorHandler(...)`、`service.PostTextConsumeQuota(...)`
- `relay/channel/openai/adaptor.go` 会调用 `service.ClaudeToOpenAIRequest(...)`
- `service/convert.go` 会直接 import `relay/common` 和 `relay/channel/openrouter`

所以这里更适合把它理解成：

- 主执行方向是 `controller -> service -> relay -> relay/channel`
- 但 import 关系上，`service` 和 `relay` 之间不是完全隔离的

### 3. 一个专门的断环设计：任务轮询

异步任务轮询是这个项目里一个很典型的“有意打断循环依赖”的例子。

`service/task_polling.go` 没有直接 import `relay` 获取任务 adaptor，而是：

1. 在 `service` 中定义最小接口 `TaskPollingAdaptor`
2. 声明 `GetTaskAdaptorFunc`
3. 在 `main.go` 中把 `service.GetTaskAdaptorFunc` 注入为 `relay.GetTaskAdaptor`

这相当于把原本可能形成的：

```text
service -> relay -> relay/channel -> service
```

改写成：

```text
service -> 抽象接口
main -> 注入 relay.GetTaskAdaptor
relay -> relay/channel
```

这说明项目作者已经意识到 `service` 和 `relay` 之间存在天然耦合点，并在任务轮询这条链路上显式做了断环处理。

## Relay 的请求生命周期

以 `/v1/chat/completions` 为例，主流程大概是：

1. `TokenAuth` 校验调用令牌
2. `ModelRequestRateLimit()` 校验当前用户在 Relay 主链路上的请求频率
3. `Distribute` 解析请求中的模型和分组，选出渠道
4. `controller.Relay()` 读取请求体并校验格式
5. `GenRelayInfo()` 组装上下文
6. 敏感词检查、token 预估、价格计算
7. `PreConsumeBilling()` 预扣费
8. 获取渠道并构建适配器
9. 适配器转换请求并调用上游
10. 解析 usage / 错误 / 流式结果
11. 成功则结算，失败则退款并视情况重试

这个流程里有两个很重要的架构点：

1. 请求体会被缓存成可重复读取的 body storage，方便重试时重复发送。
2. 计费发生在调用前后两个阶段：先预扣，再按实际 usage 结算。

## 转发时的“限流”不只有入口 429

如果只看路由，中间件层最直观的限流是 `ModelRequestRateLimit()`。

但从 Relay 运行时看，项目实际上有三层互相配合的“节流/削峰/自保护”机制：

1. 入口限流
2. 渠道失败后的重试切换
3. 坏渠道自动封禁

这三层叠在一起，才构成完整的转发治理。

## 1. 入口限流：先拦住调用方

在 `/v1` 与 `/v1beta` 同步 Relay 路由里，请求顺序是：

1. `TokenAuth()`
2. `ModelRequestRateLimit()`
3. `Distribute()`
4. `controller.Relay()`

也就是说：

- 先识别是谁在调
- 再按用户做模型调用限流
- 通过后才去选 channel 和真正转发

这层机制的重点不是“按模型名单独记桶”，而是“按用户限制其进入 Relay 主链路的频率”。

## 2. 失败后的重试：把 429 当成可切换渠道的信号

同步 Relay 在 `controller/relay.go` 里有一个主重试循环：

- 每轮都可以重新选可用 channel
- 每轮都会恢复请求体
- 成功则立即返回
- 失败则根据错误类型判断要不要切到下一条 channel

默认策略下，`429` 是可重试的：

- `shouldRetry()` 最终会走 `operation_setting.ShouldRetryByStatusCode(code)`
- 默认重试范围覆盖 `429`
- `504` 与 `524` 被硬编码排除，不重试

这意味着项目把“上游限流”更多理解成：

```text
当前这条渠道忙了
  -> 换下一条渠道再试
```

而不是立刻把错误原样返回给最终调用方。

## 3. 重试不是无条件的

以下场景会直接终止切换渠道：

- `RetryTimes` 已经耗尽
- 请求锁定了 `specific_channel_id`
- Channel Affinity 配置要求失败后不要跳出黏性渠道
- 错误显式带有 `skip-retry`
- 某些错误码被声明为永不重试

所以它不是“无限兜底代理”，而是“带明确边界的多渠道故障转移”。

## 4. 自动封禁：避免坏渠道持续承压

一旦某轮请求失败，Relay 会调用 `processChannelError()`。

这个函数会做两件关键事：

- 记录结构化渠道错误日志
- 如果满足条件，则异步调用 `DisableChannel()`

是否封禁由 `service.ShouldDisableChannel()` 决定，默认逻辑是：

- 开启了 `AutomaticDisableChannelEnabled`
- 错误是典型渠道错误，或者
- 状态码命中了 `AutomaticDisableStatusCodes`，或者
- 错误消息命中了 `AutomaticDisableKeywords`

默认状态码配置里，自动禁用更偏向鉴权/失效类问题，例如 `401`。

这也说明一个很关键的设计取向：

- `429` 默认更偏向“重试/切换渠道”
- `401`、坏 key、认证失败这类错误更偏向“禁用渠道”

## 5. 异步任务 Relay 的处理思路类似，但更保守

Midjourney、Suno、视频等任务型转发没有统一挂 `ModelRequestRateLimit()`，但提交阶段也有自己的重试逻辑。

在 `shouldRetryTaskRelay()` 里：

- `429` 会被视为可重试
- `307` 会被视为可重试
- `5xx` 大多可重试
- `400`、`408`、本地错误不重试

并且最终返回用户前，任务型接口还会把 `429` 文案统一改写成“当前分组上游负载已饱和，请稍后再试”。

所以任务型转发虽然不走入口频控，但仍然有一套“遇到上游限流时先内部消化一轮”的机制。

## RelayFormat 与 RelayMode

系统把“接口协议形态”和“业务模式”拆开建模：

- `RelayFormat`
  比如 OpenAI、Claude、Gemini、Responses、Embedding、Audio。
- `RelayMode`
  更偏执行模式，如 chat、image、embedding、audio transcription、rerank 等。

这个拆分让系统既能按协议适配，也能按业务类型分流处理。

## Task Relay

除了同步请求，系统还支持 Midjourney、Suno、视频等异步任务。

这类请求的特点是：

- 提交时就要锁定预扣费
- 后续靠任务轮询补充状态
- 完成后再根据实际结果调整计费

因此项目又定义了 `TaskAdaptor`，形成与同步 Relay 平行的一套异步代理模型。

# Relay 重写研究

## 目标

研究 relay 层中的格式转换与上游转发逻辑，是否可以被完整抽取出来，使其不再依赖数据库或 Redis。

简短结论：可以，但正确的目标不是原样搬走当前的 `relay/` 包。更合理的方向是拆出一个只负责协议转换和上游传输的 relay 执行核心，而将路由、计费、重试策略、持久化和缓存等关注点继续保留在应用层。

## 当前状态

当前的 relay 流程在同一条请求路径中混合了多种职责：

- 请求校验与请求体处理
- 敏感词检查
- token 估算与定价
- 预扣费与退款
- 渠道选择与重试
- 请求格式转换
- 上游 HTTP/WebSocket 转发
- 响应解析
- 后置计费

这一点在 [controller/relay.go](../controller/relay.go) 中表现得最明显，这个顶层 relay 入口几乎编排了整个生命周期。

各个模式下的 relay helper 也将转换/转发与计费结算混在一起：

- [relay/compatible_handler.go](../relay/compatible_handler.go)
- [relay/responses_handler.go](../relay/responses_handler.go)
- [relay/embedding_handler.go](../relay/embedding_handler.go)

adaptor 层其实已经很接近一个可复用核心了，但它仍然依赖框架和应用状态：

- [relay/channel/adapter.go](../relay/channel/adapter.go)
- [relay/channel/api_request.go](../relay/channel/api_request.go)
- [relay/channel/openai/adaptor.go](../relay/channel/openai/adaptor.go)

## 主要耦合点

### 1. `controller/relay.go` 是一个编排型巨石

当前 controller 在一条路径里同时做了下面这些事：

- 解析请求
- 构建 `RelayInfo`
- 敏感检查
- token 估算
- 定价
- 预扣费
- 重试循环
- 渠道选择
- 协议分发
- 错误归一化
- 退款与违规费用处理

这意味着格式转换和转发并没有与业务逻辑隔离开。

### 2. Relay helper 直接执行结算

在 `DoResponse` 之后，helper 会直接调用如下结算逻辑：

- `service.PostTextConsumeQuota(...)`
- `service.PostAudioConsumeQuota(...)`

这使得 helper 层不适合作为一个纯粹的传输/转换核心。

### 3. `RelayInfo` 过于庞大且过于应用化

[relay/common/relay_info.go](../relay/common/relay_info.go) 中的内容非常杂，混合了：

- 用户 / token / 订阅 / 计费状态
- 渠道元数据
- 请求协议元数据
- 流状态
- task 状态
- 价格快照
- 请求转换状态

只要 adaptor 和请求执行仍依赖这个完整结构，relay 逻辑就会持续与应用状态紧耦合。

### 4. Adaptor 依赖 `gin.Context`

adaptor 接口几乎到处都在接收 `*gin.Context`。这使得：

- 请求转换
- 请求头构造
- 上游请求执行
- 响应解析

都与 HTTP 框架耦合在一起，尽管其中大部分逻辑本质上并不依赖框架。

### 5. Transport 当前依赖应用服务

`relay/channel/api_request.go` 使用了很多应用层设施，例如：

- `service.GetHttpClient()`
- `service.NewProxyHttpClient(...)`
- Gin 的响应头设置
- 与请求生命周期绑定的 ping 行为

这说明 transport 应该独立成层，并通过依赖注入获得运行能力。

## 哪些部分可以干净地抽出来

下面这些部分很适合被抽取到一个 relay core 中：

- OpenAI / Claude / Gemini / 各 provider 专有 schema 之间的请求格式转换
- 上游 URL 构造
- 上游请求头构造
- 参数覆盖逻辑
- 出站请求体构造
- 上游 HTTP/WebSocket 执行
- 流式事件解析
- 将 provider 响应归一化为统一的内部结果

这些部分大多是无状态的，或者主要由配置驱动。

## 哪些部分应该留在 core 外面

下面这些关注点应继续留在应用层：

- 基于数据库的渠道选择
- 基于 Redis 的 affinity / cache / 限流协调
- 预扣费 / 结算 / 退款
- 用户 / token / 订阅状态
- 敏感词检查
- 重试策略决策
- 请求日志与审计
- Gin 场景下的 HTTP 响应写回细节
- 状态码映射策略

换句话说：core 不应该知道渠道来自哪里、额度如何扣减，也不应该知道 Redis 是否参与了运行。

## 推荐的目标架构

将 relay 运行时拆成四层。

### 1. `relaycore/schema`

职责：

- 统一的请求与响应类型
- 与 provider 无关的 usage 结构
- endpoint kind / relay mode 的表达

不应包含：

- 用户身份
- 配额或计费状态
- DB 或 Redis 元数据

### 2. `relaycore/provider`

职责：

- 将入口请求标准化为 provider 无关的模型
- 构造 provider 专有的上游请求
- 解析 provider 专有的上游响应
- 归一化流式事件

示例职责：

- `BuildRequest(...)`
- `ParseResponse(...)`
- `ParseStream(...)`

不应负责：

- 计费
- 渠道查找
- 持久化

### 3. `relaycore/transport`

职责：

- HTTP 请求执行
- WebSocket 拨号
- 通过注入接口选择支持代理的 client
- content-length 处理
- 请求超时与传输层错误整形

应依赖如下抽象：

- `HTTPDoer`
- `WSDialer`

而不是直接依赖 `service.GetHttpClient()`。

### 4. 应用编排层

职责：

- 从 DB / cache / config 构建执行计划
- 选择渠道
- 执行重试
- 做定价与计费
- 调用 relay core
- 结算 usage
- 写回框架相关响应

这一层应继续留在当前应用中，并成为业务策略的拥有者。

## 推荐的数据边界

这次重写里最重要的一步，是停止把完整的 `RelayInfo` 传给所有地方。

应当把它拆成更窄、更明确的对象。

### 建议的 `ExecutionPlan`

只包含上游执行配置，例如：

- provider
- endpoint kind
- base URL
- API key
- API version
- upstream model
- request method
- stream 标记
- header overrides
- param overrides
- provider settings
- proxy 配置

这个对象应该由应用层基于 DB / cache / config 构造出来，然后作为纯数据传给 relay core。

### 建议的 `RequestMeta`

包含转换 / 传输可能需要的请求元数据，例如：

- method
- path
- incoming headers
- request ID
- client IP
- content type
- accept header

它可以替代 core 中大部分 `gin.Context` 的用途。

### 建议的 `BillingContext`

包含：

- user ID
- token ID
- subscription 信息
- pre-consume quota
- billing snapshots

这部分应完全留在 core 之外。

## 推荐的接口演进方向

当前的 adaptor 抽象是一个不错的起点，但它应该逐步演进为更小、更聚焦、并且输入与框架无关的接口。

一种可能的方向：

```go
type Provider interface {
    BuildRequest(meta RequestMeta, plan ExecutionPlan, req UnifiedRequest) (*UpstreamRequest, error)
    ParseResponse(meta RequestMeta, plan ExecutionPlan, resp *http.Response) (*UnifiedResponse, error)
}
```

或者拆分得更清晰一些：

```go
type RequestBuilder interface {
    Build(meta RequestMeta, plan ExecutionPlan, req UnifiedRequest) (*UpstreamRequest, error)
}

type ResponseParser interface {
    Parse(meta RequestMeta, plan ExecutionPlan, resp *http.Response) (*UnifiedResponse, error)
}
```

这样可以保持 provider 逻辑的可复用性，而不会把 Gin 或计费状态一起拖进来。

## 关于 `RelayContext` 与处理链的进一步设计

在前面的基础上，一个很自然的方向是引入“请求链 + 共享 `RelayContext` + 可插拔中间件”的模型，用它来承载格式转换与上游 API 转发流程。

总体判断：这个方向是合适的，但它成功的关键不在于“有没有处理链”，而在于：

- `RelayContext` 是否足够瘦
- pipeline 是否只服务于 relay domain
- 中间件边界是否清晰

### 为什么处理链模型适合 relay

relay 运行时本身就是一个多阶段过程，天然适合建模为 pipeline，例如：

1. 标准化入口请求
2. 模型映射
3. provider request conversion
4. header / URL / body 构造
5. upstream forwarding
6. response parsing
7. usage 提取
8. response normalization

相比继续扩大 adaptor 接口，这种处理链更容易将职责拆细，也更适合“多协议入口 + 多 provider 输出”的多对多映射场景。

### `RelayContext` 的定位

`RelayContext` 应当被定义为“本次 relay 执行链共享的运行态”，而不是“整个请求生命周期的所有状态”。

更准确地说，可以把相关状态拆成三层语义：

- `AppContext` / `RequestScope`
- `RelayContext`
- `ExecutionPlan`

其中：

- `AppContext` / `RequestScope` 留在外层编排层，承载用户、token、quota、subscription、Gin 上下文、request id、retry policy 等应用态信息
- `ExecutionPlan` 表示静态执行配置，例如 provider、endpoint、baseURL、apiKey、model、header override、param override、proxy、provider settings
- `RelayContext` 只承载 relay pipeline 在执行过程中真正需要共享的状态

换句话说：

- `AppContext` 决定“要不要做”
- `ExecutionPlan` 决定“怎么打到上游”
- `RelayContext` 记录“这条链执行到了哪一步，以及中间产物是什么”

### 如何避免把 `RelayContext` 重新做成另一个 `RelayInfo`

最重要的原则是：`RelayContext` 不要持有数据库模型实例，也不要持有持久化与缓存客户端。

尤其不应该直接放入：

- `model.Channel`
- `model.Token`
- `model.User`
- `gorm.DB`
- Redis client
- 带强业务语义的 billing session

一旦把这些对象塞进去，后续中间件就会非常容易顺手读取模型字段、调用 DB 更新逻辑，最终让 `RelayContext` 再次膨胀成一个新的“万能状态包”。

### `RelayContext` 中更适合放什么

`RelayContext` 不必只包含 `Usage`，但应该只包含 relay 执行链真正需要共享的纯运行态数据，大体可以分成三类：

- 输入态
- 中间态
- 输出态

输入态例如：

- 标准化请求
- 请求元信息
- 执行计划快照

中间态例如：

- provider request object
- 编码后的 request body
- upstream request
- upstream response
- stream parser state

输出态例如：

- normalized response
- usage
- provider metadata
- conversion trace
- error

适合放进 `RelayContext` 的字段通常包括：

- `RelayFormat`
- `RelayMode`
- `IsStream`
- `OriginModelName`
- `UpstreamModelName`
- `Request`
- `ProviderRequest`
- `RequestBody`
- `UpstreamRequest`
- `UpstreamResponse`
- `Response`
- `Usage`
- `ShouldIncludeUsage`
- `DisablePing`
- `InputAudioFormat`
- `OutputAudioFormat`
- `RequestConversionChain`
- `FinalRequestRelayFormat`
- `Error`
- `Trace` / `Timing`

不适合放进 `RelayContext` 的通常包括：

- `*gin.Context`
- `model.Channel`
- `model.User`
- `model.Token`
- `BillingSession`
- `gorm.DB`
- Redis handle
- 渠道 affinity cache
- 订阅对象

### 渠道信息应该如何进入 pipeline

不是完全不允许渠道信息进入 pipeline，而是不应该把“数据库模型实例”直接传进去。

更合适的方式是将渠道模型投影成一个纯执行快照，例如：

- provider
- channel type
- baseURL
- apiKey
- apiVersion
- proxy
- upstream model
- header override
- param override
- provider settings

也就是说，pipeline 应该消费的是 `ExecutionPlan`，而不是 `model.Channel`。

### 一个简单而实用的约束

如果某个 `RelayContext` 定义文件需要直接 import：

- `model`
- `gorm`
- `redis`
- `gin`

通常就说明它的边界已经开始变脏了。

## 关于 pipeline 中间件的边界

虽然可以借鉴 web 框架的 middleware 形式，但不建议完全照搬一个扁平的 `next()` 链。

更推荐的是“阶段化 pipeline”，也就是：

- 先定义明确的 stage
- 再允许 stage 内部挂中间件

例如可以有：

1. Normalize Stage
2. Convert Stage
3. Build Stage
4. Transport Stage
5. Parse Stage
6. Finalize Stage

这样比把所有逻辑扁平串在一起更清楚，也能避免中间件职责逐渐模糊。

### 哪些逻辑适合进入 relay pipeline

适合放入 pipeline 的，通常是只关心 relay domain 的逻辑，例如：

- 请求标准化
- provider capability 处理
- 上游 URL / header / body 构造
- upstream request 执行
- stream event 解析
- usage 提取
- response normalization

### 哪些逻辑不适合进入 relay pipeline

如果某段逻辑需要依赖下列能力，它通常更适合留在外层 orchestrator：

- `model.*`
- `service.*` 中的计费、订阅、渠道选择
- Redis
- GORM
- Gin response writer

这些都属于“应用编排”范畴，而不是 relay core 本身。

## 关于 Event Callback / Hook 的设计

如果希望在 relay pipeline 执行过程中，在某些节点调用 `service` 中与数据库相关的方法，那么通过 event callback 或 hook 机制让“调用者自己插入副作用逻辑”是一个合理的思路。

但这里更推荐把它设计成“受控的生命周期 hook”，而不是一个任意 callback 容器或泛化 event bus。

### 为什么 hook 机制适合这个场景

核心矛盾是：

- relay core 不应直接依赖数据库和 service
- 但某些关键节点又确实需要触发外层业务行为

例如：

- 请求开始时记录日志
- 上游响应成功后做 post-consume
- 上游失败后做 refund
- 重试前记录失败原因
- 渠道命中后记录统计
- 首包到达时做指标上报

这些动作都不是 relay core 的职责，但 relay core 最清楚这些事件何时发生。因此一个自然的做法是：

- core 负责发出生命周期信号
- 外层决定是否监听，以及如何处理

### 更推荐 Hook，而不是通用 Event Bus

这个流程是强时序、强领域语义的，所以相比：

- 任意字符串事件名
- 任意载荷
- 无边界广播

更推荐定义明确、数量受控的生命周期 hook，例如：

- `OnPrepared`
- `OnBeforeSend`
- `OnResponse`
- `OnUsage`
- `OnError`
- `OnComplete`

这种方式的好处是：

- 事件集合有限
- 语义稳定
- 更容易维护兼容性
- 调用顺序清晰
- 不容易退化成“什么都能往里塞”的总线系统

### 同步 hook 与异步 event 要分开

这里有一个很关键的设计点：要明确区分两类扩展点。

第一类是同步 hook：

- 在主链路中执行
- 可以返回错误
- 可能影响流程

第二类是异步 event：

- 只是通知
- 不应影响主链路
- 更适合做日志、指标、审计

如果不做这个区分，很快就会遇到这些问题：

- 某个日志 callback 失败了，主请求是否应该失败
- 某个 billing callback 超时了，是否应该阻塞用户请求
- 某个审计 callback panic 了，relay 是否应该中断

因此应该在设计上预先划定边界。

### 哪些事情适合走 hook / event

适合的通常是“副作用型扩展点”，例如：

- 记录日志
- 指标上报
- usage 持久化
- 触发 billing settle / refund
- 记录渠道命中
- 记录失败分类
- 写审计信息

### 哪些事情不适合走 hook / event

不适合暴露给 callback 决定的，通常是 relay 主流程中的核心控制逻辑，例如：

- provider 请求如何构造
- 是否继续重试
- 响应按什么格式解析
- stream 如何转发
- 上游 URL 最终是什么

这些都应继续由 pipeline 或 orchestrator 的正式逻辑掌控，而不是被 callback 反向控制。

### callback 参数也要克制

即便采用 hook 机制，也不应让 callback 拿到“全能上下文”。

如果 callback 能直接接触：

- `*gin.Context`
- `*RelayContext`
- `*AppContext`
- `*model.Channel`
- `BillingSession`

那么 core 虽然表面没有 import `model`，实际上已经通过 callback 被外层彻底侵入。

更健康的做法是尽量传递：

- 只读快照
- 明确的事件载荷
- 最少必要信息

例如比起把整套上下文全部传出去，更好的方式是传一个像下面这样的事件对象：

- request id
- user id
- channel id
- model
- usage
- retry index

如果外层需要更多业务状态，应该由外层 orchestrator 自己补齐，而不是让 relay core 替它背负全部上下文。

## Task 类型 Relay 是否适合 Pipeline

Task 类型的 relay 同样适合 pipeline 化，但更准确地说：

- 适合复用“pipeline 机制”
- 不适合和同步 relay 共用完全相同的一条 pipeline

原因不是 task 不适合分阶段处理，而是它的生命周期与同步请求有本质差异。

### 为什么 Task relay 适合 pipeline

Task 类型 relay 同样有非常稳定的阶段边界，例如：

1. 解析并校验任务请求
2. 推导 action / task 类型
3. 估算 billing 参数
4. 构造上游提交请求
5. 提交任务到上游
6. 解析 submit response
7. 生成统一 task 返回体
8. 进入异步轮询或后续查询路径

这本身就非常适合用 pipeline 表达。

而且现有 `TaskAdaptor` 已经天然带有阶段感，例如：

- `ValidateRequestAndSetAction`
- `EstimateBilling`
- `BuildRequestURL`
- `BuildRequestHeader`
- `BuildRequestBody`
- `DoRequest`
- `DoResponse`
- `FetchTask`
- `ParseTaskResult`

所以从结构上说，Task relay 很适合作为 pipeline family 单独抽象出来。

### 为什么不能和普通同步 relay 强行共用同一条 pipeline

同步 relay 多数是：

- 一次请求
- 一次上游响应
- 当场解析 usage
- 当场完成结算或直接返回

而 Task relay 通常是：

- 一次提交
- 返回 task id
- 后续异步轮询
- 最终完成态可能出现在另一个请求、另一个 goroutine，甚至另一个进程中

这会带来几个根本差异：

第一，Task submit 的输出通常不是最终业务结果，而只是“任务已提交”。

第二，它的生命周期天然被拆成两段甚至更多段：

- Submit Pipeline
- Poll / Complete Pipeline

第三，它的计费时机通常更加复杂：

- 提交前预扣
- 提交后可能微调预估 billing
- 完成态再最终结算
- 失败时退款

第四，它的上下文语义也不同。同步 relay 更像单请求上下文，而 Task relay 更适合拆成：

- `TaskExecutionPlan`
- `TaskSubmitContext`
- `TaskPollContext`

否则一个上下文同时承载“提交态”和“完成态”，很容易再次膨胀。

## 三套 Pipeline 的最小拆分建议

整体上更适合拆成三套链：

- Sync Relay Pipeline
- Task Submit Pipeline
- Task Poll / Complete Pipeline

三者可以共用：

- pipeline engine
- middleware model
- hook model
- error model
- trace / timing model
- `ExecutionPlan` 风格的数据边界
- transport 抽象

但不应强求共用：

- 同一份大 `RelayContext`
- 同一组 stage 枚举
- 同一个最终结果结构

### 1. Sync Relay Pipeline

用于普通同步请求，例如 text / embedding / image / audio 这类“一次请求，一次响应”的链路。

建议 stage：

1. `NormalizeInput`
2. `PreparePlan`
3. `ConvertRequest`
4. `BuildUpstreamRequest`
5. `SendUpstreamRequest`
6. `ParseUpstreamResponse`
7. `FinalizeResult`

各阶段职责大致如下：

`NormalizeInput`

- 将 OpenAI / Claude / Gemini 等入口请求整理成统一请求对象
- 只做协议归一，不做计费和选路

`PreparePlan`

- 消费外层给定的 `ExecutionPlan`
- 确定 provider capability、stream flag、model mapping 的最终执行值
- 只能读 plan，不能查 DB

`ConvertRequest`

- 调 provider adaptor，将统一请求转换成 provider request

`BuildUpstreamRequest`

- 构造 URL、headers、body、form、ws 参数等

`SendUpstreamRequest`

- 发 HTTP / WS 请求

`ParseUpstreamResponse`

- 解析 provider response / stream
- 抽取 usage
- 归一化错误

`FinalizeResult`

- 形成统一结果对象并交回 orchestrator

它和现有同步 adaptor 的映射大致是：

- `ConvertOpenAIRequest`
- `ConvertClaudeRequest`
- `ConvertGeminiRequest`
- `GetRequestURL`
- `SetupRequestHeader`
- `DoRequest`
- `DoResponse`

### 2. Task Submit Pipeline

用于异步任务提交，例如视频、音乐或部分图片任务。

建议 stage：

1. `NormalizeTaskInput`
2. `ValidateTaskRequest`
3. `ResolveTaskAction`
4. `EstimateTaskBilling`
5. `BuildSubmitRequest`
6. `SubmitTask`
7. `ParseSubmitResponse`
8. `FinalizeSubmitResult`

各阶段职责大致如下：

`NormalizeTaskInput`

- 将入口任务请求整理成统一 task request

`ValidateTaskRequest`

- 做 provider 相关参数校验

`ResolveTaskAction`

- 推导本次 action，例如 create / extend / variation 等

`EstimateTaskBilling`

- 提取时长、分辨率、路数等 billing 因子
- 只产出估算参数，不直接扣费

`BuildSubmitRequest`

- 构造 submit URL / headers / body

`SubmitTask`

- 发起上游提交

`ParseSubmitResponse`

- 解析 task id、provider task payload、初始状态

`FinalizeSubmitResult`

- 生成统一 submit result，交回外层
- 外层再决定是否落库以及是否微调 billing

它和现有 `TaskAdaptor` 的映射非常自然：

- `ValidateRequestAndSetAction`
- `EstimateBilling`
- `BuildRequestURL`
- `BuildRequestHeader`
- `BuildRequestBody`
- `DoRequest`
- `DoResponse`

### 3. Task Poll / Complete Pipeline

用于任务轮询、查询以及最终完成态处理。

建议 stage：

1. `LoadPollInput`
2. `BuildPollRequest`
3. `FetchTaskState`
4. `ParseTaskState`
5. `NormalizeTaskResult`
6. `FinalizeTaskCompletion`

各阶段职责大致如下：

`LoadPollInput`

- 接收外层传入的 task identity、provider info、poll 参数
- 不直接查 DB，DB 读取应由 orchestrator 先完成

`BuildPollRequest`

- 组装查询 URL / headers / body / proxy 配置

`FetchTaskState`

- 调上游查询任务状态

`ParseTaskState`

- 解析 provider response，得到统一 `TaskInfo`

`NormalizeTaskResult`

- 生成统一 task result
- 可以顺便产出完成态 usage-like 信息或 completion metadata

`FinalizeTaskCompletion`

- 输出标准 completion result 给外层
- 外层再决定是否更新任务状态、是否补扣或退款、是否记录日志

它和现有 task 轮询相关接口的映射主要是：

- `FetchTask`
- `ParseTaskResult`
- `AdjustBillingOnComplete`

其中 `AdjustBillingOnComplete` 更适合保留为完成态结果的一部分，或由外层在 `FinalizeTaskCompletion` 之后调用，而不是深埋进 core stage 内部逻辑中。

## Task Pipeline 中更适合暴露 Hook 的位置

Task 类型 relay 比同步 relay 更适合使用 hook，因为它天然带有跨阶段副作用：

- 提交成功后创建任务记录
- 提交后修正预估 billing
- 任务完成后最终结算
- 任务失败后退款
- 状态变化时打日志 / 审计
- provider payload 落库

这些动作都不应该直接塞进 core pipeline 本体，更适合采用：

- pipeline 负责提交 / 查询 / 解析 / 归一化
- 外层 hook 负责落库 / 计费 / 状态迁移 / 审计

## 关于 Task Context 的建议

不建议做一个覆盖全部 task 生命周期的大一统 `TaskRelayContext`。

更稳妥的做法是拆成：

- `TaskExecutionPlan`
- `TaskSubmitContext`
- `TaskPollContext`

其中：

`TaskExecutionPlan`

- provider
- task type
- action
- baseURL
- apiKey
- proxy
- model
- provider settings

`TaskSubmitContext`

- 标准化 task 请求
- submit request / response
- task id
- submit result
- submit trace

`TaskPollContext`

- task identity
- poll request / response
- parsed task result
- final status
- completion metadata

这样可以避免把 submit request、submit response、task id、poll response、final completion、billing delta 全部硬塞进一个上下文。

## 迁移策略

不要试图一次性重写整个 relay 子系统。风险最低的路径是渐进式迁移。

### Phase 1. 抽出一个最小 relay core 外壳

新建一个包，例如 `pkg/relaycore` 或 `relay/core`，先放入：

- `ExecutionPlan`
- `RequestMeta`
- `Result`
- provider 接口
- transport 接口

在这个阶段，继续复用现有 DTO 是完全可以接受的，这样能避免第一次迁移范围过大。

### Phase 2. 优先迁一条阶段感最强的链路

如果目标是尽快验证 pipeline 语义，一个很有吸引力的起点其实是 Task Submit Pipeline，因为现有 `TaskAdaptor` 已经天然按阶段拆得比较清楚。

原因：

- `TaskAdaptor` 当前接口和 pipeline stage 映射最自然
- 更容易先验证 stage + hook 的组织方式
- 能较早检验“提交态”和“异步完成态”边界是否清晰

一个可行的起点是 task submit 相关适配层。

不过如果目标是优先覆盖面和复用价值，那么 OpenAI-compatible 文本 relay 仍然是同步链路中的最佳第一候选。

原因：

- 复用价值最高
- provider 覆盖最广
- 能同时覆盖请求转换、转发、响应解析和 usage 提取

对应起点可以是：

- [relay/compatible_handler.go](../relay/compatible_handler.go)
- [relay/channel/openai/adaptor.go](../relay/channel/openai/adaptor.go)

### Phase 3. 抽 transport 公共能力

把下面文件中的通用请求执行行为抽出来：

- [relay/channel/api_request.go](../relay/channel/api_request.go)

目标是隔离出：

- URL 构造
- header override 逻辑
- request 创建
- HTTP 执行
- WebSocket 拨号

同时移除它对 Gin 和应用层全局 service 的直接依赖。

### Phase 4. 让 controller 流程退化为纯编排

重构 [controller/relay.go](../controller/relay.go)，使它变成：

1. 解析并校验请求
2. 构建定价和计费状态
3. 选择路由 / 渠道
4. 构建 `ExecutionPlan`
5. 调用 relay core
6. 结算 usage 或退款
7. 写回最终响应 / 错误

到这一步，架构分层会变得非常清晰。

### Phase 5. 统一补齐剩余 pipeline family

在第一条链路验证完成之后，再补齐剩余两类复杂路径：

- SSE 流式
- WebSocket realtime
- Task Poll / Complete Pipeline

这些路径的生命周期更重，也更容易在早期重构中再次与应用编排耦合，因此更适合作为后续阶段处理。

## 一个实用的重写原则

relay core 绝不能知道渠道来自 `model.Channel`，也不应该知道当前应用底层使用的是 SQLite / MySQL / PostgreSQL / Redis。

应用层应当先把所有依赖持久化的数据完整解析成纯执行输入，再调用 core。

换句话说：

- 应用层拥有选路和业务策略
- relay core 拥有协议转换与上游执行

## 最终判断

这件事是可行的，而且值得做。

成功的正确定义不是：

- “把当前 `relay/` 包原样搬到别处”

成功的正确定义应该是：

- “把应用编排和协议转换 / 上游执行彻底分开”

如果做得好，结果会是：

- provider 维护更容易
- 测试更干净
- 框架耦合更少
- 能形成一个可复用的 relay engine
- 计费与路由边界更清晰

## 建议的第一个具体动作

先引入一个更窄的 `ExecutionPlan` 和 `RequestMeta`，然后选一条 OpenAI-compatible 文本链路，让它先通过新的 core 入口执行，同时继续把计费和渠道选择保留在现有应用层。

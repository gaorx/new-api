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

## Relay 的请求生命周期

以 `/v1/chat/completions` 为例，主流程大概是：

1. `TokenAuth` 校验调用令牌
2. `Distribute` 解析请求中的模型和分组，选出渠道
3. `controller.Relay()` 读取请求体并校验格式
4. `GenRelayInfo()` 组装上下文
5. 敏感词检查、token 预估、价格计算
6. `PreConsumeBilling()` 预扣费
7. 获取渠道并构建适配器
8. 适配器转换请求并调用上游
9. 解析 usage / 错误 / 流式结果
10. 成功则结算，失败则退款并视情况重试

这个流程里有两个很重要的架构点：

1. 请求体会被缓存成可重复读取的 body storage，方便重试时重复发送。
2. 计费发生在调用前后两个阶段：先预扣，再按实际 usage 结算。

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

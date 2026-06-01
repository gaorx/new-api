# 04 - 核心概念与关系

## 为什么这一节重要

这个项目真正的复杂度，不在某个 provider 的单独接入，而在一组核心概念叠加之后形成的运行模型。

## User

`model/User` 是平台用户，而不是上游供应商用户。

它承载：

- 平台登录身份
- 用户分组 `Group`
- 配额 `Quota`
- OAuth 绑定
- 用户设置 `Setting`
- 邀请、支付、订阅等平台侧信息

用户是“谁在使用平台”的主体。

## Token

`model/Token` 是 API 调用凭证，是用户对外调用 Relay API 的授权载体。

它承载：

- 属于哪个用户 `UserId`
- 还有多少剩余额度 `RemainQuota`
- 限制哪些模型 `ModelLimits`
- 使用哪个逻辑分组 `Group`
- 是否允许跨分组重试 `CrossGroupRetry`

Token 不是“渠道 key”，而是“平台颁发给下游调用者的 key”。

## Group

`Group` 是平台内部非常核心的抽象，它不是用户组这么简单，而是同时参与：

- 用户可见/可用资源隔离
- 令牌路由范围控制
- 模型倍率和组间倍率计算
- auto-group 自动路由
- 渠道能力过滤

在这个系统里，`Group` 更像“资源池 / 业务线 / 价格策略边界”。

## Channel

`model/Channel` 表示一个上游接入渠道。

它通常对应：

- 某个供应商账户
- 某个代理服务
- 某个 API Key 集
- 某个特定 Base URL / 区域 / 版本

Channel 保存了上游访问所需的关键材料：

- `Type`
- `Key`
- `BaseURL`
- `Models`
- `Group`
- `ModelMapping`
- `Setting`
- `HeaderOverride`
- `ChannelInfo`（多 key 模式等）

Channel 是“如何连到上游”的实体。

## Ability

`model/Ability` 是这个项目很关键、也很容易忽略的概念。

它把 `Channel` 展开成：

```text
(group, model, channel_id) -> enabled / priority / weight / tag
```

也就是说，Ability 不是独立业务对象，而是“渠道在某个分组下对某个模型可服务能力”的索引层。

关系可以理解为：

```text
Channel --展开--> 多条 Ability
Ability = Channel 在某 Group 下支持某 Model 的能力记录
```

系统选渠道时，不是直接扫描 `Channel.Models` 字符串，而是优先使用 `Ability`。

## Option / Setting

系统配置有两层：

1. 环境变量
2. 数据库 `options` 表

`model/Option` + `common.OptionMap` + `setting/config/ConfigManager` 共同构成配置中心。

特点：

- 启动时先加载默认值和环境变量
- 再从 `options` 表覆盖
- 后台定时 `SyncOptions()` 热更新
- 某些模型配置通过 `setting/config` 的反射注册机制统一管理

所以这个项目的很多“系统行为”其实是运行时可改的，而不是写死在代码里。

## Pricing / Billing

计费并不是“请求成功就扣点 quota”这么简单，而是一套完整机制：

- 价格来源：倍率、固定单价、表达式计费
- 扣费方式：钱包额度 / 订阅额度
- 扣费时机：预扣 -> 实际结算 -> 失败退款

关键对象：

- `types.PriceData`
- `relay/common/BillingSettler`
- `service/BillingSession`
- `billingexpr` 表达式系统

## 概念关系图

最重要的一张关系图可以这样看：

```text
User
  |
  +-- owns --> Token
  |              |
  |              +-- chooses/limits --> Group
  |              +-- limits --> Model set
  |
  +-- has --> Quota / Subscription / Settings

Channel
  |
  +-- expands into --> Ability(group, model, channel_id)
  |                         |
  |                         +-- used by --> Distributor / Channel Selector
  |
  +-- contains --> upstream key / base_url / model_mapping / overrides

Incoming Request
  |
  +-- authenticated by --> Token
  +-- scoped by --> User + Group + Model
  +-- resolved to --> Channel
  +-- wrapped as --> RelayInfo
  +-- billed by --> BillingSession
  +-- executed by --> Adaptor
  +-- recorded as --> Log / Metrics / Quota Data
```

如果换一句更工程化的话：

- `User/Token` 决定“谁有资格请求什么”
- `Group/Ability/Channel` 决定“请求应该发到哪里”
- `PriceData/BillingSession` 决定“这次请求怎么收费”
- `RelayInfo/Adaptor` 决定“这次请求怎么执行”

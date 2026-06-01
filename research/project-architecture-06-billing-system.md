# 06 - 计费体系

## 为什么计费体系复杂

这个项目的计费不是“请求成功就扣一下额度”，而是一套完整的会话化计费模型。

## 预扣 + 结算

统一流程是：

1. 根据请求估算 prompt tokens / max tokens
2. 计算预扣额度
3. 请求发出前先预扣
4. 成功后根据实际 usage 结算差额
5. 失败时退款

这套机制封装在 `service/BillingSession` 中。

## 资金来源

从代码看，至少有两种资金来源：

- 钱包额度
- 订阅额度

所以 BillingSession 不是单纯扣 Token 的 `RemainQuota`，而是同时协调：

- 用户资金来源
- 平台 Token 额度
- 订阅使用量

## 三种定价方式

项目中同时存在三种价目表达方式：

1. 固定价格 `ModelPrice`
2. 倍率计费 `ModelRatio + GroupRatio + CompletionRatio + CacheRatio ...`
3. 表达式计费 `tiered_expr`

其中表达式计费最灵活，也最先进。

## 表达式计费系统

`pkg/billingexpr/expr.md` 描述了一套完整的“定价 DSL”：

- 以真实 $/1M tokens 价格表达
- 支持 `p/c/cr/cc/img/ai/ao` 等变量
- 支持 `tier()`、`param()`、`header()` 等函数
- 支持按请求参数附加规则
- 支持预扣和结算时用同一表达式重算

这一层让项目从“配置倍率”升级成了“可编程计费引擎”。

## 从请求到计费的链路

可以粗略理解为：

1. 请求进入 Relay
2. 根据模型和分组计算价格
3. 建立 `BillingSession`
4. 请求前预扣
5. 请求后按实际 usage 补扣或退款
6. 写入日志、统计和订阅用量

## 计费相关核心对象

- `types.PriceData`
  当前请求的价格计算结果。
- `relay/common/BillingSettler`
  统一的结算接口。
- `service/BillingSession`
  单次请求的预扣、结算、退款生命周期。
- `billingexpr`
  动态定价表达式引擎。

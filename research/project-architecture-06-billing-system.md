# 06 - 计费体系

## 为什么计费体系复杂

这个项目的计费不是“请求成功就扣一下额度”，而是一套完整的会话化计费模型。

这里最重要的前置认知是：

- 平台对外常说“token”“额度”，但内部真正统一结算的是 `quota`
- `Token.RemainQuota` 记录的也是 quota，而不是上游模型 API 返回的原始 token 数

因此“消耗了多少 token”与“最终扣了多少 quota”之间，始终隔着一层换算逻辑。

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

## Quota 不是 1:1 扣的

这个项目里，原始 usage 通常不会被直接按 `1 token = 1 quota` 扣减。

更常见的是先经历如下换算：

```text
prompt/completion/cache/audio/image/tool usage
  -> 结合模型定价规则
  -> 叠加分组倍率
  -> 折算成 quota
```

对于常见文本模型，实际结算思路可以概括成：

```text
quota ≈
(
  输入 tokens
  + 输出 tokens * completion_ratio
  + cache / cache write / image / audio / tool surcharge 等修正
)
* model_ratio
* group_ratio
```

如果模型走固定价格模式，则更接近：

```text
quota = model_price * QuotaPerUnit * group_ratio
```

所以：

- 不是固定 1:1
- 同样的 usage，在不同模型下会扣不同 quota
- 同样的 usage，在不同分组下也会扣不同 quota
- 某些用户组到资源分组之间还可能有 `GroupGroupRatio` 特殊倍率

## Quota 和原始 token 数的大小关系

`quota` 通常是“真实 usage token 数经过计费规则缩放后的结果”，但它**不保证一定大于**原始 token 数。

它可能出现几种情况：

- 等于原始 token 数
  当 `model_ratio = 1`、`completion_ratio = 1`、`group_ratio = 1` 且没有其他修正项时。
- 大于原始 token 数
  这是很常见的情况，尤其是在输出倍率、模型倍率或分组倍率大于 `1` 时。
- 小于原始 token 数
  也完全可能，例如便宜模型 `model_ratio < 1`，或者某个分组设置了折扣倍率 `< 1`。
- 等于 `0`
  免费模型、免费分组，或者关闭免费模型预扣时都可能出现。

所以更准确的理解应该是：

```text
quota = usage 的计费映射结果
而不是 usage.total_tokens 的简单别名
```

一个简化例子：

- 假设输入 `1000`、输出 `500`
- 若 `model_ratio = 1`、`completion_ratio = 2`、`group_ratio = 1`
- 则 `quota = (1000 + 500 * 2) * 1 * 1 = 2000`
- 这时原始 `usage.total_tokens = 1500`，所以 quota 比 token 数大

但如果换成更便宜的模型，例如：

- `model_ratio = 0.2`
- `completion_ratio = 2`
- `group_ratio = 1`
- 则 `quota = (1000 + 500 * 2) * 0.2 = 400`

这时 quota 就明显小于原始 token 数 `1500`。

## `QuotaPerUnit` 的含义

`quota` 不是美元金额，也不是原始 token 数，而是平台内部统一的结算单位。

代码里通过 `QuotaPerUnit` 把“价格”或“倍率结果”换算到 quota 体系中。  
这意味着整个系统是：

```text
上游 usage / 模型价格
  -> 平台内部 quota
  -> 用户余额 / token 额度 / 订阅额度 的统一扣减
```

这也是为什么充值、订阅、请求结算最终都能落到同一套额度系统上。

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

换句话说，系统里真正发生的是：

1. 先用 `Token` 证明“谁在请求”
2. 再用定价规则把 usage 换成 `quota`
3. 最后从用户余额、订阅额度和 token 额度上做统一结算

## 计费相关核心对象

- `types.PriceData`
  当前请求的价格计算结果。
- `relay/common/BillingSettler`
  统一的结算接口。
- `service/BillingSession`
  单次请求的预扣、结算、退款生命周期。
- `billingexpr`
  动态定价表达式引擎。

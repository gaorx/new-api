# 06 - 计费体系

## 为什么计费体系复杂

这个项目的计费不是“请求成功就扣一下额度”，而是一套完整的会话化计费模型。

如果你关心的是“支持哪些支付方式、充值/订阅订单怎么完成、每种支付网关原理有什么差异”，请配合阅读 [project-architecture-15-payment-system.md](/Users/gaorx/Works/my/new-api/research/project-architecture-15-payment-system.md)。

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

## “订阅”在这个项目里到底指什么

这里的“订阅”更准确地说，不是单纯“按月自动扣费”的 SaaS 会员，而是：

- 一份有有效期的套餐模板 `SubscriptionPlan`
- 用户购买后生成的一条订阅实例 `UserSubscription`
- 这条实例在有效期内可被请求消耗的订阅额度

也就是说，项目里的订阅本质上是“带时长、带额度、可选带周期重置”的套餐，而不是天然等同于“月付会员”。

### 有效期和额度重置是两套独立配置

`subscription_plans` 里有两组容易混淆的字段：

- `duration_unit` / `duration_value` / `custom_seconds`
- `quota_reset_period` / `quota_reset_custom_seconds`

第一组决定的是**这份订阅能活多久**。

例如：

- `1 month`
- `1 year`
- `7 days`
- 自定义秒数

第二组决定的是**这份订阅里的额度会不会周期性清零重置**。

可选值包括：

- `never`
- `daily`
- `weekly`
- `monthly`
- `custom`

所以“月套餐”和“每月重置额度”在实现上是两回事：

- 可以是“有效期 1 个月，但额度从不重置”
- 也可以是“有效期 1 年，但额度按月重置”
- 还可以是“有效期 7 天，额度按天重置”

### 默认看起来像月订阅，但不是强约束

默认值确实更接近“月订阅”：

- `duration_unit` 默认是 `month`
- `duration_value` 默认是 `1`

但这只是默认配置，不是系统写死的商业规则。管理员完全可以把套餐配置成按年、按天、按小时，或者自定义秒数。

### “每个月重置 quota”不是必然行为

只有当套餐的 `quota_reset_period = monthly` 时，订阅额度才会按月重置。

而且这里的“按月重置”不是“从购买时间起每 30 天重置一次”，而是按自然月边界对齐：

- 下一个重置点是**下个月 1 号 00:00**

同理：

- `daily` 对齐到次日 `00:00`
- `weekly` 对齐到下一个周一 `00:00`
- `custom` 才是基于当前基准时间直接加秒数

因此，如果用户在月中购买一个“按月重置额度”的年套餐，第一次重置通常会很快到下个月 1 号，而不是等满 30 天。

### 用户买到的是套餐快照，不是永远跟随模板

用户购买后会生成 `user_subscriptions` 记录，其中会固化：

- `amount_total`
- `amount_used`
- `start_time`
- `end_time`
- `last_reset_time`
- `next_reset_time`

这说明购买成功后，系统关心的是“这条订阅实例当前如何消耗和何时过期/重置”，而不是每次都回头重新解释套餐模板。

### 订阅额度和钱包额度是两条并行资金路径

在请求扣费时，系统会根据用户的 `billing_preference` 选择优先走：

- `subscription_only`
- `subscription_first`
- `wallet_first`
- `wallet_only`

因此订阅不是展示层的会员标签，而是实际参与请求预扣和结算的资金来源之一。

### 当前没有统一的自动续费延长订阅机制

从现有代码看，项目里虽然接入了多种支付方式，也能处理订阅购买订单，但没有形成一套统一的“自动续费成功后自动延长用户订阅实例”的核心机制。

一个明显信号是 Waffo Pancake 集成里明确使用 `OnetimeProduct`，并直接注明原因是：

- 目前没有 renewal event handling
- 如果支付侧自动续费，而平台侧不自动延长访问权，会造成体验不一致

所以截至当前代码状态，更准确的结论是：

- 订阅支持“购买并生成一段有效期内的额度实例”
- 但不应默认把它理解为“标准 SaaS 自动续费月会员”

### 一句话总结

这个项目里的“订阅”应该理解为：

```text
有时长限制的额度套餐
  + 可选的周期性额度重置
  + 可参与请求计费的资金来源
```

而不是简单理解成：

```text
每月自动扣款
  + 每月自动重置
```

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

## Token 到 Quota 的共识公式

如果把实现细节抽掉，项目里把原始 usage token 换成 quota 的主路径可以归纳成下面几类。

### 1. 普通文本模型的倍率计费

主实现位于：

- `relay/helper/price.go`
- `service/text_quota.go`

可以概括成：

```text
quota =
(
  promptBase
  + completionTokens * completionRatio
)
* modelRatio
* groupRatio
+ toolSurchargeQuota
+ audioInputSeparateQuota
```

其中 `promptBase` 不是简单的 `prompt_tokens`，而是会把几类特殊输入拆开后分别乘自己的倍率：

```text
promptBase =
  basePromptTokens
  + cacheReadTokens * cacheRatio
  + cacheWriteTokens * cacheCreationRatio
  + cacheWriteTokens5m * cacheCreationRatio5m
  + cacheWriteTokens1h * cacheCreationRatio1h
  + imageTokens * imageRatio
```

如果当前请求还带有 `OtherRatios`，那么在上式算完之后，还会继续整体连乘：

```text
quota = quota * product(OtherRatios)
```

### 2. 音频 / 实时场景的倍率计费

主实现位于：

- `service/quota.go`

可概括成：

```text
quota =
(
  inputTextTokens
  + outputTextTokens * completionRatio
  + inputAudioTokens * audioRatio
  + outputAudioTokens * audioRatio * audioCompletionRatio
)
* modelRatio
* groupRatio
```

这条路径更适合把文本输入、文本输出、音频输入、音频输出分别看成四类计费对象。

### 3. 固定价格模型

如果模型启用了固定价格模式 `usePrice=true`，核心模型费用不再按 token 数线性变化，而是直接走：

```text
quota = modelPrice * QuotaPerUnit * groupRatio
```

如果还带 `OtherRatios`，则同样会继续整体连乘：

```text
quota = modelPrice * QuotaPerUnit * groupRatio * product(OtherRatios)
```

这类模式常见于：

- 按次计费模型
- 图片 / 视频任务模型
- 某些不适合按 token 细分的能力

### 4. Tiered Expression 动态计费

如果模型使用 `tiered_expr` 表达式计费，则先让表达式产出真实价格，再统一折算成 quota：

```text
quota = exprOutput / 1_000_000 * QuotaPerUnit * groupRatio
```

这里的 `exprOutput` 是按 `$ / 1M tokens` 表达的实际价格，不再走 `modelRatio`、`completionRatio` 这一套传统倍率链路。

### 5. GroupGroupRatio 对 GroupRatio 的覆盖

还有一个容易忽略的共识是：

- 默认先取 `groupRatio`
- 如果存在 `userGroup -> usingGroup` 的特殊倍率 `GroupGroupRatio`
- 则实际参与结算的是这个特殊倍率，而不是普通分组倍率

可以把它理解成：

```text
actualGroupRatio = GroupGroupRatio or GroupRatio
```

所以很多公式里写的 `groupRatio`，更准确地说其实是“最终生效分组倍率”。

## 缩放参数字典

下面这些参数就是 token 变成 quota 时最关键的缩放因子。

### 基础倍率

- `modelRatio`
  含义：模型基础倍率。
  作用：普通输入/输出 token 的主倍率基数；文本、音频等倍率计费最终都会乘它。

- `groupRatio`
  含义：当前使用分组的倍率。
  作用：把同一模型在不同分组上的价格拉开；几乎所有计费路径最终都会乘它。

- `GroupGroupRatio`
  含义：用户组到使用分组的特殊覆盖倍率。
  作用：如果命中，它会覆盖普通 `groupRatio`，成为最终生效的分组倍率。

- `QuotaPerUnit`
  含义：价格结果换算成平台内部 quota 的全局系数。
  作用：固定价格模式、动态表达式模式、工具附加费、部分单独价格项最后都靠它落到 quota 体系。

### 输入 / 输出细分倍率

- `completionRatio`
  含义：输出 token 相对输入 token 的倍率。
  作用：把 `completionTokens` 折算成更高或更低的计费权重。

- `cacheRatio`
  含义：缓存命中 token 的倍率。
  作用：把 `cachedTokens` 从普通输入里拆出来单独按缓存读取价计费。

- `cacheCreationRatio`
  含义：缓存写入 token 的默认倍率。
  作用：把 `cache creation tokens` 按缓存写入价计费。

- `cacheCreationRatio5m`
  含义：Claude 5 分钟缓存写入倍率。
  作用：当上游 usage 能区分 5m 缓存写入时，按这个倍率单独计费。

- `cacheCreationRatio1h`
  含义：Claude 1 小时缓存写入倍率。
  作用：当上游 usage 能区分 1h 缓存写入时，按这个倍率单独计费。

- `imageRatio`
  含义：图片输入 token 的倍率。
  作用：把图片输入 token 从普通 prompt token 中拆出来单独计费。

- `audioRatio`
  含义：音频 token 的基础倍率。
  作用：实时/音频模型里用于音频输入和音频输出的基础价格因子。

- `audioCompletionRatio`
  含义：音频输出相对音频输入的补全倍率。
  作用：让 `outputAudioTokens` 在 `audioRatio` 之外再乘一个输出倍率。

### 固定价格参数

- `modelPrice`
  含义：固定价格模型的基础价格。
  作用：启用 `usePrice` 时，不再按 token 数乘 `modelRatio`，而是直接用 `modelPrice * QuotaPerUnit * groupRatio` 算核心费用。

### 任务 / 多媒体附加倍率

- `OtherRatios`
  含义：一组额外附加倍率，通常由任务适配器根据用户请求推导出来。
  作用：在基础 quota 算完后，再整体连乘，适合表达“时长、尺寸、分辨率、张数”这类非 token 维度的价格放大因子。

当前代码里常见的 `OtherRatios` 键包括：

- `seconds`
  含义：视频时长倍率。

- `size`
  含义：尺寸倍率。

- `resolution`
  含义：分辨率倍率。

### 动态表达式参数

在 `tiered_expr` 模式下，传统的 `modelRatio` / `completionRatio` 不再是主导参数，表达式变量本身就是定价参数：

- `p`
  含义：输入 token 数。

- `c`
  含义：输出 token 数。

- `cr`
  含义：缓存读取 token 数。

- `cc`
  含义：缓存写入 token 数。

- `cc1h`
  含义：1 小时缓存写入 token 数。

- `img`
  含义：图片输入 token 数。

- `ai`
  含义：音频输入 token 数。

- `ao`
  含义：音频输出 token 数。

- `len`
  含义：完整输入上下文长度。
  作用：更适合做分档条件判断，而不是直接拿来当价格倍率。

## 需要特别区分的“倍率”和“附加费”

严格说，并不是所有影响最终 quota 的参数都属于“乘法倍率”。

项目里还有几类“先单独算一笔 quota，再加到最终结果里”的附加费：

- Web Search 调用价格
- File Search 调用价格
- Image Generation Call 按次价格
- 某些音频输入的单独价格

因此更完整的理解应该是：

```text
最终 quota
  = 基础 token 费用 * 各类倍率
  + 工具或特殊能力的附加费用
```

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

## `quota`、`used_quota`、`request_count` 不是同一种更新策略

这几个字段都和“请求后状态更新”有关，但实现上并不是统一走一套缓存或落库逻辑。

最容易混淆的是：

- `user.quota`
- `user.used_quota`
- `user.request_count`
- `token.remain_quota` / `token.used_quota`

它们在 relay 完成后的更新路径并不相同。

### `user.quota`：每次请求都会产生变更，但未必立刻写 DB

`user.quota` 表示用户钱包额度。

在请求发生预扣、补扣、退款时，最终都会走：

- `model.IncreaseUserQuota(...)`
- `model.DecreaseUserQuota(...)`

这两个函数的行为是：

1. 如果 Redis 开启，异步更新用户缓存里的 `Quota` 字段。
2. 如果 `BATCH_UPDATE_ENABLED=true`，把变更先记到进程内存中的 batch store。
3. 如果没有开启 batch，则直接执行数据库 `UPDATE quota = quota +/- ?`。

所以更准确的描述应该是：

```text
每次 relay 都会产生 user.quota 的变更
  -> Redis 用户缓存会尽量跟上
  -> DB 可能立即更新，也可能延迟批量刷新
```

这意味着“每次 relay 都更新 quota”是对的，但“每次 relay 都立刻改数据库这一行”并不一定对。

### `user.used_quota` / `user.request_count`：不走 Redis 计数缓存，主要走 DB 或批量刷库

消费结算成功后，文本、音频、图片等主链路通常会调用：

- `model.UpdateUserUsedQuotaAndRequestCount(userId, quota)`

它的行为是：

1. 如果 `BATCH_UPDATE_ENABLED=true`，把 `used_quota += quota` 和 `request_count += 1` 先累加到当前进程内存。
2. 后台批量更新协程按 `BATCH_UPDATE_INTERVAL` 定时刷回数据库。
3. 如果没有开启 batch，则直接执行数据库更新。

它**不会**像 `user.quota` 那样维护一份 Redis 计数缓存。

所以：

```text
user.used_quota / user.request_count
  -> 不是先写 Redis
  -> 要么直接写 DB
  -> 要么先写本机内存聚合，再定时批量写 DB
```

默认批量刷新间隔来自：

- `BATCH_UPDATE_INTERVAL`

默认值是 `5` 秒。

### `token.remain_quota` / `token.used_quota`：和 `user.quota` 类似，也支持批量落库

调用 token 额度在预扣和退款时，会通过：

- `model.DecreaseTokenQuota(...)`
- `model.IncreaseTokenQuota(...)`

更新：

- `remain_quota`
- `used_quota`

如果 Redis 开启，会异步更新 token 缓存；如果 batch 开启，也可能先进入内存聚合，再批量写库。

所以 token 额度侧和 user 钱包额度侧比较像，都是：

- 有缓存层
- 有可选批量落库
- 最终真实结算仍然要回到数据库

### 查询侧也不是统一策略

读路径同样分化明显：

- `GetUserQuota(...)` 会优先读 Redis 用户缓存，失败再回数据库。
- `GetUserUsedQuota(...)` 直接查数据库。

因此，如果 batch 已开启，就要意识到：

- `user.quota` 的读取更容易看到缓存中的较新值
- `user.used_quota` / `user.request_count` 更可能在几秒内落后于刚发生的请求

### 一句话记忆

可以把这几类数据记成：

```text
user.quota / token.remain_quota
  = 热路径余额字段
  = Redis 缓存 + DB 最终落地

user.used_quota / user.request_count
  = 统计字段
  = DB 直写或进程内批量刷库
  = 默认不走 Redis 计数缓存
```

## 计费相关核心对象

- `types.PriceData`
  当前请求的价格计算结果。
- `relay/common/BillingSettler`
  统一的结算接口。
- `service/BillingSession`
  单次请求的预扣、结算、退款生命周期。
- `billingexpr`
  动态定价表达式引擎。

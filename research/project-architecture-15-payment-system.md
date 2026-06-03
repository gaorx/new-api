# 15 - 支付系统与支付方式专题

## 这篇文档关注什么

这篇文档专门说明 `new-api` 里的“支付”能力，而不是更广义的“计费”。

两者关系是：

- 支付：用户如何把真实世界的钱变成平台里的可用额度或订阅实例
- 计费：用户在调用模型时，平台如何把 usage 结算成 quota 扣减

如果你想看请求为什么这样扣费，主文档还是 [project-architecture-06-billing-system.md](./project-architecture-06-billing-system.md)。  
如果你想看项目到底支持哪些支付手段、每种怎么接、订单怎么完成，这篇更合适。

## 总体结构

从代码看，支付系统大体分成四层：

1. 前端支付入口
2. Controller 创建支付订单 / 结账会话
3. 第三方支付平台完成支付并回调
4. 本地订单落账，转成钱包额度或订阅实例

可以简化理解为：

```text
用户点击支付
  -> new-api 创建本地待支付订单
  -> 跳第三方收银台 / 结账页
  -> 第三方回调 new-api
  -> new-api 校验回调并完成订单
  -> 充值成功后增加 quota，或订阅成功后生成 user_subscription
```

这里很重要的一点是：  
`new-api` 不把“前端跳到支付页”当成成功，而是**必须等 webhook 或通知回调确认**，才会真正发放额度或开通订阅。

## 支付能力分成两类

从业务效果看，项目里的支付主要分成两类：

### 1. 钱包充值

充值成功后，最终结果是增加用户的钱包额度，底层一般落到：

- `top_ups`
- `users.quota`

这类入口主要在：

- `/api/user/pay`
- `/api/user/stripe/pay`
- `/api/user/creem/pay`
- `/api/user/waffo/pay`
- `/api/user/waffo-pancake/pay`

### 2. 订阅购买

支付成功后，最终结果不是直接给钱包加钱，而是：

- 创建或完成 `subscription_orders`
- 生成 `user_subscriptions`

这类入口主要在：

- `/api/subscription/balance/pay`
- `/api/subscription/epay/pay`
- `/api/subscription/stripe/pay`
- `/api/subscription/creem/pay`
- `/api/subscription/waffo-pancake/pay`

## 统一的本地订单模型

虽然外部网关很多，但项目内部只维护两类主订单：

### 充值订单

使用 `model.TopUp`，核心字段包括：

- `trade_no`：本地订单号
- `payment_method`：前端选择的支付方式
- `payment_provider`：实际支付提供商
- `amount`：购买的额度数量
- `money`：实际支付金额
- `status`：`pending/success/failed/...`

这说明不同网关虽然协议不同，但内部会先收敛到同一套充值订单事实表。

### 订阅订单

使用 `model.SubscriptionOrder`，核心字段包括：

- `trade_no`
- `payment_method`
- `payment_provider`
- `plan_id`
- `money`
- `provider_payload`
- `status`

它的作用是把“支付动作”和“订阅实例生效”解耦开来。  
支付平台只负责告诉 `new-api` “这笔钱付成功了”；至于如何变成平台内订阅实例，由本地业务决定。

## 支付前的几个统一前置条件

不是所有支付入口都一上来就能用，系统通常还有几层前置约束。

### 合规确认

`GetTopUpInfo` 会暴露：

- `payment_compliance_confirmed`
- `payment_compliance_terms_version`

很多支付与订阅购买接口在执行前也会调用 `requirePaymentCompliance(...)`。  
这意味着项目把“支付合规条款确认”作为运行期开关，而不是只写在文档里。

### 网关是否已配置

每种支付网关都不是无条件开启：

- EPay：需要 `PayAddress/EpayId/EpayKey`
- Stripe：需要 `StripeApiSecret/StripeWebhookSecret/...`
- Creem：需要 `CreemApiKey`，正式模式还需要 `CreemWebhookSecret`
- Waffo：需要 API Key、私钥、公钥证书等
- Waffo Pancake：需要 `MerchantID/PrivateKey`，充值还依赖 `StoreID/ProductID`

所以“代码支持”和“当前实例可用”是两回事。

### 最小充值额与金额换算

不同网关会各自检查最小充值额度，例如：

- `MinTopUp`
- `StripeMinTopUp`
- `WaffoMinTopUp`
- `WaffoPancakeMinTopUp`

而且用户前端看到的 `amount`，有时是金额，有时是 tokens 风格的额度数量。  
控制器会先结合：

- `QuotaDisplayType`
- `QuotaPerUnit`
- group topup ratio
- amount discount

把展示层输入换算成实际支付金额。

也就是说，支付系统不是简单地“传入 100 就收 100”，而是会经过平台自己的价格映射。

## 支持的支付方式总览

按当前代码，项目支持这些支付手段：

1. EPay / 易支付
2. Stripe
3. Creem
4. Waffo
5. Waffo Pancake
6. 余额支付

其中“余额支付”不是第三方网关，而是站内资金支付方式，只用于订阅购买。

---

## 一、EPay / 易支付

### 它是什么

在这个项目里，EPay 更像一个“聚合支付入口”。

`new-api` 自己并不直接和支付宝、微信原生 SDK 打交道，而是：

- 先把订单发给易支付网关
- 再由易支付去承接具体支付方式
- 最后易支付回调 `new-api`

因此，`EPay` 是支付提供商，`alipay/wxpay/custom1...` 更像它下面的具体支付方法。

### 支持哪些具体手段

默认配置里至少包含：

- `alipay`
- `wxpay`

默认文案分别对应：

- 支付宝
- 微信

此外，`PayMethods` 是可配置数组，所以理论上还可以接：

- 其他易支付支持的方法类型
- 项目里自定义命名的渠道标识

换句话说，这一层的可扩展性很强，但前提是底层易支付网关本身支持。

### 支持哪些业务

EPay 支持：

- 钱包充值
- 订阅购买

对应本地接口分别是：

- `/api/user/pay`
- `/api/subscription/epay/pay`

### 工作原理

EPay 链路大体是：

```text
用户选择支付方式（如 alipay / wxpay）
  -> new-api 创建本地订单
  -> 调用 go-epay 生成支付地址和参数
  -> 前端跳转 / 提交到易支付收银台
  -> 易支付异步通知 new-api
  -> new-api Verify 通知并完成本地订单
```

这条链路有几个特点：

### 1. 支付方式是“开放枚举”

`RequestEpay` 和 `SubscriptionRequestEpay` 并不写死只能传 `alipay` / `wxpay`。  
它们依赖 `operation_setting.ContainsPayMethod(...)` 判断当前方式是否在配置列表里。

这说明：

- 默认常见方式是支付宝和微信
- 实际可用方式由管理员配置决定

### 2. 采用同步回跳 + 异步通知双通道

EPay 同时有：

- `notify`
- `return`

其中真正可靠的完成依据是通知回调。  
浏览器回跳更多是为了改善用户体验，便于把用户带回控制台看到成功或失败页。

### 3. 订单完成前会先落本地 pending 记录

这样即使第三方回调晚到，也有本地 `trade_no` 能对上。  
如果只靠回调临时拼装数据，订单归属、幂等和补单都会更难做。

### 更适合什么场景

EPay 更适合：

- 面向中国大陆用户的支付宝 / 微信支付
- 希望通过聚合支付而不是自己直连多个原生支付渠道
- 需要保留灵活自定义支付方式枚举的场景

---

## 二、Stripe

### 它是什么

Stripe 在项目中属于“直连接账网关”。

系统直接使用 Stripe SDK 创建 Checkout Session，并依赖 Stripe Webhook 作为支付确认来源。

### 支持哪些具体手段

项目代码里对前端暴露的支付类型只有一个：

- `stripe`

但 Stripe Checkout 本身最终允许用户看到哪些付款方式，不由 `new-api` 逐项枚举，而主要取决于：

- Stripe 账户能力
- Checkout Session 配置
- Stripe 在该地区自动提供的付款方式

从当前代码还能看出，项目显式处理了异步成功/失败事件，因此至少考虑过这类“延迟确认型” Stripe 付款方式：

- bank transfer 一类
- SEPA 一类
- 其他不是立刻 `paid` 的付款方式

### 支持哪些业务

Stripe 支持：

- 钱包充值
- 订阅购买

对应接口：

- `/api/user/stripe/pay`
- `/api/subscription/stripe/pay`

### 工作原理

### 充值

```text
前端提交 amount
  -> new-api 计算实际应付金额
  -> 创建本地 top_up pending 订单
  -> 创建 Stripe Checkout Session（mode=payment）
  -> 前端打开 pay_link
  -> Stripe webhook 回调
  -> new-api 完成充值订单并发放 quota
```

### 订阅购买

```text
前端提交 plan_id
  -> new-api 校验套餐启用状态和 stripe_price_id
  -> 创建本地 subscription_order pending 订单
  -> 创建 Stripe Checkout Session（mode=subscription）
  -> Stripe webhook 回调
  -> new-api 完成订阅订单并创建 user_subscription
```

### 这条链路的关键特点

### 1. 充值与订阅使用不同的 Checkout 模式

- 充值：`payment`
- 订阅购买：`subscription`

这说明 Stripe 在支付侧确实区分了一次性付款和订阅型结账，但平台内“订阅成功后如何发放访问权”仍由 `new-api` 自己掌握。

### 2. Webhook 才是最终真相来源

Stripe 回调里明确处理了：

- `checkout.session.completed`
- `checkout.session.expired`
- `checkout.session.async_payment_succeeded`
- `checkout.session.async_payment_failed`

这说明系统没有偷懒地只看前端跳转结果，而是把异步支付状态也纳入了正式订单状态机。

### 3. 允许延迟确认型支付方式

`completed` 事件里如果 `payment_status != paid`，系统不会立刻发额度，而是等待后续异步成功事件。  
这对银行转账、SEPA 一类方式尤其重要。

### 4. 充值金额和套餐价格来源不同

- 充值：本地按额度、倍率、折扣计算
- 订阅：直接使用套餐绑定的 `stripe_price_id`

也就是说：

- 充值是平台自己定价后去 Stripe 收钱
- 订阅是平台先在 Stripe 里预建价格，再按价格 ID 拉起购买

### 更适合什么场景

Stripe 更适合：

- 海外信用卡 / 全球支付场景
- 需要 Stripe Checkout 和 Stripe Webhook 的成熟闭环
- 一次性充值与套餐型购买并存的场景

---

## 三、Creem

### 它是什么

Creem 在项目里也是独立支付提供商，但它的接法更偏“产品化结账”。

也就是说，它不是单纯给一个金额去收款，而是通常要求先绑定某个产品 ID，再发起 checkout。

### 支持哪些具体手段

前端暴露的是：

- `creem`

但真正售卖的对象是 `product_id` 对应的商品。

对于充值场景，后台维护一组 `CreemProducts`，每个产品大致包含：

- `productId`
- `name`
- `price`
- `currency`
- `quota`

所以 Creem 的详细支付方式并不是“支付宝 / 微信 / Apple Pay”这种前台枚举，而是：

- 先选商品
- 再进入 Creem checkout
- 由 Creem 处理实际收银细节

### 支持哪些业务

Creem 支持：

- 钱包充值
- 订阅购买

对应接口：

- `/api/user/creem/pay`
- `/api/subscription/creem/pay`

### 工作原理

### 充值

```text
前端先选一个 Creem product
  -> new-api 解析配置里的产品列表
  -> 按产品价格和额度创建本地 top_up pending 订单
  -> 调用 Creem API 生成 checkout_url
  -> 前端跳转 Creem checkout
  -> Creem webhook 回调
  -> new-api 完成订单并给用户加 quota
```

### 订阅购买

```text
前端选择 plan_id
  -> new-api 读取套餐并检查 creem_product_id
  -> 创建本地 subscription_order pending 订单
  -> 构造轻量商品快照并生成 checkout_url
  -> Creem webhook 回调
  -> new-api 完成订阅订单
```

### 这条链路的关键特点

### 1. 充值不是“按输入金额随意充”

Creem 充值走的是商品列表模式。  
用户购买的是某个后台预先定义好的产品，而不是任意金额。

### 2. 产品本身就携带价格和额度

这意味着对于 Creem 充值：

- 价格不是现场算出来再让第三方收
- 而是后台先把“多少额度卖多少钱”编码进产品配置

### 3. Webhook 还承担了订单类型分流

Creem 回调里会识别订单类型，再决定走：

- 充值完成
- 订阅完成

说明同一个 webhook 入口实际上复用了多种业务。

### 更适合什么场景

Creem 更适合：

- 用固定商品包卖额度
- 用商品 ID 绑定订阅套餐
- 希望支付侧先有产品实体，再由平台做本地订单映射

---

## 四、Waffo

### 它是什么

Waffo 是另一个全球支付网关接入，但和 Stripe / Creem 不同，它在项目里显式暴露了若干支付方法配置。

### 支持哪些具体手段

默认 `WaffoPayMethods` 里包含：

- `Card`
- `Apple Pay`
- `Google Pay`

更细一点说，默认映射大致是：

- `Card` -> `CREDITCARD,DEBITCARD`
- `Apple Pay` -> `APPLEPAY`
- `Google Pay` -> `GOOGLEPAY`

这里有个实现层上的关键点：

- 前端优先只传 `pay_method_index`
- 服务端再按索引去自己的白名单列表中解析真实 `PayMethodType/PayMethodName`

所以客户端并不能随便构造一个任意支付方式发给 Waffo，而是由服务端控制允许的方式集合。

### 支持哪些业务

Waffo 当前支持：

- 钱包充值

从现有路由和控制器看，没有单独的 Waffo 订阅购买入口。

### 工作原理

```text
前端输入充值额度并选择 Waffo 支付方式
  -> new-api 校验 pay_method_index 是否在服务端白名单中
  -> 创建本地 top_up pending 订单
  -> 调用 Waffo SDK 创建订单
  -> 返回 redirect URL / orderAction
  -> 前端跳转 Waffo
  -> Waffo webhook 回调
  -> new-api 验签、解析订单状态
  -> 充值成功后发放 quota
```

### 这条链路的关键特点

### 1. 支付方式由服务端白名单控制

这比“前端直接传 applepay/googlepay/card 文本”更安全。  
它避免了客户端伪造一个服务端没打算开放的支付方式。

### 2. 支持币种格式差异

代码里对 `JPY/KRW/VND/IDR` 这类零小数位币种做了专门格式处理。  
说明 Waffo 这一层不只是“统一收 USD”，而是已经考虑过多币种金额格式差异。

### 3. 订单号设计偏向可追踪性

Waffo 里把：

- `paymentRequestId`
- `merchantOrderId`

尽量保持一致，方便排查链路问题。

### 4. 充值金额仍由平台定价逻辑决定

Waffo 和 Stripe 的充值比较像，都是：

- 前端给出额度数量
- 平台先算应付金额
- 再把金额发给第三方收款

### 更适合什么场景

Waffo 更适合：

- 海外用户卡支付
- Apple Pay / Google Pay 类快捷支付
- 希望由服务端精确控制开放哪些支付方式

---

## 五、Waffo Pancake

### 它是什么

Waffo Pancake 是项目里实现最“平台化”的一个支付接入。

它不只是一个简单的收银台接口，而是带有：

- Merchant / Store / Product 配置
- 后台建店与建商品流程
- Authenticated Checkout
- Buyer Identity
- Webhook 环境隔离

这条链路的设计明显比其他网关更重。

### 支持哪些具体手段

前端暴露的支付类型是：

- `waffo_pancake`

但它背后的支付详细手段不是由 `new-api` 自己枚举，而是由 Waffo Pancake 的托管结账页处理。  
`new-api` 负责的是：

- 选择哪个 Product
- 传什么价格快照
- 绑定哪个 BuyerIdentity
- 用哪个 webhook URL 收结果

所以对业务方来说，它更像：

- 一个托管式 checkout 平台
- 而不是项目自己维护的支付方式列表

### 支持哪些业务

Waffo Pancake 支持：

- 钱包充值
- 订阅购买

对应接口：

- `/api/user/waffo-pancake/pay`
- `/api/subscription/waffo-pancake/pay`

### 工作原理

### 充值

```text
前端输入充值额度
  -> new-api 计算应付金额
  -> 创建本地 top_up pending 订单
  -> 创建 Pancake Authenticated Checkout Session
  -> 携带 ProductID + PriceSnapshot + BuyerIdentity + trade_no
  -> 前端跳转 checkout_url
  -> Pancake webhook 回调
  -> new-api 验签、校验环境、解析 trade_no
  -> 完成充值订单
```

### 订阅购买

```text
前端提交 plan_id
  -> new-api 检查 plan.WaffoPancakeProductId
  -> 创建本地 subscription_order pending 订单
  -> 创建 Authenticated Checkout Session
  -> 把本地 trade_no 放进 OrderMerchantExternalID
  -> webhook 回调后完成订阅订单
```

### 这条链路的关键特点

### 1. 使用 Authenticated Checkout，而不是匿名 checkout

系统会把用户 ID 规范化成稳定的 `BuyerIdentity`。  
这样即使用户在收银页修改邮箱，本地仍然能确认“这笔订单属于哪个平台用户”。

这是它和很多“只靠邮箱回填订单”的接法最大的不同。

### 2. 使用 `OrderMerchantExternalID` 回写本地订单号

Pancake 自己有内部 `ORD_*` 订单号，但 `new-api` 真正用来查本地订单的是：

- `OrderMerchantExternalID = 本地 trade_no`

这样 webhook 回来后可以直接映射本地订单，不需要再做二次关联表。

### 3. webhook 路径按 `test/prod` 分环境

路由是：

- `/api/waffo-pancake/webhook/test`
- `/api/waffo-pancake/webhook/prod`

并且处理器还会校验 webhook payload 里的 `event.mode` 是否和 URL 环境一致。  
这是一种很实用的“防误配”设计：就算商户后台把测试回调填到了生产 URL，也能在服务端被识别出来。

### 4. 充值和订阅共用同一 webhook，但靠订单号前缀分流

例如：

- 充值前缀：`WAFFO_PANCAKE-`
- 订阅前缀：`WAFFO_PANCAKE_SUB-`

这意味着：

- 外部支付平台看到的是同类 checkout
- 平台内部再根据订单号语义决定调用 `RechargeWaffoPancake(...)` 还是 `CompleteSubscriptionOrder(...)`

### 5. 价格覆盖通过 `PriceSnapshot` 完成

Waffo Pancake 充值并不是每个金额都先建一个产品。  
系统只要有基础 `ProductID`，再在每次结账会话里传 `PriceSnapshot` 覆盖价格即可。

这样带来的好处是：

- 不需要为每个充值金额预创建商品
- 价格仍然可以保留平台侧动态计算逻辑

### 6. 订阅侧故意使用 `OnetimeProduct`，而不是自动续费型产品

这是一个非常重要的业务决策。

当前代码明确表达的意思是：

- 平台还没有统一的 renewal event handling
- 如果支付网关自动续费，但平台没有同步延长订阅访问权，会出现体验和账务不一致

所以目前更准确的理解是：

- Waffo Pancake 可以卖“订阅套餐”
- 但支付侧实现仍然按一次性购买处理
- 成功后由平台创建一条有时长的订阅实例

而不是典型的“支付平台自动续费订阅”

### 更适合什么场景

Waffo Pancake 更适合：

- 想要托管式 checkout
- 想把买家身份和本地用户强绑定
- 想减少前端自行拼支付参数的复杂度
- 想要更强的环境隔离和 webhook 安全控制

---

## 六、余额支付

### 它是什么

余额支付不是第三方支付，而是站内支付方式。

用户之前通过充值获得了 quota，之后可以直接拿这部分站内余额购买订阅套餐。

### 支持哪些业务

余额支付当前用于：

- 订阅购买

对应接口：

- `/api/subscription/balance/pay`

### 工作原理

```text
用户选择一个订阅套餐
  -> new-api 检查套餐是否允许余额支付
  -> 检查用户当前 quota 是否充足
  -> 直接在本地完成扣减
  -> 创建 user_subscription
```

这里没有第三方回调，也没有外部收银台。  
它本质上是平台内部账户余额结算。

### 这条链路的关键特点

### 1. 不依赖外部支付平台

所以：

- 不存在 webhook 失败问题
- 不存在外部签名校验
- 但必须非常注意本地事务一致性

### 2. 套餐级别可单独控制

`subscription_plans.allow_balance_pay` 表示：

- 某个套餐是否允许余额购买

这意味着平台可以做更细的商品策略，例如：

- 某些套餐只能现金购买
- 某些套餐允许钱包余额直接买

---

## 支付方式和支付提供商的区别

项目里有一个很容易混淆但很重要的设计：

- `payment_method`
- `payment_provider`

它们不是一回事。

### `payment_provider`

更接近“订单实际走的是哪家网关”，例如：

- `epay`
- `stripe`
- `creem`
- `waffo`
- `waffo_pancake`
- `balance`

### `payment_method`

更接近“用户感知到的付款方式”或“业务选择的方式”，例如：

- `alipay`
- `wxpay`
- `stripe`
- `creem`
- `waffo`
- `waffo_pancake`
- `balance`

最典型的例子就是 EPay：

- `payment_provider = epay`
- `payment_method = alipay` 或 `wxpay`

这样建模的好处是：

- 统计时可以区分“网关表现”与“用户实际使用的支付手段”
- 补单时也能防止把不同网关的订单混着处理

## 幂等、安全和回调处理上的共同模式

虽然各网关实现不同，但整体上有几个共通原则。

### 1. 先建本地 pending 订单，再跳第三方

这是整个支付系统最基础的结构。  
它解决的是：

- 回调到来时如何找到订单
- 支付页关闭后如何查订单状态
- 管理员如何补单
- 如何做幂等和并发锁

### 2. 回调处理前先验签

各网关都使用自己的验签方式：

- EPay：SDK Verify
- Stripe：Webhook Secret
- Creem：HMAC-SHA256
- Waffo：SDK verify
- Waffo Pancake：SDK verify

也就是说，系统不会因为“收到了一个 POST”就直接发额度。

### 3. 完成订单前会做本地加锁或状态检查

常见模式是：

- `LockOrder(tradeNo)`
- 查询订单当前状态
- 只允许 `pending -> success/failed`

这能防止：

- 重复回调
- 人工补单和 webhook 并发
- 同一订单被不同支付网关误处理

### 4. 通过 `payment_provider` 防止错单

例如：

- Stripe 回调只应该完成 `PaymentProviderStripe` 的订单
- Waffo Pancake 回调只应该完成 `PaymentProviderWaffoPancake` 的订单

代码里对这类错配有显式校验。  
这能防止“同一个 trade_no 被错误网关完成”的事故。

## 前端展示层是动态的，不是写死的

`GetTopUpInfo` 会把支付能力动态返回给前端，例如：

- `enable_online_topup`
- `enable_stripe_topup`
- `enable_creem_topup`
- `enable_waffo_topup`
- `enable_waffo_pancake_topup`
- `pay_methods`
- `waffo_pay_methods`
- `creem_products`

因此控制台里的支付按钮不是全写死在前端，而是：

- 由后端根据当前配置和合规状态动态下发
- 前端再根据返回值决定显示哪些支付入口

这使得同一套前端能适应不同站点部署。

## 当前不应误解的几点

### 1. “支持 Stripe”不等于前端枚举了所有 Stripe 付款方式

`new-api` 只知道自己走 Stripe Checkout。  
至于 Checkout 最终给用户显示卡、银行转账还是别的方式，主要由 Stripe 决定。

### 2. “支持订阅支付”不等于所有网关都做了自动续费闭环

特别是 Waffo Pancake，当前实现明确偏向“一次性购买后生成订阅实例”，而不是全自动续费订阅。

### 3. “支持支付宝 / 微信”主要体现在 EPay 聚合层

当前代码里没有单独的支付宝官方 SDK 接入，也没有单独的微信支付官方 SDK 接入。  
这两种方式主要是通过 EPay 聚合网关暴露出来的。

### 4. 代码支持不等于部署默认开启

很多支付入口只有在：

- 凭证完整
- webhook 已配置
- 合规已确认

之后才会在运行时真正开放。

## 一句话总结

这个项目的支付系统可以概括成：

```text
多网关接入
  + 本地统一订单模型
  + webhook 驱动的最终确认
  + 充值与订阅两条业务落账链路
  + 动态配置、合规开关与支付方式白名单控制
```

如果你只想快速记住每种方式：

- EPay：聚合支付，默认承接支付宝/微信，也可扩展自定义方式
- Stripe：Checkout + Webhook，支持充值和订阅
- Creem：基于商品 ID 的 checkout，支持充值和订阅
- Waffo：显式暴露 Card / Apple Pay / Google Pay，当前主要用于充值
- Waffo Pancake：托管式 Authenticated Checkout，支持充值和订阅，强调 BuyerIdentity 和环境隔离
- 余额支付：站内 quota 直接购买订阅，不走第三方

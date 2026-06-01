# 09 - 横切能力与总体结论

## 关键横切能力

除了主干链路，这个项目还有一些贯穿全局的横切能力。

### i18n

- 后端：`i18n/`
- 前端：`web/default/src/i18n/`

说明项目从一开始就不是单语言系统。

### 缓存

缓存是多层的：

- Redis
- 内存缓存
- 渠道缓存
- Channel Affinity Cache
- 前端本地缓存

### 监控与性能

项目内置了：

- 系统监控
- 性能指标 `pkg/perf_metrics`
- pprof
- 可选 Pyroscope

这说明它被设计成一个长期运行的服务端产品，而不是一次性工具。

### OAuth / Passkey / 2FA

项目在“平台账户安全”上投入比较多：

- 多 OAuth 提供商
- 自定义 OAuth Provider
- Passkey
- 2FA

这对一个 API 网关项目来说是比较重的平台化设计。

## 架构风格总结

### 这个项目更像什么

从工程形态上说，`new-api` 更像：

- 一个 AI API Gateway
- 加上一套 BSS/计费系统
- 加上一套 IAM/权限系统
- 加上一套控制台与运营后台

它已经明显超出了“简单代理”的范围。

### 它最核心的几个设计亮点

1. `RelayInfo` 统一承载代理链路上下文。
2. `Channel + Ability` 把上游接入与可路由能力拆开建模。
3. `Distribute + Retry + Affinity` 形成了较完整的流量调度体系。
4. `BillingSession + billingexpr` 形成了较完整的可编程计费体系。
5. `OptionMap + ConfigManager + SyncOptions` 形成了运行时配置中心。

### 它的复杂度主要来自哪里

复杂度不在单个 provider 的接入，而在这几个维度叠加：

- 多协议兼容
- 多租户/多用户/多分组
- 多渠道路由
- 多种计费模式
- 同步请求 + 异步任务
- 多数据库兼容
- 前后端一体的平台能力

## 建议继续深挖的代码

如果下一步要继续深入，我建议按这个顺序看：

1. `main.go`
2. `router/relay-router.go`
3. `middleware/distributor.go`
4. `controller/relay.go`
5. `relay/common/relay_info.go`
6. `relay/channel/adapter.go`
7. `model/channel.go`
8. `model/ability.go`
9. `service/billing_session.go`
10. `relay/helper/price.go`
11. `pkg/billingexpr/expr.md`
12. `web/default/src/routes/` 和 `web/default/src/features/`

## 最终结论

这个项目的核心不是“把 OpenAI 接口转发出去”，而是：

“围绕 AI 模型调用，建立一套统一接入、统一鉴权、统一分发、统一计费、统一运营的网关平台。”

其中最重要的三条主线是：

1. `Token / Group / Channel / Ability`
   决定请求路由到哪里。
2. `RelayInfo / Adaptor / RelayFormat`
   决定请求如何被执行。
3. `PriceData / BillingSession / Subscription / billingexpr`
   决定请求如何收费和结算。

如果把这三条主线掌握住，这个仓库的大部分复杂度就能被解释清楚。

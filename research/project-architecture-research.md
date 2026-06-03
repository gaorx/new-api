# new-api 项目整体架构调研

## 说明

原来的单篇长文档已经按主题拆分为多个小文件，便于按需阅读、继续扩展和单独维护。

这一页现在只承担两个职责：

1. 作为总目录，告诉你应该从哪篇开始读。
2. 作为“信息归属表”，约束重复内容应该收敛到哪篇主文档。

全局概括、顶层架构和核心主线已经统一收敛到：

- [01 - 总览](/Users/gaorx/Works/my/new-api/research/project-architecture-01-overview.md)

建议阅读顺序如下：

1. [01 - 总览](/Users/gaorx/Works/my/new-api/research/project-architecture-01-overview.md)
2. [02 - 启动流程与后端分层](/Users/gaorx/Works/my/new-api/research/project-architecture-02-startup-and-backend-layers.md)
3. [03 - Relay 运行时架构](/Users/gaorx/Works/my/new-api/research/project-architecture-03-relay-runtime.md)
4. [04 - 核心概念与关系](/Users/gaorx/Works/my/new-api/research/project-architecture-04-core-concepts-and-relationships.md)
5. [05 - 渠道分发与选路机制](/Users/gaorx/Works/my/new-api/research/project-architecture-05-channel-routing.md)
6. [06 - 计费体系](/Users/gaorx/Works/my/new-api/research/project-architecture-06-billing-system.md)
7. [07 - 配置系统与数据存储兼容性](/Users/gaorx/Works/my/new-api/research/project-architecture-07-configuration-and-storage.md)
8. [08 - 前端控制台架构](/Users/gaorx/Works/my/new-api/research/project-architecture-08-frontend-console.md)
9. [09 - 横切能力与总体结论](/Users/gaorx/Works/my/new-api/research/project-architecture-09-cross-cutting-capabilities-and-summary.md)
10. [10 - 认证、安全与权限模型](/Users/gaorx/Works/my/new-api/research/project-architecture-10-auth-and-security.md)
11. [11 - 异步任务与后台作业](/Users/gaorx/Works/my/new-api/research/project-architecture-11-async-tasks-and-background-jobs.md)
12. [12 - 中间件职责与请求链路](/Users/gaorx/Works/my/new-api/research/project-architecture-12-middleware-and-request-flow.md)
13. [13 - 模型元数据、价格来源与请求时序](/Users/gaorx/Works/my/new-api/research/project-architecture-13-model-metadata-pricing-and-request-sequence.md)
14. [14 - 模型列表与价格接口](/Users/gaorx/Works/my/new-api/research/project-architecture-14-model-list-and-pricing-interfaces.md)
15. [15 - 支付系统与支付方式专题](/Users/gaorx/Works/my/new-api/research/project-architecture-15-payment-system.md)

## 文档分工

下面这张表用于说明每篇文档的主要边界，后续增补内容时尽量放进对应“主文档”，其他文档只保留结论和跳转。

| 文档 | 主要职责 | 典型问题 |
| --- | --- | --- |
| `01-overview` | 全局入口与阅读导航 | 这个项目整体像什么 |
| `02-startup-and-backend-layers` | 启动流程、后端层次、main 装配 | 系统怎么启动，后端怎么分层 |
| `03-relay-runtime` | Relay 主运行时、`RelayInfo`、`Adaptor`、重试、执行链 | 请求进入 Relay 后怎么跑 |
| `04-core-concepts-and-relationships` | User / Token / Group / Channel / Ability / Pricing 概念关系 | 各核心名词分别是什么 |
| `05-channel-routing` | `group + model -> ability -> channel` 选路与熔断恢复 | 为什么这次命中某条 channel |
| `06-billing-system` | 预扣、结算、quota、订阅、表达式计费 | 为什么这样扣费 |
| `07-configuration-and-storage` | 配置来源、缓存、Redis/DB/内存分工、多节点一致性 | 数据到底存在哪、怎么同步 |
| `08-frontend-console` | 前端控制台功能与后端映射 | 控制台页面和后端能力怎么对应 |
| `09-cross-cutting-capabilities-and-summary` | 横切能力与总括结论 | 还有哪些全局能力值得关注 |
| `10-auth-and-security` | 身份、权限、敏感词、OAuth、Passkey、2FA | 谁能调，如何鉴权，哪里做安全控制 |
| `11-async-tasks-and-background-jobs` | 异步任务、轮询、后台定时作业 | 视频/Suno/MJ 这类任务怎么跑 |
| `12-middleware-and-request-flow` | 路由与中间件上下文注入 | 请求在进 controller 前发生了什么 |
| `13-model-metadata-pricing-and-request-sequence` | 模型元数据、价格来源、一次请求的元数据决策链 | `model/vendor/channel/pricing` 怎么联系 |
| `14-model-list-and-pricing-interfaces` | `/v1/models`、`/api/pricing` 及相关接口 | 模型列表和价格接口怎么组织 |
| `15-payment-system` | 支付网关、支付方式、充值/订阅订单完成链路 | 这个项目到底支持哪些支付，钱怎么变成 quota 或订阅 |
| `relay.md` | Relay 协议互转专题 | 是不是统一转 OpenAI，哪些地方直通 |
| `chat-completions-relay-flow.md` | `/v1/chat/completions` 专项链路 | 这个接口的特殊分支怎么走 |
| `log.md` | Relay/消费/错误日志专题 | 调用日志记不记、记在哪、不同路径长什么样 |
| `options.md` | `options` 表配置键专题 | 某个 option key 是干什么的 |
| `database-schema-summary.md` | 数据库表结构与关系总表 | 某张表存什么、字段怎么分工 |

## 去重规则

后续继续维护 `research/` 时，建议按下面规则写：

- 全局定义只在 `01-overview` 保留一份。
- Relay 通用运行时只在 `03-relay-runtime` 保留一份。
- `group + model -> ability -> channel` 只在 `05-channel-routing` 详细展开。
- quota、预扣、结算、订阅、表达式计费只在 `06-billing-system` 详细展开。
- 模型元数据、价格来源、刷新机制只在 `13` 详细展开。
- `/v1/models` 与 `/api/pricing` 的接口边界只在 `14` 详细展开。
- 某个专题如果必须引用通用背景，只保留 1 到 3 句结论，并链接到主文档。

## 建议

如果你是第一次读这个项目：

1. 先看“总览”和“核心概念与关系”
2. 再看“Relay 运行时架构”和“渠道分发与选路机制”
3. 最后看“计费体系”和“配置系统”

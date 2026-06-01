# new-api 项目整体架构调研

## 说明

原来的单篇长文档已经按主题拆分为多个小文件，便于按需阅读、继续扩展和单独维护。

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

## 一句话概括

`new-api` 本质上是一个“面向多上游 AI 提供商的统一网关 + 配额计费平台 + 管理控制台”。

它的核心不是单纯的 API 转发，而是把以下能力组合在一起：

- 统一入口协议兼容层
- 多渠道分发与故障重试
- 用户/令牌/分组权限体系
- 配额预扣、结算、退款、订阅计费
- 配置中心与动态定价
- 管理后台与运营面板

## 建议

如果你是第一次读这个项目：

1. 先看“总览”和“核心概念与关系”
2. 再看“Relay 运行时架构”和“渠道分发与选路机制”
3. 最后看“计费体系”和“配置系统”

如果你后续要继续深挖源码，我也可以继续把这组文档补成：

- 请求时序图版
- 计费系统深度版
- 渠道接入开发指南版
- 新人 onboarding 版
- API / 权限矩阵版

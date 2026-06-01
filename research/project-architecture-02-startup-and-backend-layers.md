# 02 - 启动流程与后端分层

## 启动与装配流程

主入口在 `main.go`。

### 启动阶段做了什么

`main()` 的启动主线大致是：

1. `InitResources()`
2. 初始化环境变量与日志
3. 初始化比例/价格配置、HTTP 客户端、Token 编码器
4. 初始化主数据库和日志数据库
5. 初始化 Redis、性能指标、系统监控、i18n、自定义 OAuth
6. 初始化内存缓存、渠道缓存、配置同步任务、数据看板任务
7. 启动渠道自动测试、模型更新、订阅重置、Codex 凭证刷新等后台任务
8. 创建 Gin Server，挂载中间件和路由
9. 嵌入并提供前端静态资源

相关入口：

- `main.go`
- `common/init.go`
- `router/main.go`

### 运行模式特点

系统有明显的“主节点/从节点”意识：

- `common.IsMasterNode` 由 `NODE_TYPE` 控制
- 主数据库迁移只在主节点执行
- 多个后台任务只在主节点或特定条件下执行

这说明项目天然支持多节点部署，但控制面和后台调度更偏向主节点统一执行。

## 后端分层职责

项目显式采用分层架构：`Router -> Controller -> Service -> Model`。

### Router

路由层负责暴露不同产品面的入口：

- `router/api-router.go`
  用户、管理员、系统设置、支付、订阅、性能、OAuth 等管理 API。
- `router/relay-router.go`
  OpenAI / Claude / Gemini / Midjourney / Suno 等代理入口。
- `router/dashboard.go`
  兼容旧 dashboard 接口。
- `router/web-router.go`
  前端静态资源和 SPA 路由入口。

这里的关键点是：项目并不是一个单一 API，而是至少同时暴露了三类接口：

1. 平台管理 API
2. AI Relay API
3. Web 控制台入口

### Middleware

中间件层承担了很多真正“决定系统行为”的逻辑：

- 身份认证：`UserAuth` / `AdminAuth` / `RootAuth` / `TokenAuth`
- 全局限流、关键接口限流、模型请求限流
- `Distribute()` 渠道选择与上下文注入
- 请求体解压、请求体可重复读取、统计、日志、CORS、i18n

其中 `middleware/distributor.go` 是核心中的核心，因为它把“一个客户端请求”转成了“某个用户在某个分组下访问某个模型，并选中了某个渠道”的上下文。

### Controller

控制器层主要做两类工作：

1. 面向管理 API 的资源控制器
   比如用户、渠道、模型、订阅、支付、日志、选项。
2. 面向 Relay 的统一入口控制器
   比如 `controller/relay.go`。

`controller/relay.go` 不做具体协议实现，而是：

- 解析并校验请求
- 生成 `RelayInfo`
- 做敏感词检查、token 估算、价格计算、预扣费
- 驱动重试与渠道切换
- 调用 `relay/` 中的具体处理器

### Service

服务层更像“业务规则编排层”，典型职责有：

- 渠道选择：`service/channel_select.go`
- 分组可用性和 auto-group：`service/group.go`
- 渠道亲和性缓存：`service/channel_affinity.go`
- 统一计费会话：`service/billing_session.go`
- 敏感词、下载、图像、音频、任务轮询、订阅重置

这里的一个架构特点是：

`service/` 不直接等于“领域服务”，它更像整个系统行为的 orchestrator。

### Model

`model/` 不是单纯的数据结构定义，而是包含：

- GORM 模型
- 数据库初始化与迁移
- SQLite / MySQL / PostgreSQL 兼容逻辑
- 缓存读写
- 渠道能力表维护
- 配额、日志、订阅、任务等领域数据操作

最典型的例子是：

- `Channel` 表示渠道本体
- `Ability` 是“渠道-分组-模型”的展开映射表

这说明系统并不是运行时每次动态解析渠道支持能力，而是把它预展开到数据库/缓存中，以便高效选路。

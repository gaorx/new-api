# 02 - 启动流程与后端分层

## 启动与装配流程

主入口在 `main.go`。

### 后端入口形态

后端只有一个真正的程序入口：`main.go` 里的 `main()`。

它不是 `cobra` / `urfave/cli` 那种多子命令程序，没有 `serve`、`worker`、`migrate` 这类拆开的独立命令；当前是一个单一二进制，通过少量启动参数和大量环境变量控制行为。

目前能从代码里确认的启动参数主要有：

- `--port`：指定监听端口
- `--log-dir`：指定日志目录
- `--version`：打印版本并退出
- `--help`：打印帮助并退出

也就是说，这个项目的后端运行模型更接近：

```text
一个统一后端进程
  + 启动时初始化资源
  + 挂载管理 API / Relay API / Web 路由
  + 按节点角色决定是否执行迁移和后台任务
```

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

### 核心连接资源的持有方式

目前后端对数据库、Redis 以及一部分运行时状态的管理方式，整体上更接近：

```text
包级全局变量
  + 启动时初始化一次
  + 业务代码中直接引用
```

而不是典型的依赖注入风格。

已经确认的几个核心资源如下：

- 主业务库：`model.DB`
- 日志库：`model.LOG_DB`
- Redis 客户端：`common.RDB`
- Redis 开关：`common.RedisEnabled`
- 数据库类型标志：`common.UsingSQLite`、`common.UsingMySQL`、`common.UsingPostgreSQL`

这些对象或状态都定义为包级变量，随后在启动流程中完成赋值。

### DB / Redis 的初始化顺序

从 `InitResources()` 可以确认，资源初始化顺序大致是：

1. `common.InitEnv()`
2. `model.InitDB()`
3. `model.InitOptionMap()`
4. `model.InitLogDB()`
5. `common.InitRedisClient()`

也就是说：

- `model.InitDB()` 负责建立主库连接，并把连接放入全局变量 `model.DB`
- `model.InitLogDB()` 负责建立日志库连接；若未配置 `LOG_SQL_DSN`，则直接令 `model.LOG_DB = model.DB`
- `common.InitRedisClient()` 负责建立 Redis 连接，并把客户端放入全局变量 `common.RDB`

所以运行时语义更像：

```text
进程启动
  -> 初始化全局 DB / LOG_DB / RDB
  -> 后续业务层直接使用这些全局连接
```

### 业务代码如何使用这些连接

`model/` 层大量代码直接引用 `DB`：

- `DB.Where(...)`
- `DB.Create(...)`
- `DB.Transaction(...)`

这说明多数数据访问函数并不通过参数传入 `*gorm.DB`，而是直接依赖全局主库句柄。

Redis 侧也是类似风格：

- 业务代码直接访问 `common.RDB`
- 或先判断 `common.RedisEnabled`
- `common/redis.go` 中再封装一层 `RedisSet`、`RedisGet`、`RedisDel` 等辅助函数

因此更准确地说，这个项目对“连接资源”的抽象不是“连接对象逐层传递”，而是“全局单例 + 少量辅助封装”。

### 补充：不是完全没有局部注入

虽然整体风格是全局资源持有，但少量新代码会把全局连接再传入局部组件中使用。

例如某些缓存封装会把 `common.RDB` 传给内部结构体或 helper；不过它的来源仍然是全局 Redis 客户端，而不是从 request scope 或应用容器中分发出来。

因此从架构判断上，应把当前项目理解为：

- 主体是包级全局资源模式
- 局部存在少量“基于全局对象再包装”的写法
- 不是严格的依赖注入式后端

### Migration 是怎么触发的

这个项目当前没有独立的 `migrate` 子命令，也不是靠定时任务周期性跑 schema migration。

数据库 schema migration 的触发方式是：

1. 进程启动
2. `main()` 调 `InitResources()`
3. `InitResources()` 内先执行 `model.InitDB()`
4. 如配置了独立日志库，再执行 `model.InitLogDB()`
5. 只有 `common.IsMasterNode == true` 时才真正进入 `migrateDB()` / `migrateLOGDB()`

也就是说，它的 schema migration 更准确地说是：

```text
master 节点每次启动/重启时自动执行一次
slave 节点只连库，不执行 migration
```

几个关键特征：

- 不是定时任务
  代码里没有发现按分钟/按小时自动跑 DB migration 的 ticker / cron。
- 不是单独命令
  也没有 `newapi migrate`、`newapi upgrade` 之类 CLI 入口。
- 是启动前置步骤
  migration 发生在 HTTP server 启动之前；如果初始化 DB 失败，进程会直接退出。
- 主库和日志库分开处理
  `InitDB()` 负责主业务库；`InitLogDB()` 在 `LOG_SQL_DSN` 存在时负责日志库的 `logs` 表迁移。

### Migration 的实际执行内容

`model.InitDB()` 里的迁移逻辑以 `model/main.go` 为中心，采用“GORM `AutoMigrate` + 少量手写兼容迁移”的方式。

主链路如下：

- 先执行定制列迁移
  - `migrateSubscriptionPlanPriceAmount()`：把 `subscription_plans.price_amount` 从浮点型迁到 `decimal(10,6)`（MySQL / PostgreSQL）
  - `migrateTokenModelLimitsToText()`：把 `tokens.model_limits` 迁到 `text`
- 再执行 `DB.AutoMigrate(...)`
  - 迁移业务主表，如 `users`、`tokens`、`channels`、`abilities`、`tasks`、`subscription_*` 等
- 对 SQLite 单独补兼容逻辑
  - `ensureSubscriptionPlanTableSQLite()` 用手写 DDL/补列方式保证 `subscription_plans` 结构完整
- 日志库走单独迁移
  - `migrateLOGDB()` 只做 `LOG_DB.AutoMigrate(&Log{})`

这说明它并不是版本号驱动、逐文件编号的 migration 框架，而是：

```text
启动时自动检查当前表结构
  + 用 AutoMigrate 补齐大多数结构
  + 对少数跨库差异大的列做显式兼容迁移
```

### 多节点下的迁移语义

`common.IsMasterNode` 由 `NODE_TYPE` 控制：

- `NODE_TYPE != slave`：视为 `master`，会执行 schema migration
- `NODE_TYPE = slave`：不会执行 schema migration

因此多节点部署下，当前预期模型是：

- 一个集群通常只保留一个 `master`
- 所有 `slave` 共享同一个库，但依赖 `master` 先把 schema 升到位

代码里没有看到 leader election 或分布式锁来协调“多个 master 同时迁移”的场景，所以不应把 migration 设计理解为天然多主安全。

### 运行模式特点

系统有明显的“主节点/从节点”意识：

- `common.IsMasterNode` 由 `NODE_TYPE` 控制
- 主数据库迁移只在主节点执行
- 多个后台任务只在主节点或特定条件下执行

这说明项目天然支持多节点部署，但控制面和后台调度更偏向主节点统一执行。

### 主从节点不等于进程拆分

这里有一个很重要、也很容易误解的点：

`master/slave` 只是同一个后端程序的“运行角色”差异，不是两个不同的可执行程序。

从 `router.SetRouter()` 的实现看，进程启动后会在同一个 Gin Server 上同时注册：

- `SetApiRouter()`：管理 API、用户 API、系统设置、支付、订阅、性能等接口
- `SetDashboardRouter()`：兼容旧 dashboard 接口
- `SetRelayRouter()`：OpenAI / Claude / Gemini / Midjourney / Suno 等 Relay 入口
- `SetVideoRouter()`：视频相关代理接口

因此从代码结构上看，管理面和转发面是共存在一个进程里的，而不是天然拆成：

```text
一个专用管理服务器
  +
多个专用转发服务器
```

### `slave` 节点实际减少了什么

`NODE_TYPE=slave` 的效果主要是减少“控制面维护职责”，而不是只保留 Relay 路由。

当前代码中，`slave` 节点主要会：

- 跳过数据库迁移
- 不启动订阅重置、渠道自动测试、Codex 凭证刷新、上游模型巡检等后台任务

但路由层并没有把 `/api` 和 `/v1` 做严格分离；也就是说，`slave` 不是“代码意义上的纯转发节点”，而更像：

```text
同一个单体后端
  master: 负责迁移 + 后台维护任务 + 同样提供 API/Relay
  slave:  不负责迁移和部分后台任务 + 仍然提供 API/Relay
```

如果部署上需要“1 个管理节点 + N 个纯转发节点”的硬隔离，通常还需要依赖网关、反向代理或额外代码改造来限制 `slave` 节点暴露的路由范围。

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

换句话说，从当前实现来看，管理 API 和转发 API 不是两个后端服务，而是同一个后端服务里的两类路由面。

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

# 07 - 配置系统与数据存储兼容性

## 配置系统

配置系统是这个项目非常有辨识度的一部分。

### `setting/` 包到底是什么

`setting/` 不是单纯“放配置文件的目录”，而更接近一层**运行时配置定义层**。  
这里的代码主要做三件事：

1. 定义配置结构和默认值
2. 提供运行时读取入口，例如 `GetPaymentSetting()`、`GetFetchSetting()`
3. 把配置对象注册到统一配置管理器，或接入旧版 `OptionMap` 热更新链路

因此从职责上看，可以把这套设计拆成三层：

```text
setting/*
  配置定义层
  - struct
  - 默认值
  - getter
  - 注册到 ConfigManager

model.Option / common.OptionMap
  配置缓存与同步层
  - options 表读写
  - 进程内缓存
  - 热更新分发

database.options
  配置持久化层
```

这也是为什么阅读 `setting/` 时会感觉“像配置文件”，但真正生效又明显不是从 YAML / TOML / JSON 文件直接加载。

### 三层来源

配置大致来自三层：

1. 代码默认值
2. 环境变量
3. 数据库 options 表

更准确地说：

- **代码默认值**
  写在 `setting/*` 或 `common/*` 的全局变量与默认 struct 中。
- **环境变量**
  主要由 `common.InitEnv()` 在启动早期加载，适合端口、数据库、Redis、节点角色、同步周期等基础运行参数。
- **数据库 `options` 表**
  这是绝大多数“后台可改系统设置”的最终持久化位置。

因此日常运维里看到的“站点设置”“支付设置”“签到设置”“主题设置”等，主来源通常不是 `.env`，而是 `options` 表。

### 运行时热更新

`model.SyncOptions()` 定时从数据库同步到内存，说明大部分系统设置都支持热更新，不需要重启。

这个热更新有两种生效路径：

1. **当前节点本地立即生效**
   管理接口调用 `model.UpdateOption()` 后，会先落库，再立刻调用 `updateOptionMap()` 更新当前进程内存。
2. **其他节点延迟同步生效**
   其他节点依靠 `model.SyncOptions(common.SyncFrequency)` 周期性从数据库拉取更新。

所以更准确的描述是：

```text
当前节点：改完立即生效
其他节点：下一个同步周期生效
```

### 默认同步周期不是“几分钟”，而是 60 秒

这个项目里多个重要的运行时同步都共享 `SYNC_FREQUENCY` 这一环境变量，默认值是 `60` 秒。

它至少影响两类常驻内存数据：

1. `OptionMap` 配置热更新
2. `ChannelCache` 渠道内存缓存刷新

因此如果没有显式修改环境变量，系统默认不是“每几分钟刷新一次”，而是：

```text
每 1 分钟轮询数据库
```

### 这套热更新是直接读数据库，不经过 Redis

这里很容易误解，因为项目整体确实用了 Redis。

但对于：

- `options` 表配置同步
- `channels` / `abilities` 对应的渠道内存缓存同步

当前实现都是：

```text
database -> 当前进程内存
```

而不是：

```text
database -> Redis -> 当前进程内存
```

也就是说，Redis 并不是这两类运行时热数据的中心广播层。

### 两种组织方式

项目里配置并不是单一风格：

1. 老风格：很多配置直接映射到 `common.OptionMap`
2. 新风格：通过 `setting/config/ConfigManager` 统一注册结构化配置

这表明项目正处在“从散式配置向结构化配置演进”的过程中。

### 新风格配置：结构化 `ConfigManager`

新风格配置的典型写法是：

- 在 `setting/...` 中定义一个 struct
- 提供默认值
- 在 `init()` 里调用 `config.GlobalConfig.Register(...)`
- 业务代码通过 `GetXXXSetting()` 读取

例如：

- `operation_setting.PaymentSetting`
- `operation_setting.GeneralSetting`
- `operation_setting.CheckinSetting`
- `operation_setting.TokenSetting`
- `operation_setting.MonitorSetting`
- `system_setting.FetchSetting`
- `console_setting.ConsoleSetting`

这些配置在数据库中的键名是扁平化的：

```text
payment_setting.amount_options
payment_setting.amount_discount
general_setting.docs_link
checkin_setting.enabled
fetch_setting.enable_ssrf_protection
console_setting.announcements
```

也就是说，代码里是结构化对象，落库时仍然是 `options` 表中的 `key/value`。

### 旧风格配置：`OptionMap` + 全局变量

旧风格配置没有统一 struct，而是：

- 先在 `model.InitOptionMap()` 里声明默认键值
- 再从数据库逐项覆盖
- `updateOptionMap()` 根据 key 把值同步到 `common`、`setting`、`operation_setting` 等包级变量

典型例子包括：

- `Price`
- `USDExchangeRate`
- `MinTopUp`
- `StripeApiSecret`
- `TopupGroupRatio`
- `ModelRatio`
- `AutomaticDisableKeywords`

这类配置依然支持热更新，但代码组织上更分散，也更依赖字符串 key。

### group 定义主要落在 `options` 表，而不是独立 `groups` 表

`group` 在这个项目里很重要，但当前并没有单独的 `groups` 主表。

分组定义本身主要散落在 `options` 表里的几项配置中，最核心的是：

- `GroupRatio`
  这是系统当前 group 集合最核心的来源。配置 JSON 的 key 基本就构成了“系统里有哪些 group”。

- `UserUsableGroups`
  定义 group 的展示文案，以及默认哪些 group 可供用户使用。

- `AutoGroups`
  定义 `auto` 分组模式下会轮询哪些真实 group。

- `GroupGroupRatio`
  定义“用户所属 group -> 实际使用 group”的特殊倍率映射。

此外还有一些与 group 强相关、但不直接充当“group 名单定义”的配置：

- `TopupGroupRatio`
- `ModelRequestRateLimitGroup`

因此从配置角度更准确地说：

```text
没有独立 groups 表
  -> group 名单主要来自 options.GroupRatio
  -> 其他 options 键补充分组的可用性、auto 路由、特殊倍率、限流和充值规则
```

而业务表中的：

- `users.group`
- `tokens.group`
- `channels.group`
- `abilities.group`

主要是在引用这些配置里约定好的 group 名。

### `setting/` 内部也不是完全统一的

虽然很多新代码已经迁到结构化配置，但 `setting/` 目录内部仍然同时存在几种形态：

1. **结构化系统设置**
   如 `setting/operation_setting/*.go`、`setting/system_setting/*.go`、`setting/console_setting/*.go`
2. **旧式全局变量设置**
   如 `setting/payment_creem.go`、`setting/payment_stripe.go`、`setting/payment_waffo.go`
3. **带辅助序列化逻辑的配置**
   如倍率、支付方式、敏感词、自动分组等，会额外提供 `ToJSONString()` / `Update...ByJSONString()` 一类函数

所以 `setting/` 更像“配置相关能力集合”，而不是一套已经彻底统一完毕的配置框架。

### 启动时如何装载

配置装载顺序大致如下：

1. `common.InitEnv()`
   先加载环境变量和基础运行参数。
2. `model.InitDB()`
   建立数据库连接。
3. `model.InitOptionMap()`
   先填充代码默认值，再从 `options` 表加载数据库配置覆盖到内存。
4. `go model.SyncOptions(common.SyncFrequency)`
   服务启动后继续周期性热同步。

这意味着：

- `.env` / 环境变量更偏基础运行参数
- `setting` 默认值是兜底
- 数据库 `options` 才是大多数后台设置的真实来源

### master / slave 是否都会读取这些设置

会。  
`model.SyncOptions(common.SyncFrequency)` 在 `main()` 中是无条件启动的，不区分 `master` 还是 `slave`。

因此：

- `master` 会定期同步配置
- `slave` 也会定期同步配置
- 两者共享同一个 `options` 表时，能够保持系统设置最终一致

但也要注意两点：

1. 这是**轮询同步**，不是数据库变更订阅
2. 多节点之间存在一个 `SYNC_FREQUENCY` 长度的最终一致性窗口，默认是 60 秒

### master / slave 不只是都会同步 `options`，也都会刷新渠道内存缓存

除了配置同步外，只要开启了 `MEMORY_CACHE_ENABLED`，服务启动时还会：

1. 先执行一次 `InitChannelCache()`
2. 再后台启动 `SyncChannelCache(common.SyncFrequency)`

这一段同样不区分 `master` 还是 `slave`。

因此在运行时语义上：

- `master` 会定期从数据库刷新渠道缓存
- `slave` 也会定期从数据库刷新渠道缓存
- 两者对 `channel` / `group -> model -> channel` 路由索引的理解依靠轮询保持最终一致

真正只在 `master` 上运行的，更多是迁移、订阅重置、Codex 凭证自动刷新、上游模型巡检等后台任务，而不是这类基础缓存同步。

### 运行期能不能动态修改

可以，答案是明确的“能”。

常见路径是：

1. 管理后台或管理 API 提交新设置
2. `controller.UpdateOption` 调用 `model.UpdateOption`
3. 新值写入 `options` 表
4. 当前节点立即刷新内存
5. 其他节点在下一轮 `SyncOptions` 时读到新值

所以从系统行为上看，这套配置系统已经具备：

- 持久化
- 运行时修改
- 多节点传播

它并不是“只在启动时读取一次的静态配置文件系统”。

## 运行时数据到底分布在哪一层

这个项目如果只看名字，很容易以为“既然用了 Redis，那大多数热数据都在 Redis”。但实际并不是这样。

更准确的理解应该是：

### 1. 配置与渠道路由：以进程内存为主，数据库为最终来源

这类数据包括：

- `OptionMap`
- `ratio_setting` 中的倍率映射
- `channelsIDM`
- `group2model2channels`

它们的特点是：

- 启动时加载到当前进程
- 后台定时从数据库轮询刷新
- 当前节点更新时可立即改本地内存
- 其他节点最终通过轮询追平

因此它们更接近：

```text
DB 持久化
进程内存热读
多节点轮询同步
```

### 2. 用户 / Token 缓存：Redis 优先，失败回落数据库

另一类数据则明显更依赖 Redis，例如：

- 用户基础信息
- 用户额度
- 用户分组
- token 信息

这一层的典型行为是：

1. 先尝试从 Redis 读
2. Redis miss 或失败时回退数据库
3. 成功从数据库取到后，再异步回填 Redis

因此 Redis 在本项目中更像是：

- 面向高频实体读取的共享缓存层
- 而不是配置与路由状态的统一事件分发层

### 3. 多 key channel 状态：主要跟着 channel 一起存

对于多 key 渠道，单 key 的临时禁用状态并没有独立拆成 Redis 中央状态表，而是保存在 `ChannelInfo` 中，例如：

- `MultiKeyStatusList`
- `MultiKeyDisabledReason`
- `MultiKeyDisabledTime`

这进一步说明，渠道状态体系仍然以：

- 当前进程内存中的 channel 对象
- 数据库中的 channel 持久化记录

为主，而不是以 Redis 为核心状态机。

## 多节点一致性模型与它的边界

从上面的设计可以推导出一个很重要的架构特征：项目大量使用的是**最终一致性轮询同步**，不是强一致或主动推送。

这具体表现为：

1. 某节点修改配置或更新 channel 状态时，会先影响自己
2. 状态被写入数据库
3. 其他节点在下一轮 `SYNC_FREQUENCY` 中读取到变化
4. 在这个窗口内，不同节点可能持有不完全一致的内存视图

这套机制实现简单，也足够实用，但代价是：

- 配置更新不是瞬时全网生效
- channel 自动禁用不是全节点同时切换
- 某个节点的局部判断可能在后续传播为全局状态

因此如果从分布式系统视角概括，可以说：

```text
本项目对配置和渠道状态采用“数据库为事实来源、节点本地内存为热缓存、轮询实现多节点最终一致”的同步模型。
```

## 数据存储策略

项目明确支持：

- SQLite
- MySQL
- PostgreSQL

这不是 README 层面的口号，代码里确实做了大量兼容工作：

- `model/main.go` 中区分保留字引用方式
- 布尔值 SQL 表达差异兼容
- SQLite 特殊迁移逻辑
- 主库和日志库拆分

这意味着 `model/` 层不仅是 ORM 层，也是数据库方言适配层。

## Migration 策略

这里要把两种“迁移”分开看：**数据库 schema migration** 和 **业务配置数据迁移**。

### Schema migration：启动时自动执行，不是定时任务

当前项目的 schema migration 不是通过定时任务触发，也不是通过独立 CLI 子命令触发，而是嵌在启动流程里：

- `InitResources()` 启动时先调用 `model.InitDB()`
- 主库初始化完成后，再调用 `model.InitLogDB()`
- 两者都会先连接数据库、设置连接池
- 只有 `common.IsMasterNode` 为真时，才真正执行 `migrateDB()` / `migrateLOGDB()`

因此运维语义上更准确的描述是：

```text
master 节点每次启动或重启时自动尝试 migration
slave 节点只连接数据库，不执行 migration
```

几个要点：

- 不是周期性任务
  代码里没有看到 ticker/cron 定时跑 schema migration。
- 不是外部手工步骤
  默认部署路径下，不需要先手动执行独立迁移命令。
- 是启动前置
  migration 发生在服务正式监听端口之前；若失败，启动会中断。
- 主库 / 日志库分离
  业务主表和日志表的迁移入口不同，日志库在配置 `LOG_SQL_DSN` 时独立处理。

### Schema migration 的实现风格

当前风格不是“版本号 + 一串 migration 文件”，而是：

- 以 `gorm.AutoMigrate(...)` 为主体
- 配合少量手写、幂等式的兼容迁移函数
- 针对 SQLite / MySQL / PostgreSQL 差异单独分支处理

已确认的显式兼容迁移包括：

- `migrateTokenModelLimitsToText()`
  把 `tokens.model_limits` 迁为 `text`
- `migrateSubscriptionPlanPriceAmount()`
  把 `subscription_plans.price_amount` 迁为 `decimal(10,6)`
- `ensureSubscriptionPlanTableSQLite()`
  用 SQLite 兼容 DDL 和补列逻辑保证 `subscription_plans` 结构完整

这类设计的特点是：

- 对新增字段、索引、普通表扩展比较省心
- 对复杂重构、数据回填、强版本顺序控制则相对弱一些

### 业务配置数据迁移：目前看到的是手动接口触发

除了 schema migration，项目里还存在少量“业务数据迁移”逻辑，但它们不是开机自动跑的。

目前最明确的一例是：

- `POST /api/option/migrate_console_setting`
- Root 权限
- 对应 `controller/MigrateConsoleSetting`

它做的是把旧版 `options` 表里的若干控制台配置键迁移到新的 `console_setting.*` 命名空间，并清理旧键。

所以从运维视角看，可以把当前迁移机制理解成：

```text
Schema 迁移：master 启动时自动执行
业务配置数据迁移：按需通过管理接口手动触发
```

## 多节点部署建议

结合启动逻辑、`NODE_TYPE` 分支和 Redis / DB 初始化代码，可以把当前项目的多节点模型理解为：

```text
同一个后端程序
  + 1 个 master 节点
  + N 个 slave 节点
  + 共享主数据库
  + 可选共享日志数据库
  + 共享 Redis
```

这里的 `master/slave` 不是两个不同可执行程序，而是同一个程序的两种运行角色。

### `master` 和 `slave` 的实际分工

当前代码里，职责划分主要是“控制面是否执行维护任务”，而不是“是否提供不同 HTTP 路由”。

- `master`
  负责数据库迁移，以及渠道自动测试、订阅重置、Codex 凭证刷新、上游模型巡检等后台任务。
- `slave`
  不负责数据库迁移，也不启动上述控制面后台任务。

但两者都会启动同一个 Gin 服务，也都会注册管理 API 和 Relay API。  
因此它更像“数据面平行扩容 + 控制面由 master 单点负责”，而不是严格意义上的“管理服务 / 转发服务”物理拆分。

### 推荐部署拓扑

如果要把当前代码以比较稳妥的方式部署成多节点，比较推荐的结构是：

```text
                +------------------+
                |  MySQL/PostgreSQL|
                +------------------+
                         ^
                         |
                +------------------+
                |      Redis       |
                +------------------+
                         ^
                         |
      +------------------+------------------+
      |                                     |
+-------------+                    +-------------------+
|   master    |                    |   slave x N       |
| new-api     |                    |   new-api         |
| NODE_TYPE=  |                    | NODE_TYPE=slave   |
| master      |                    |                   |
+-------------+                    +-------------------+
```

推荐做法：

- 只保留一个 `master`
- 用多个 `slave` 做水平扩容
- 所有节点连接同一套主数据库
- 所有节点连接同一个 Redis
- 如启用独立日志库，所有节点也应连接同一个 `LOG_SQL_DSN`

### 为什么通常只能有一个 `master`

当前代码没有看到 leader election、自动选主或分布式协调机制。

这意味着：

- `master/slave` 是靠环境变量人工指定的
- 如果同时跑多个 `master`，它们可能会一起执行迁移和后台任务
- 这类并发维护行为并没有被设计成天然多主安全

因此部署上更合理的约束是：**一个集群内只设置一个 `master`**。

### DB 和 Redis 为什么需要共享

共享数据库是多节点正确工作的前提，因为系统状态主要保存在数据库中：

- 用户、令牌、渠道、能力、选项、订阅、任务、日志等都在 DB
- `slave` 节点虽然不跑迁移，但仍然会读写业务数据

共享 Redis 则主要用于缓存与多节点行为一致性：

- 用户 / Token 缓存
- 渠道亲和性缓存
- 其他依赖 Redis 的缓存或限流辅助逻辑

如果各节点各用一套 Redis，会让缓存命中、亲和性和部分状态行为变得割裂。

### Redis 在当前项目里到底承担什么职责

Redis 在这个项目里不是“必须有”的主存储，而是一层可选增强。

- 配置了 `REDIS_CONN_STRING` 时启用 Redis
- 没配置时，`common.InitRedisClient()` 会把 `common.RedisEnabled` 设为 `false`
- 关闭 Redis 后，部分能力会退回数据库直读或进程内缓存 / 限流实现

因此从系统定位上看，Redis 主要承担三类职责：

1. 热路径对象缓存
2. 限流计数与短时间窗口状态
3. 短周期实时指标聚合

需要特别注意的是：当前项目的登录 session 不是放在 Redis，而是 Gin 的 cookie session store。也就是说，Redis 不承担控制台登录态持久化职责。

### Redis 的接入方式

`common/redis.go` 封装了本项目最基础的 Redis 访问方法，包括：

- `SET` / `GET`
- `DEL`
- `HSET` / `HGETALL`
- `INCRBY`
- `HINCRBY`
- `HSET field`

其中有两种使用风格：

1. **直接使用 `common.RDB`**
   典型场景是限流、性能指标聚合。
2. **走项目封装**
   典型场景是用户缓存、Token 缓存，以及 `pkg/cachex` 提供的 namespaced string cache。

从 value 组织方式看，当前 Redis 数据大致分成三类：

- `Hash`
  用于缓存结构化对象，如用户、Token、性能 bucket。
- `String`
  用于保存简单计数、整数 ID 或 JSON 串。
- `List`
  用于滑动时间窗限流。

### Redis key 总览

下面按“key 模式 / 含义 / value 类型 / value 结构”整理当前代码里实际会写入 Redis 的 key。

#### 1. 用户缓存

key 模式：

```text
user:<userId>
```

含义：

- 缓存用户基础信息
- 用于减少登录态用户信息、分组、额度、设置等字段的数据库读取

value 类型：

- `Hash`

字段结构：

- `Id`
- `Group`
- `Email`
- `Quota`
- `Status`
- `Username`
- `Setting`

value 形态：

- 所有字段最终都按字符串写入 Redis
- 例如：

```text
Id = "12"
Group = "default"
Email = "alice@example.com"
Quota = "100000"
Status = "1"
Username = "alice"
Setting = "{\"language\":\"zh-CN\"}"
```

补充说明：

- `Setting` 本质上是 `dto.UserSetting` 的 JSON 字符串
- 单字段更新时，会直接更新 hash 里的某个 field，如 `Quota`、`Group`、`Username`
- TTL 使用 `common.RedisKeyCacheSeconds()`，其值等于 `SYNC_FREQUENCY`，默认 60 秒

#### 2. Token 缓存

key 模式：

```text
token:<hmac(tokenKey)>
```

含义：

- 缓存 API Token 对象
- 用于请求鉴权和额度检查时减少数据库访问

value 类型：

- `Hash`

字段结构来自 `model.Token`，主要包括：

- `Id`
- `UserId`
- `Status`
- `Name`
- `CreatedTime`
- `AccessedTime`
- `ExpiredTime`
- `RemainQuota`
- `UnlimitedQuota`
- `ModelLimitsEnabled`
- `ModelLimits`
- `AllowIps`
- `UsedQuota`
- `Group`
- `CrossGroupRetry`

value 形态：

- 仍然全部按字符串形式写入 Redis
- bool 会被写成 `"true"` / `"false"`
- 指针字段若为空，会被写成空字符串

补充说明：

- Redis key 用的不是明文 token，而是 `common.GenerateHMAC(token.Key)` 的结果
- 写缓存前会调用 `token.Clean()`，因此缓存对象本身不会保存明文 `Key`
- `RemainQuota` 会通过 `HINCRBY` 做原子增减
- TTL 同样使用 `SYNC_FREQUENCY`，默认 60 秒

#### 3. 通用 IP 限流

key 模式：

```text
rateLimit:<mark><clientIP>
```

例子：

```text
rateLimit:GW127.0.0.1
rateLimit:GA203.0.113.8
rateLimit:CT198.51.100.10
```

含义：

- 基于客户端 IP 的全局 / 关键接口 / 上传下载限流

当前看到的 `mark` 包括：

- `GW`：Global Web
- `GA`：Global API
- `CT`：Critical
- `DW`：Download
- `UP`：Upload

value 类型：

- `List`

value 形态：

- 列表元素是请求时间字符串
- 时间格式固定为：

```text
2006-01-02T15:04:05.000Z
```

补充说明：

- 通过 `LLen`、`LPush`、`LIndex`、`LTrim` 组合实现滑动窗口风格限流
- key 的过期时间统一使用 `common.RateLimitKeyExpirationDuration`，当前默认 20 分钟

#### 4. 按用户 ID 的限流

key 模式：

```text
rateLimit:<mark>:user:<userId>
```

例子：

```text
rateLimit:SR:user:42
```

含义：

- 对已经认证的用户按 userId 做限流
- 当前明确看到 `SearchRateLimit` 走这套 key

value 类型：

- `List`

value 形态：

- 与 IP 限流完全一致，也是时间字符串列表

#### 5. 邮箱验证码发送限流

key 模式：

```text
emailVerification:EV:<clientIP>
```

例子：

```text
emailVerification:EV:127.0.0.1
```

含义：

- 控制同一 IP 在短时间内发送邮箱验证码的频率

value 类型：

- `String`

value 形态：

- 一个整数计数器字符串，如 `"1"`、`"2"`、`"3"`

TTL：

- 30 秒

补充说明：

- 第一次 `INCR` 成功后才设置过期时间
- 逻辑限制是 30 秒内最多 2 次

#### 6. 通知发送限流

key 模式：

```text
notify_limit:<userId>:<notifyType>:<YYYYMMDDHH>
```

例子：

```text
notify_limit:42:email:2026060113
```

含义：

- 控制用户在某个小时窗口内的通知发送次数

value 类型：

- `String`

value 形态：

- 整数计数器字符串

TTL：

- `NOTIFICATION_LIMIT_DURATION_MINUTE` 分钟
- 默认初始化值是 10 分钟

#### 7. 渠道亲和性缓存

key 模式：

```text
new-api:channel_affinity:v1:<suffix>
```

其中 `<suffix>` 的拼接规则是：

```text
[ruleName][:modelName][:usingGroup]:affinityValue
```

是否包含 `ruleName` / `modelName` / `usingGroup`，由具体规则的：

- `IncludeRuleName`
- `IncludeModelName`
- `IncludeUsingGroup`

决定。

含义：

- 让同一类“亲和 key”尽量命中相同的 channel
- 用于多 key / 多 channel 场景下提高请求黏性和一致性

value 类型：

- `String`

value 形态：

- 被选中的 `channelID`，以整数串存储
- 例如：

```text
"9527"
```

补充说明：

- 这类 key 通过 `pkg/cachex.HybridCache[int]` 管理
- Redis codec 是 `IntCodec`
- TTL 由规则自身 `TTLSeconds` 或全局 `DefaultTTLSeconds` 决定
- 这里的 key 后缀直接包含原始 `affinityValue`，不是哈希值

#### 8. 渠道亲和性 usage 统计缓存

key 模式：

```text
new-api:channel_affinity_usage_cache_stats:v1:<ruleName>\n<usingGroup>\n<keyFp>
```

含义：

- 记录某个渠道亲和性 key 在窗口期内的命中与 usage 聚合信息

value 类型：

- `String`

value 形态：

- JSON 串，对应 `ChannelAffinityUsageCacheCounters`

字段包括：

- `cached_token_rate_mode`
- `hit`
- `total`
- `window_seconds`
- `prompt_tokens`
- `completion_tokens`
- `total_tokens`
- `cached_tokens`
- `prompt_cache_hit_tokens`
- `last_seen_at`

示意：

```json
{
  "cached_token_rate_mode": "cached_over_prompt",
  "hit": 12,
  "total": 18,
  "window_seconds": 3600,
  "prompt_tokens": 12000,
  "completion_tokens": 4500,
  "total_tokens": 16500,
  "cached_tokens": 6000,
  "prompt_cache_hit_tokens": 0,
  "last_seen_at": 1717230000
}
```

补充说明：

- `keyFp` 不是原始 affinity value，而是其 SHA1 前 8 位摘要
- 这类 key 同样由 `pkg/cachex.HybridCache` 管理
- TTL 等于该条统计对应的窗口秒数，通常与 affinity TTL 对齐

#### 9. 订阅套餐缓存

key 模式：

```text
new-api:subscription_plan:v1:<planId>
```

含义：

- 缓存订阅套餐 `SubscriptionPlan`

value 类型：

- `String`

value 形态：

- `SubscriptionPlan` 的 JSON 串

字段很多，典型包括：

- `Id`
- `Title`
- `Subtitle`
- `PriceAmount`
- `Currency`
- `DurationUnit`
- `DurationValue`
- `Enabled`
- `UpgradeGroup`
- `TotalAmount`

TTL：

- 由 `SUBSCRIPTION_PLAN_CACHE_TTL` 控制
- 默认 300 秒

#### 10. 订阅套餐标题信息缓存

key 模式：

```text
new-api:subscription_plan_info:v1:sub:<userSubscriptionId>
```

含义：

- 缓存“某条用户订阅记录对应的套餐标题信息”

value 类型：

- `String`

value 形态：

- JSON 串，对应：

```json
{
  "PlanId": 3,
  "PlanTitle": "Pro Monthly"
}
```

TTL：

- 由 `SUBSCRIPTION_PLAN_INFO_CACHE_TTL` 控制
- 默认 120 秒

#### 11. 实时性能指标 bucket

key 模式：

```text
perf:<model>:<group>:<bucketTs>
```

例子：

```text
perf:gpt-4o:default:1717228800
```

含义：

- 暂存某个模型、某个分组、某个时间 bucket 的实时性能聚合数据
- 后续查询会把 Redis 中“当前活动 bucket”与数据库历史聚合结果合并

value 类型：

- `Hash`

字段结构：

- `req`
- `ok`
- `lat`
- `ttft`
- `ttft_n`
- `out`
- `gen_ms`

含义分别是：

- `req`：请求数
- `ok`：成功数
- `lat`：总延迟毫秒
- `ttft`：TTFT 总和毫秒
- `ttft_n`：TTFT 样本数
- `out`：输出 token 总数
- `gen_ms`：生成耗时总和毫秒

value 形态：

- 全部是整数计数字段，按 Redis hash field 存储

TTL：

- 1 小时

### 当前没有放进 Redis 的典型状态

有几个点很容易被误判：

- 登录 session 不在 Redis
  当前明确使用的是 cookie store
- `model/channel_cache.go` 的渠道缓存是进程内内存缓存，不是 Redis
- 一些旧的 `constant/cache_key.go` 常量，如 `user_group:%d`，当前代码中并没有看到实际 Redis 读写落到这些 key

因此如果线上排查 Redis 数据，优先应该围绕上面列出的实际 key 模式，而不是只看常量名或猜测性的缓存命名。

### SQLite 不适合作为多节点共享主库

代码支持 SQLite，但它更适合单机或本地开发。

如果多节点部署时没有配置 `SQL_DSN`，程序会退回本地 SQLite 文件。这会导致每个节点拥有自己的本地库，而不是共享业务状态，因此不适合作为真正的多节点集群方案。

多节点正式部署更合理的选择是：

- MySQL
- PostgreSQL

### 多节点必须统一的关键配置

如果多个节点属于同一个逻辑集群，至少以下配置应保持一致：

- `SESSION_SECRET`
  否则多机之间的会话无法互认。
- `CRYPTO_SECRET`
  否则加密字段和相关功能可能不兼容。
- `SQL_DSN`
  所有节点应连接同一主库。
- `LOG_SQL_DSN`
  如果使用独立日志库，应指向同一日志库。
- `REDIS_CONN_STRING`
  所有节点应连接同一个 Redis。

### 对外暴露建议

由于代码层并没有把 `slave` 节点限制成“只开 Relay 路由”，如果想实现更接近“管理节点 + 转发节点”的效果，通常需要依赖部署层做额外约束：

- `master`：可暴露完整管理入口和 Relay 入口
- `slave`：建议只对外暴露 `/v1`、`/v1beta`、`/mj`、`/suno` 等转发路径
- `/api`、管理后台入口更适合只经由 `master` 暴露

也就是说，现阶段“纯转发节点”更多是**部署策略**，而不是**代码内建角色**。

### 建议暴露的接口边界

如果把 `master` 作为主控制节点、把 `slave` 作为主要转发节点，比较实用的暴露策略如下。

#### `master` 建议暴露

- `/api/*`
  管理 API、用户 API、系统设置、支付、订阅、性能接口等。
- 管理后台前端入口
  即控制台相关页面和静态资源入口。
- 可选保留 Relay 入口
  如 `/v1/*`、`/v1beta/*`、`/mj/*`、`/suno/*`，用于兜底、调试或低流量场景。

`master` 的核心定位更适合是：

```text
控制面主节点
  + 管理后台
  + 配置变更入口
  + 定时任务与迁移
  + 可选参与少量转发
```

#### `slave` 建议暴露

- `/v1/*`
- `/v1beta/*`
- `/mj/*`
- `/suno/*`
- 其他明确属于 Relay / 代理能力的入口

`slave` 更适合承接绝大多数模型调用流量，也就是：

```text
数据面转发节点
  + OpenAI 兼容请求
  + Gemini / Claude 等兼容请求
  + Midjourney / Suno / 视频类任务提交与查询
```

#### `slave` 不建议直接暴露

- `/api/*`
  尤其是管理员、Root、系统设置、支付回调、性能管理等控制面接口。
- 管理后台页面入口
- 其他仅用于平台运营和系统维护的入口

这样做的原因不是代码禁止，而是为了让职责边界更清晰：

- 管理流量集中到 `master`
- 模型转发流量主要落到 `slave`
- 降低把控制面接口暴露到所有转发节点的风险

#### 一个更贴近实际的部署理解

因此在推荐部署里，可以把两类节点理解成：

```text
master = 控制面主节点，兼具转发能力
slave  = 数据面转发节点，不承担控制面维护任务
```

这也是为什么虽然 `master` 代码上同样能转发，但在实际集群里，通常仍然会让 `slave` 承担大部分转发请求。

## 为什么这一层重要

对这个项目来说，配置和存储不是纯基础设施，它们直接决定：

- 哪些模型可用
- 哪些分组存在
- 哪些用户能访问什么
- 如何计费
- 如何在不同数据库上安全运行

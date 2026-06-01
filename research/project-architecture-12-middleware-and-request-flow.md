# 12 - 中间件职责与请求链路

## 为什么这一节值得单独写

这个项目的很多核心行为并不在 Controller 里直接完成，而是在 `middleware/` 这一层提前决定：

- 这个请求属于哪一类入口
- 它用什么身份进入系统
- 是否被限流或拦截
- 它最终会被分发到哪个渠道
- 请求结束后要不要做清理、记录和缓存控制

所以理解中间件，基本就等于理解这个项目的请求运行时。

## 中间件可以分成哪几类

从职责上看，`middleware/` 里的逻辑大致可以分成五类：

1. 基础请求处理
2. 认证与权限
3. 限流与安全防护
4. Relay 分发与上下文注入
5. 协议适配与入口兼容

下面按这个结构整理。

## 1. 基础请求处理

### `RequestId()`

作用：

- 为每个请求生成唯一请求 ID
- 写入 `gin.Context`
- 写入 `request context`
- 回写到响应头

这让后续日志、错误信息、渠道调用链都能挂上统一的 request id。

它是全局中间件，在 `main.go` 里最早挂载的一批之一。

### `PoweredBy()`

作用：

- 给响应添加 `X-New-Api-Version`

这是一个很轻量的响应标识中间件，主要用于暴露当前服务版本。

### `I18n()`

作用：

- 为当前请求确定语言
- 优先使用用户设置
- 其次读取 `Accept-Language`
- 最后回落到默认语言

后面的很多错误信息和 API 返回文案都依赖它在 context 里设置的语言信息。

### `RouteTag(tag)`

作用：

- 给请求打上分类标签，比如 `api`、`relay`、`old_api`、`web`

它本身不做业务判断，主要给日志系统提供结构化上下文。

### `SetUpLogger()`

作用：

- 注册 Gin 的请求日志格式
- 把时间、路由标签、请求 ID、状态码、耗时、客户端 IP、方法、路径串起来输出

所以这个项目的访问日志不是默认 Gin 格式，而是带业务标签的自定义格式。

### `CORS()`

作用：

- 允许跨域访问
- 放开常见方法
- 接受任意请求头

它主要用于 API / Relay 入口，保证浏览器和第三方客户端能直接调用。

### `Cache()`

作用：

- 给 Web 静态资源设置缓存策略
- 首页 `/` 不缓存
- 其余静态资源默认缓存一周
- 额外下发一个 `Cache-Version`

这说明前端静态资源的缓存控制是后端统一注入的，而不是完全依赖前端构建产物本身。

### `DisableCache()`

作用：

- 对敏感响应显式禁止缓存

现在主要挂在“查看 key”这类高风险接口上，例如查看 token key、channel key。

### `DecompressRequestMiddleware()`

作用：

- 自动解压 `gzip` / `br` 压缩请求体
- 对解压后的请求体做大小限制

这主要服务于 Relay 入口，因为模型调用场景下客户端更可能直接上传压缩请求体。

### `BodyStorageCleanup()`

作用：

- 在请求结束后清理请求体缓存
- 清理临时文件来源缓存

这是一个典型的“请求结束钩子”中间件，避免可重复读取请求体和文件下载缓存持续占用磁盘/内存。

### `StatsMiddleware()`

作用：

- 统计当前活跃连接数

它的逻辑很简单，但体现出 Relay 层对“当前并发压力”有基础观测需求。

## 2. 认证与权限

### `TryUserAuth()`

作用：

- 尝试从 Session 中拿用户 ID
- 如果拿不到也不拦截

适合“游客也能访问，但如果用户已登录就顺带识别”的接口。

### `UserAuth()`

作用：

- 要求用户已登录
- 支持 Session 或 access token
- 校验 `New-Api-User` 请求头是否和当前用户一致
- 校验用户状态是否禁用
- 把用户身份写入 context

也就是说，这里的控制台鉴权不是简单判断 session 是否存在，而是带有额外的用户绑定校验。

### `AdminAuth()`

作用：

- 在 `UserAuth()` 的基础上要求管理员权限

主要用于渠道管理、日志管理、用户管理等运营面接口。

### `RootAuth()`

作用：

- 在 `UserAuth()` 的基础上要求 root 权限

主要用于系统级敏感能力，比如部分系统设置、渠道敏感信息查看、性能控制等。

### `TokenOrUserAuth()`

作用：

- 允许“控制台 Session 用户”或“API Token 调用方”二选一通过

适合那些既可能从前端控制台发起，又可能从外部 API 客户端发起的接口。当前视频代理里能看到这种用法。

### `TokenAuthReadOnly()`

作用：

- 宽松校验 token 是否存在
- 不检查 token 是否禁用、过期、余额耗尽
- 仍然检查 token 所属用户是否被封禁

这个设计很适合日志、用量查询类只读接口，因为系统希望即使 token 已经不可用于继续消费，也仍允许查询历史信息。

### `TokenAuth()`

这是 Relay 链路里最重要的认证中间件之一。

它的职责不只是“验证 Authorization”，还包括：

- 兼容 WebSocket 子协议里的 key
- 兼容 Anthropic 风格的 `x-api-key`
- 兼容 Gemini 风格的 query `key` 和 `x-goog-api-key`
- 兼容 Midjourney 的 `mj-api-secret`
- 统一解析 `sk-` 前缀和带后缀的 key 形式
- 校验 token 有效性
- 校验 token 的 IP 白名单
- 校验 token 所属用户状态
- 把用户分组、token 分组、模型限制、cross-group retry、指定 channel 等信息写入 context

从系统角度看，它其实是在做：

```text
不同上游兼容入口的鉴权归一化
  -> 统一成内部 token 身份模型
  -> 为分发中间件准备上下文
```

### `WssAuth()`

当前是空实现。

它更像一个曾经预留但尚未完成的 WebSocket 专用认证入口。

## 3. 限流与安全防护

### `GlobalWebRateLimit()`

作用：

- 针对 Web 前端入口做全局限流

主要挂在静态站点路由上。

### `GlobalAPIRateLimit()`

作用：

- 针对 `/api` 和旧 dashboard API 做全局限流

这是管理面 API 的第一层公共防护。

### `CriticalRateLimit()`

作用：

- 针对高风险接口做更严格的限流

典型场景包括：

- 登录
- 注册
- 邮箱验证码
- 找回密码
- 支付
- OAuth state 生成

### `SearchRateLimit()`

作用：

- 针对搜索接口按用户 ID 限流

它不是按 IP 限流，而是按认证用户限流，这样更能防止通过代理轮换绕过限制。

### `DownloadRateLimit()` 与 `UploadRateLimit()`

作用：

- 提供下载/上传类接口的限流器

当前代码里能看到实现，但在现有路由挂载里没有看到明显使用。

### `ModelRequestRateLimit()`

这是 Relay 数据面很关键的一层。

作用：

- 对模型调用按用户做请求数限制
- 同时区分“总请求数”和“成功请求数”
- 可以按 group 覆盖默认限制
- Redis 开启时走 Redis 版本，否则走内存版本

这说明它不是一个粗糙的统一频控，而是更接近“按业务资源池配置配额门槛”的限流器。

进一步看代码，它有几个很重要的实现细节：

- 它的“作用域”是 `/v1/*` 和 `/v1beta/*` 这类同步 Relay 路由
- `ModelRequestRateLimit` 这个名字容易让人误会成“按模型限流”，但实际 key 维度是“按认证用户 ID”
- 它不会把模型名拼进限流 key，所以本质上是“模型调用入口限流”，不是“每个模型单独限流”
- group 只决定“当前请求采用哪组阈值”，不会把不同 group 分开计数

可以把它理解成：

```text
当前请求命中了哪个 relay group
  -> 读取那组限流阈值
  -> 但计数桶本身仍然按 userId 共享
```

### 它限制的是哪两类计数

它同时维护两道门：

- `totalMaxCount`
  统计总请求数，包含失败请求；`0` 表示不限制
- `successMaxCount`
  统计成功请求数，只在响应状态 `< 400` 时记成功

这样做的意图比较清晰：

- 总请求数防止恶意刷失败请求
- 成功请求数控制真正的业务吞吐

### Redis 版本是怎么做的

Redis 模式下，两类计数不是同一种算法：

- 成功请求数：用 Redis List 存最近成功请求时间戳，属于滑动时间窗思路
- 总请求数：用 `common/limiter/lua/rate_limit.lua` 里的 Lua 脚本做令牌桶

总请求数那条链路的参数比较值得注意：

- `capacity = totalMaxCount * durationSeconds`
- `rate = totalMaxCount`
- `requested = durationSeconds`

这等价于：

- 每次请求消耗 `durationSeconds` 个 token
- 每秒补充 `totalMaxCount` 个 token
- 初始桶容量刚好允许一个完整时间窗内的最大请求量

所以它表达的是“在 `N` 分钟窗口内最多 `X` 次请求”，但实现方式比固定窗口更平滑。

### 内存回退版本是怎么做的

如果 Redis 没开，就退回 `common/InMemoryRateLimiter`：

- 每个 key 保存一个时间戳队列
- 超过上限时，看最老一条是否已经滑出窗口
- 后台 goroutine 按过期周期清理整条 key

这里有一个实现差异要注意：

- 总请求数的内存实现和 Redis 语义基本一致
- 成功请求数在内存模式下会先用一个 `successKey_check` 做预检查

这意味着内存模式的“成功请求限额”行为会比 Redis 版本更保守一些，更接近“先按尝试次数拦一层，再对成功请求落正式计数”，两种后端并不是完全同语义。

### group 覆盖规则

group 限流配置来自 `ModelRequestRateLimitGroup`，格式是：

```json
{
  "default": [200, 100],
  "vip": [0, 1000]
}
```

含义分别是：

- 第 1 个数字：总请求上限，`0` 表示不限
- 第 2 个数字：成功请求上限，必须大于等于 `1`

命中规则是：

- 优先取 token group
- 没有 token group 时回退到 user group
- 如果该 group 配了覆盖值，就替换全局默认值
- 如果没配，就继续用全局的 `ModelRequestRateLimitCount` 和 `ModelRequestRateLimitSuccessCount`

但要注意，覆盖的只是阈值，不是存储桶：

- Redis key 仍然只按 userId 组织
- 所以同一用户切换 group，本质上还是在共用同一套计数历史

### 哪些转发路径不会走这层限流

这一层不是所有“转发相关接口”都会经过：

- `/v1/*` 同步 Relay：会经过
- `/v1beta/*` Gemini 风格同步 Relay：会经过
- `/v1/models`、`/v1beta/models` 这类模型列表：不会经过
- `/pg/*` Playground：不会经过
- `/mj/*`、`/suno/*`、视频/任务型接口：不会经过这层，而是交给后续任务重试和上游错误处理链路

所以从架构上说，项目把“同步文本/图片/音频等主 Relay 数据面限流”和“异步任务型转发治理”分成了两套处理思路。

### `EmailVerificationRateLimit()`

作用：

- 单独限制邮箱验证码发送频率

默认策略是短时间内只允许极少次数发送，避免被刷接口。

### `TurnstileCheck()`

作用：

- 校验 Cloudflare Turnstile 人机验证
- 通过后把结果记进 session

它主要用于公开暴露、容易被刷的接口，比如注册、登录、发验证码、找回密码。

### `SecureVerificationRequired()`

作用：

- 检查当前用户是否在最近 5 分钟内做过一次安全验证
- 未验证或已过期则拒绝访问

这是典型的 step-up verification，用于读取敏感密钥等高危操作。

### `OptionalSecureVerification()`

作用：

- 不阻止请求继续
- 只是把“是否已完成安全验证”写入 context

当前路由里没有看到实际挂载，但它给未来“按验证状态显示不同能力”的接口留了口子。

### `ClearSecureVerification()`

作用：

- 清理安全验证状态

它是一个辅助函数，不是标准的链式中间件。

### `SystemPerformanceCheck()`

作用：

- 检查系统 CPU / 内存 / 磁盘是否超过阈值
- 如果超过，直接拒绝新的 Relay 请求

这说明系统对数据面调用有简单但直接的“过载保护闸门”。

它虽然不属于传统意义上的“按用户限流”，但在转发视角下很重要，因为它会在真正进入 Relay 前就返回 `503`，等价于一层“系统级总闸门”。

## 4. Relay 分发与上下文注入

### `Distribute()`

这是整个 `middleware/` 目录里最核心的运行时中间件。

它做的事包括：

- 从不同类型请求里提取模型名
- 判断当前请求属于哪一种 relay mode
- 处理不同路径的模型默认值与特例
- 检查 token 的模型访问限制
- 读取当前使用分组
- 处理 Playground 自定义 group
- 尝试命中 channel affinity
- 按 group + model 从候选渠道里选择可用 channel
- 把 channel key、base url、header override、param override、model mapping、organization、multi-key 索引等信息写入 context
- 请求成功后记录 affinity 使用结果

它的本质不是普通“转发前预处理”，而是：

```text
把外部来的 HTTP 请求
  -> 翻译成内部可执行的 Relay 上下文
```

这个上下文随后会被 `controller/relay.go` 和 `relay/channel/*` 真正消费。

## 5. 协议适配与入口兼容

### `KlingRequestConvert()`

作用：

- 把 Kling 风格视频请求改写成系统统一的视频生成入口
- 重写请求体
- 重写请求路径

这样后续逻辑就可以继续复用统一视频 relay 主链路，而不需要给 Kling 单独再写一套完整控制器流程。

### `JimengRequestConvert()`

作用：

- 把即梦官方接口的请求体改写成统一内部格式
- 根据 `Action` 决定是提交任务还是查询任务
- 重写路径、方法和部分上下文

这也是一种“先归一化再复用主链路”的做法。

## 6. 导出但当前不在主路由明显生效的项

从当前代码能直接看到几项已经导出，但在现有主路由挂载中没有明显使用：

- `WssAuth()`
- `RelayPanicRecover()`
- `OptionalSecureVerification()`
- `DownloadRateLimit()`
- `UploadRateLimit()`

这不一定代表它们没价值，更可能表示：

- 历史上用过，后来迁移了
- 为未来扩展预留
- 被别处间接调用但不在主路由文件显式挂载

## 实际请求链路怎么走

理解中间件最有帮助的方式，不是只看单个函数，而是看“某类请求实际经过哪些中间件”。

下面按几条主要路径来整理。

## 1. 全局入口链路

所有请求进入 Gin 之后，最先经过的是 `main.go` 里的全局中间件：

1. `RequestId()`
2. `PoweredBy()`
3. `I18n()`
4. Gin 自定义日志格式
5. Session 中间件

所以从一开始，所有请求就已经具备了：

- 请求 ID
- 版本响应头
- 当前语言环境
- session 读写能力
- 统一日志格式

## 2. `/api/*` 管理面 API 链路

`router/api-router.go` 中，`/api` 路由组的公共中间件大致是：

1. `RouteTag("api")`
2. `gzip.Gzip(...)`
3. `BodyStorageCleanup()`
4. `GlobalAPIRateLimit()`

然后根据具体子接口再叠加：

- `UserAuth()`
- `AdminAuth()`
- `RootAuth()`
- `CriticalRateLimit()`
- `TurnstileCheck()`
- `DisableCache()`
- `SecureVerificationRequired()`
- `SearchRateLimit()`

所以一个典型高风险接口，比如查看渠道 key：

```text
/api/channel/:id/key
  -> 全局 RequestId / I18n / Session
  -> RouteTag(api)
  -> gzip response
  -> BodyStorageCleanup
  -> GlobalAPIRateLimit
  -> RootAuth
  -> CriticalRateLimit
  -> DisableCache
  -> SecureVerificationRequired
  -> controller.GetChannelKey
  -> 请求结束后清理 body / 文件缓存
```

这条链路体现的是控制台管理面的典型思路：

- 先做公共治理
- 再做身份校验
- 再做限流和缓存控制
- 最后才进入业务控制器

## 3. `/v1/*` Relay 主链路

`router/relay-router.go` 中最核心的 OpenAI 风格 Relay 路由组是 `/v1`。

这一组的公共中间件顺序大致是：

1. `CORS()`
2. `DecompressRequestMiddleware()`
3. `BodyStorageCleanup()`
4. `StatsMiddleware()`
5. `RouteTag("relay")`
6. `SystemPerformanceCheck()`
7. `TokenAuth()`
8. `ModelRequestRateLimit()`
9. `Distribute()`
10. 具体 Relay Controller

所以一个典型的 `/v1/chat/completions` 请求，运行时可以理解成：

```text
进入系统
  -> 允许跨域
  -> 如有需要先解压请求体
  -> 注册请求结束清理逻辑
  -> 活跃连接数 +1
  -> 打上 relay 标签
  -> 检查系统是否过载
  -> 解析并验证 token
  -> 写入用户、分组、模型限制等上下文
  -> 校验模型调用频率
  -> 从请求里提取 model
  -> 根据 group / model / token 限制 / affinity 选择 channel
  -> 写入渠道 key、base URL、override、relay_mode 等上下文
  -> 进入 relay controller
  -> 请求结束后记录 affinity、连接数 -1、清理 body/临时文件
```

这就是数据面最核心的执行骨架。

## 4. `/v1/models` 与模型列表接口

`/v1/models`、`/v1beta/models`、`/v1beta/openai/models` 这些模型列表接口会走较短的链路：

1. `RouteTag("relay")`
2. `TokenAuth()`
3. 直接进入对应 controller

这里没有统一挂上 `Distribute()`，因为模型列表查询不一定需要真正选一个下游 channel 才能处理到和聊天/生成完全相同的深度。

## 5. `/pg/*` Playground 链路

`/pg` 路由组的大致链路是：

1. `RouteTag("relay")`
2. `SystemPerformanceCheck()`
3. `UserAuth()`
4. `Distribute()`
5. Playground controller

这里有两个特点：

- 它要求的是控制台用户身份，不是 API token
- `Distribute()` 会额外读取 Playground 请求里的 `group` 字段，并检查它是否属于用户可用分组

也就是说，Playground 是“控制台用户身份驱动的 Relay 入口”，不是普通外部 API 调用入口。

## 6. 视频代理链路

`router/video-router.go` 里有几条略有区别的视频路径。

### OpenAI 风格视频代理

`/v1` 下的视频代理大致会走：

1. `RouteTag("relay")`
2. `TokenAuth()` 或 `TokenOrUserAuth()`
3. `Distribute()`
4. 视频 controller

### Kling 兼容入口

`/kling/v1` 会多一层：

1. `RouteTag("relay")`
2. `KlingRequestConvert()`
3. `TokenAuth()`
4. `Distribute()`
5. 视频 controller

也就是说，Kling 先被翻译成统一内部请求，再复用后面的标准链路。

### 即梦兼容入口

`/jimeng` 则会走：

1. `RouteTag("relay")`
2. `JimengRequestConvert()`
3. `TokenAuth()`
4. `Distribute()`
5. 视频 controller

和 Kling 一样，本质也是“协议适配层 + 标准 relay 主链路”。

## 7. Midjourney / Suno 等任务型 Relay 链路

这些路由也会进入 `TokenAuth()` 和 `Distribute()`，但 `Distribute()` 内部会根据路径和动作判断：

- 这次请求是否需要重新选 channel
- 这次请求的模型名应该如何推断
- 当前 `relay_mode` 是提交任务、查任务还是通知

也就是说，对这些异步任务型平台来说，`Distribute()` 同时承担了“路径语义解析器”的角色。

## 8. Web 静态站点链路

`router/web-router.go` 里静态站点的链路更简单：

1. `gzip.Gzip(...)`
2. `GlobalWebRateLimit()`
3. `Cache()`
4. 静态资源服务
5. 默认补 `RouteTag = web`

它的主要目标不是复杂鉴权，而是：

- 压缩
- 防滥刷
- 控制缓存
- 提供前端静态资源

## 一个简化理解模型

如果把整套中间件体系压缩成一句话，可以理解成：

```text
管理面请求：
  先做身份和安全，再进控制器

Relay 请求：
  先做 token 归一化和限流，再做渠道分发，最后才真正转发
```

## 这套设计透露出的架构特征

从中间件设计本身，可以反推出这个项目的几个架构偏好：

### 1. 认证不只是“准入”，还是上下文准备

`TokenAuth()` 和 `UserAuth()` 都不是简单地做一个布尔判断，而是会把大量运行时上下文提前注入。

这意味着后续 controller / service 并不需要再次从数据库把这层信息重建一遍。

### 2. Relay 运行时是前置决策型

`Distribute()` 在进入 controller 之前就已经基本决定了：

- 模型名
- 分组
- channel
- key
- relay mode

所以 `controller/relay.go` 更像执行器，而不是第一决策者。

### 3. 安全控制是分层叠加的

同一个高危接口可能同时经过：

- 身份认证
- 关键限流
- 人机验证
- 禁止缓存
- 二次安全验证

这说明系统没有依赖单点防护，而是倾向于多层叠加。

### 4. 兼容多协议时优先做归一化

Kling、即梦这类特殊入口没有单独发展出完全平行的控制链，而是尽量先改写成统一内部格式，再接到已有视频 Relay 主链路上。

这是一种比较节制的复杂度管理方式。

## 总结

`middleware/` 在这个项目里不是普通配角，而是请求运行时的“控制平面”。

如果只记一个最重要的结论，可以记成：

- 控制台 API 侧，中间件主要负责身份、安全、限流和缓存控制
- Relay 数据面，中间件主要负责 token 归一化、请求体处理、模型限流和渠道分发

其中最关键的两个点是：

1. `TokenAuth()` 负责把各种外部调用方式统一收敛成内部 token 身份
2. `Distribute()` 负责把一个抽象模型请求落到具体渠道执行上下文

这两个中间件基本定义了整个数据面的入口形态。

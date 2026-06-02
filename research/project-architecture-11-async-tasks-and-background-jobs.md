# 11 - 异步任务与后台作业

## 为什么这一节需要补

现有文档已经解释了同步 Relay，但这个项目并不只处理“请求进来，立刻返回 usage”的场景。

代码里还存在一条平行主线：

- Midjourney
- Suno
- 视频类任务
- 订阅重置
- 渠道自动测试
- 模型同步
- 凭证刷新

也就是说，这个系统不只是请求网关，还是一个带周期性作业的长期运行平台。

## 两类后台行为

从 `main.go` 看，后台作业可以粗分成两类。

### 1. 运行时维护作业

主要包括：

- `model.SyncChannelCache()`
- `model.SyncOptions()`
- `model.UpdateQuotaData()`
- `controller.AutomaticallyTestChannels()`
- `controller.StartChannelUpstreamModelUpdateTask()`
- `service.StartCodexCredentialAutoRefreshTask()`
- `service.StartSubscriptionQuotaResetTask()`

这类任务更偏“控制面维护”，目标是让：

- 渠道缓存保持可用
- 配置支持热更新
- 数据看板持续刷新
- 上游模型列表和渠道状态自动维护
- 订阅周期自动重置

### 2. 异步任务轮询作业

这类任务更偏“数据面后处理”。

典型入口有：

- `controller.UpdateTaskBulk()`
- `service.TaskPollingLoop()`
- `controller.UpdateMidjourneyTaskBulk()`

它们负责追踪那些不会在一次 HTTP 响应里结束的任务。

## 为什么异步任务是独立子系统

同步 Relay 的特点是：

- 请求进来
- 调上游
- 同一条链路内返回结果
- 顺带结算

而异步任务不是这样。

以 Suno、视频任务为例，提交阶段通常只会拿到：

- 上游任务 ID
- 初始状态
- 需要后续轮询的上下文

所以系统必须把任务落库，再在未来某个轮询周期里继续推进。

## 核心抽象：`TaskPollingAdaptor`

`service/task_polling.go` 定义了任务轮询所需的最小接口：

- `Init`
- `FetchTask`
- `ParseTaskResult`
- `AdjustBillingOnComplete`

同时，`main.go` 通过 `service.GetTaskAdaptorFunc = relay.GetTaskAdaptor` 做依赖注入，显式打破 `service -> relay` 的循环依赖。

这个点很关键，说明作者已经把“异步任务轮询”当成独立运行时模型来处理，而不是硬塞进同步 Relay。

## 异步任务生命周期

可以把它理解为下面这条链路：

1. 用户提交任务型请求
2. 系统选路并调用上游
3. 写入本地 `tasks` 或 `midjourneys` 记录
4. 返回任务号给客户端
5. 后台轮询器按平台和渠道聚合查询
6. 状态推进到成功 / 失败 / 超时
7. 根据结果执行退款或差额结算

相比同步请求，这里新增了两个系统职责：

- 状态持久化
- 跨请求生命周期的计费修正

## 超时、CAS 与退款

`service/task_polling.go` 里有几个很值得注意的实现细节。

### 超时清扫

`sweepTimedOutTasks()` 会先于主轮询执行，按超时时间清理未完成任务。

这说明任务系统不是“无限等上游”，而是有平台自己的超时策略。

### CAS 更新

代码里使用 `UpdateWithStatus(oldStatus)` 这种按旧状态 compare-and-set 的方式推进任务状态。

目的很明确：

- 防止多个轮询分支互相覆盖
- 防止正常轮询与超时清扫相互踩状态

这说明任务子系统已经考虑了并发一致性，而不是简单的最后写入覆盖。

### 失败退款

任务失败或超时后，会触发 `RefundTaskQuota()`。

因此任务计费不是“提交时扣完就结束”，而是与状态推进绑定在一起。

## 为什么任务和计费要一起看

同步请求里，计费是一次请求内完成的。

异步任务里，计费被拆成了两个阶段：

1. 提交时预扣
2. 完成或失败时根据最终状态调整

这也是为什么 `TaskPollingAdaptor` 里会有 `AdjustBillingOnComplete()` 这种接口。

它把“任务状态推进”和“最终收费”绑定在了一起。

## 任务平台不是单一实现

从路由和控制器可以看到，项目并不是只有一种任务平台：

- `/mj/*` 对应 Midjourney
- `/suno/*` 对应 Suno
- `task_video.go` / `video_proxy*.go` 对应视频相关任务

所以任务系统本质上和 Relay 一样，也是“统一框架 + 多平台适配器”的设计。

## 这块对理解项目意味着什么

如果忽略这部分，容易把项目看成：

- 一个同步 API 转发网关

但把异步任务和后台作业加进来后，更准确的理解应该是：

```text
在线请求网关
  + 异步任务编排器
  + 周期性运维作业系统
  + 带状态持久化的计费后处理
```

这也是项目平台化程度很高的另一个证据。

## 为什么任务必须落库，而不是纯转发

如果系统只是纯 API 转发，那么它最多能处理：

- 请求进来
- 把请求转给上游
- 把上游响应原样回给客户端

这对同步聊天、embedding、一次性图片生成还说得过去，但对任务型接口并不够。

任务型接口的核心特征是：

- 首次提交通常只返回一个任务号
- 最终结果不会在本次 HTTP 请求里产生
- 后续还要继续查询状态、取结果、结算或退款

因此，这个系统必须自己持有一份本地任务状态，而不是只做一次透传。

### 纯转发做不到的几件事

#### 1. 提供平台自己的任务查询接口

系统不仅提交任务，还提供：

- 用户自己的任务列表
- 管理员的全站任务列表
- 按公开 `task_id` 查询任务详情

这些能力都依赖本地 `tasks` 表，而不是每次都直接回源上游。

#### 2. 在原请求结束后继续轮询

任务提交成功以后，HTTP 请求已经结束，但后台还要继续：

- 扫描未完成任务
- 按平台和渠道聚合
- 调用各 `TaskAdaptor.FetchTask(...)`
- 推进状态到成功、失败或超时

这意味着系统要跨请求保存任务上下文。

#### 3. 做异步结算、退款和差额补扣

任务提交时通常是预扣费，完成后才知道最终状态，某些平台甚至要按最终 token 或真实参数重算。

所以系统必须保存：

- 任务属于哪个用户
- 任务走哪个渠道
- 任务当时的计费上下文
- 任务最终状态

没有落库，进程重启后这些上下文都会丢失，异步账单就无法可靠恢复。

#### 4. 支持结果代理和权限校验

像视频内容代理这类场景，系统在用户下载结果前还要做：

- 判断任务是否属于当前用户
- 判断任务是否已经成功
- 找回原始渠道
- 确定该用 `upstream_task_id` 还是已保存的 `result_url`

这也要求任务信息长期保存在本地。

#### 5. 支持基于历史任务的 remix / continuation

某些二次生成不是独立任务，而是“基于一条旧任务继续做”。系统会从原任务中恢复：

- 原始模型名
- 原渠道
- 计费倍率
- 部分请求参数

这决定了 task 不是一次性的转发副产品，而是后续操作的业务上下文。

## `Task` 在库中被哪些场景读取

从当前实现看，`tasks` 记录至少被下面几类路径读取。

### 1. 用户与管理员查询

读取用途：

- 用户查看自己的任务列表
- 管理员查看全站任务列表
- 用户按 `task_id` 查询单条任务

这部分主要读取：

- `task_id`
- `status`
- `progress`
- `fail_reason`
- `submit_time`
- `finish_time`
- `data`
- `result_url`

### 2. 后台轮询推进状态

后台轮询会读取未完成任务，再按平台和渠道分组调用上游查询状态。

这部分主要读取：

- `platform`
- `channel_id`
- `action`
- `private_data.upstream_task_id`
- `private_data.key`

轮询后又会回写：

- `status`
- `progress`
- `start_time`
- `finish_time`
- `fail_reason`
- `data`
- `private_data.result_url`

### 3. 结果代理

视频代理接口不会盲目访问上游，而是先查 task 再决定如何取结果。

这部分主要读取：

- `user_id`
- `status`
- `channel_id`
- `private_data.upstream_task_id`
- `private_data.result_url`
- `private_data.key`

### 4. 异步计费与退款

任务完成或失败后，系统会基于 task 中保存的快照做结算或退款。

这部分主要读取：

- `quota`
- `group`
- `private_data.billing_source`
- `private_data.subscription_id`
- `private_data.token_id`
- `private_data.billing_context`

### 5. remix / continuation

基于原任务继续提交时，系统会先查原 task，再恢复它的上下文。

这部分主要读取：

- `channel_id`
- `properties.origin_model_name`
- `properties.upstream_model_name`
- `private_data.billing_context`
- `data`

## `Task` 表字段用途拆解

更细的字段说明可以看：

- [database-schema-summary.md](/Users/gaorx/Works/my/new-api/research/database-schema-summary.md)

这里先从运行时职责角度，把字段分成四组。

### 1. 给用户和管理端看的字段

- `task_id`
- `status`
- `progress`
- `fail_reason`
- `submit_time`
- `start_time`
- `finish_time`
- `data`

### 2. 给轮询器和 provider 适配器用的字段

- `platform`
- `channel_id`
- `action`
- `private_data.upstream_task_id`
- `private_data.key`

### 3. 给结果代理和内容访问用的字段

- `private_data.result_url`
- `private_data.upstream_task_id`
- `channel_id`

### 4. 给异步计费和退款用的字段

- `quota`
- `group`
- `private_data.billing_source`
- `private_data.subscription_id`
- `private_data.token_id`
- `private_data.billing_context`

## `Task` 生命周期时序表

下面这张表把“什么时候写入，后面谁来读”串在一起。

| 阶段 | 关键动作 | 主要写入/更新字段 | 后续读取方 |
|---|---|---|---|
| 提交前准备 | 在 `RelayInfo` 中确定 `Action`、模型名、公开任务 ID、计费上下文 | 尚未落库 | 任务创建阶段 |
| 提交成功 | 初始化并插入 `Task` 记录 | `task_id`、`user_id`、`group`、`channel_id`、`platform`、`quota`、`action`、`status=NOT_START`、`progress=0%`、`data` | 所有后续流程 |
| 绑定上游任务 | 保存对外任务号与上游真实任务号之间的映射 | `private_data.upstream_task_id`、`private_data.billing_source`、`private_data.subscription_id`、`private_data.token_id`、`private_data.billing_context` | 轮询、代理、退款、补扣 |
| 用户查详情/列表 | 不写，只读本地状态 | 无 | 用户端、管理端 |
| 后台轮询 | 读取未完成任务，调用上游查询状态 | 无 | 轮询器 |
| 轮询更新中 | 根据上游结果推进状态 | `status`、`progress`、`start_time`、`data` | 后续查询、最终结算 |
| 轮询成功 | 标记完成并确定最终结果地址 | `status=SUCCESS`、`progress=100%`、`finish_time`、`private_data.result_url` | 查询接口、视频代理、账单结算 |
| 轮询失败 | 标记失败并记录失败原因 | `status=FAILURE`、`progress=100%`、`finish_time`、`fail_reason` | 查询接口、退款逻辑 |
| 异步结算 | 根据最终状态和实际参数重算或退款 | 可能更新 `quota`，并据 `billing_context` 执行补扣/退款 | 账单与日志 |
| 结果代理 | 校验权限并代理视频/结果内容 | 一般不写，只读 `status`、`channel_id`、`upstream_task_id`、`result_url` | 最终内容访问 |
| remix / continuation | 从旧任务恢复上下文生成新任务 | 不改旧任务，只读取旧记录 | 新任务提交流程 |

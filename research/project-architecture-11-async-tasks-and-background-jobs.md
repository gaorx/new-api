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

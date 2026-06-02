# 05 - 渠道分发与选路机制

## 为什么分发机制是系统中枢

`new-api` 不是把请求固定转发给某一个上游，而是要在“用户、令牌、分组、模型、渠道能力、优先级、亲和性、重试策略”之间做决策。

## 基础分发

`middleware/Distribute()` 会从请求中提取模型名，并结合：

- Token 限制
- 当前使用分组
- 用户可用分组
- 请求路径类型
- 指定 channel id
- 渠道亲和性缓存

最后选出一个 Channel。

## Ability 驱动选路

真正的选路主要依赖 `Ability`：

- 按 `group + model` 找到候选渠道
- 先按 `priority` 分层
- 同优先级内按 `weight` 随机

因此：

- `priority` 更像“优先级层级”
- `weight` 更像“层内流量权重”

### `Ability` 到底是什么

这里的 `Ability` 不是“权限声明”，而是**渠道能力展开表**。

一条 `Ability` 记录的核心含义是：

- 某个 `group`
- 可以访问某个 `model`
- 对应某个 `channel_id`
- 当前是否启用
- 该映射的优先级和权重是多少

也就是说，它更像：

```text
group + model -> channel
```

的候选路由索引，而不是“请求上下文本身的来源表”。

请求里的 `model` 来自客户端，请求使用的 `group` 来自用户 / token / 分发上下文；`Ability` 负责回答的是：

```text
这个 group 请求这个 model 时，当前有哪些 channel 可以用
```

### `Ability` 如何生成

`Ability` 不是管理员一条条手工维护的主数据，而通常是由 `Channel` 的：

- `Group`
- `Models`
- `Priority`
- `Weight`
- `Status`

展开生成。

例如某个 channel 配置了：

- `group = default,vip`
- `models = gpt-4o,gpt-4.1`

那么系统会展开出 4 条 `Ability` 记录。

因此在数据建模上：

- `Channel` 更像“原始配置对象”
- `Ability` 更像“为了高效按 group/model 选路而展开出的索引层”

### 一个 `group + model` 可以匹配多个 channel

这也是整套路由的关键。

同一个 `group + model` 往往会命中多条 `Ability`，分别指向不同的 channel。系统并不是“查到一条就结束”，而是：

1. 先找出全部候选 channel
2. 先只看最高 `priority` 那一层
3. 在这一层内按 `weight` 做加权选择
4. 如果失败重试，再降到下一档 `priority`

因此更准确地说：

- `priority` 控制主备层级
- `weight` 控制同层流量分配

### 一个具体例子

假设同一个 `group=vip`、`model=gpt-4o` 下有 4 个候选 channel：

- `A`: `priority=10`, `weight=80`
- `B`: `priority=10`, `weight=20`
- `C`: `priority=5`, `weight=100`
- `D`: `priority=1`, `weight=100`

那么路由行为通常是：

1. 首次尝试时，只看最高优先级 `10` 这一层，也就是 `A/B`
2. 在 `A/B` 内按 `80:20` 做加权选择
3. 如果这一层失败并进入重试，才可能降到 `C`
4. 再继续失败，才可能降到 `D`

这说明当前实现不是“所有命中 channel 一起纯权重随机”，而是“优先级分层 + 层内加权”。

### 内存索引视角下的 `Ability`

项目在开启 `MEMORY_CACHE_ENABLED` 时，不会每次都直接查 `abilities` 表。

它会把渠道和能力关系预展开到进程内存里，核心结构大致是：

- `channelsIDM`: `channel_id -> channel`
- `group2model2channels`: `group -> model -> []channel_id`

所以运行时常见路径更接近：

```text
request
  -> 得到 group / model
  -> 命中内存索引 group2model2channels
  -> 得到候选 channel 列表
  -> 按 priority / weight 选一个
```

从架构语义上看，`abilities` 是数据库中的能力索引表，而 `group2model2channels` 是它在进程内的热路径投影。

## Auto Group

如果 Token 使用 `auto` 分组，则会结合：

- 用户可用分组
- `setting.AutoGroups`
- `CrossGroupRetry`

形成“跨资源池分组重试”的策略。

这是项目里很有特点的一层抽象，说明它不只是单组静态路由，而是支持分组级故障切换。

## Channel Affinity

`service/channel_affinity.go` 又加了一层“亲和性”：

- 可以按规则缓存某类请求对应的渠道
- 适合缓存命中、会话一致性、特殊供应商行为兼容等场景

这让渠道选择从“纯随机/优先级”进一步演化成“带状态记忆的路由”。

## 分发机制和 Relay 的关系

从系统视角看：

1. `Distribute` 负责“先选一个可能合适的渠道”
2. `controller/relay.go` 负责“把请求按该渠道送出去”
3. 如果调用失败，Relay 再根据重试逻辑继续切换渠道

因此，分发机制不是单独存在的，它是 Relay 运行时的前半段。

## 渠道失败、自动禁用与熔断传播

### 自动禁用针对的是 `channel`，不是独立的 `ability`

当某次调用失败时，系统会根据错误类型、状态码和关键字规则决定是否触发自动禁用。

但这里被真正“封禁”的主体通常不是某一条单独 `Ability`，而是整个 `Channel`：

- `channels.status` 会被更新为 `AutoDisabled`
- 当前节点内存中的 channel 缓存会立即更新
- 该 channel 对应的 `abilities.enabled` 会被同步改成 `false`

所以实际效果是：

- 选路层面看起来像“某些 ability 消失了”
- 但根因通常是底层 channel 被整体摘掉了

### 多 key channel 还有更细粒度的失效状态

如果一个 channel 是多 key 模式，那么系统还支持“只禁用某个 key，而不是整条 channel 直接失效”。

这部分状态保存在 `ChannelInfo` 中，例如：

- `MultiKeyStatusList`
- `MultiKeyDisabledReason`
- `MultiKeyDisabledTime`

只有当所有 key 都不可用时，整条 channel 才会进入 `AutoDisabled`。

这说明当前系统的熔断粒度并不完全统一：

- 单 key channel：通常直接 channel 级禁用
- 多 key channel：先 key 级禁用，必要时升级到 channel 级禁用

### 熔断传播顺序：先本机内存，再数据库，最后其他节点同步

当前实现不是“先写数据库，再等本机刷新”，而是：

1. 触发熔断的节点先更新自己进程内的 channel cache
2. 再把 `channels.status` 和原因写入数据库
3. 再更新对应 `abilities.enabled`
4. 其他节点依靠定时同步把数据库最新状态刷进自己的内存

因此它是一个：

```text
本机立即生效
其他节点最终一致
```

的传播模型。

### 这一设计的收益与风险

收益是：

- 当前出错节点能立刻把坏 channel 摘掉
- 状态可持久化
- 其他节点最终会收敛到一致状态

但它也有明显风险：

- 单个节点的局部网络抖动，可能被放大为全局 channel 故障
- 其他节点即使本来访问正常，也会在下一个同步周期后一起停用该 channel
- 在多节点环境中，这是一种“单点观测触发全局状态变更”的架构取舍

因此更精确地说，当前的自动熔断不是分布式投票式熔断，而是：

```text
单节点判定
数据库落库
多节点轮询传播
```

的最终一致机制。

### 熔断后如何恢复：以自动测活为主，必要时也可手工恢复

channel 被自动熔断后，并不是只能永久停留在 `AutoDisabled`。

当前项目已经提供了自动恢复链路，但它依赖两类开关：

- 是否开启自动渠道测试
- 是否开启自动重新启用

恢复流程大致是：

1. 某个 channel 因失败被标记为 `AutoDisabled`
2. `master` 节点后台定时执行全量渠道测试
3. 测试到这个 channel 时，如果本轮测试成功
4. 并且启用了自动重新启用逻辑
5. 系统会把该 channel 恢复为 `Enabled`

因此从行为上看，它更像：

```text
自动禁用 -> 定时测活 -> 测试成功后自动恢复
```

而不是“失败后永久停用，必须人工处理”。

### 自动恢复只针对 `AutoDisabled`

这里要特别区分两种禁用来源：

- `AutoDisabled`
- `ManuallyDisabled`

自动测活恢复机制只会尝试恢复 `AutoDisabled` 的 channel。  
如果一个 channel 是被管理员手工禁用的，那么这套自动恢复不会把它重新拉起。

这说明当前系统对“自动熔断”和“人工运维禁用”做了明确区分：

- 自动熔断：允许自动恢复
- 手工禁用：尊重人工决策，不自动恢复

### 自动恢复默认不是强制开启

自动恢复能力虽然存在，但它并不是无条件生效。

影响恢复行为的关键点包括：

1. 自动渠道测试是否开启
2. 自动渠道测试的周期是多少
3. `AutomaticEnableChannelEnabled` 是否开启

其中自动测试周期的默认值是 10 分钟，但默认并不一定启用；如果配置了相应监控设置或环境变量，系统才会按周期执行。

因此更准确地说：

- 项目支持自动恢复
- 自动恢复依赖 `master` 节点后台定时测活
- 是否真正生效取决于运行时配置

### 多节点下的恢复传播与熔断传播类似

恢复时的传播模型和熔断时基本一致：

1. 执行恢复的节点先更新本地内存
2. 再把数据库中的 channel 状态改回启用
3. 相关 `abilities.enabled` 同步恢复
4. 其他节点在下一轮同步时感知到该 channel 重新可用

所以恢复同样不是“全节点瞬时一致”，而是：

```text
当前节点立即恢复
其他节点最终一致恢复
```

### 这意味着什么

从系统设计上看，当前 channel 生命周期已经形成闭环：

- 失败时可以自动熔断
- 熔断后可以定时自动测活
- 测活成功后可以自动恢复

但这个闭环依然建立在“单节点执行 + 数据库传播 + 多节点轮询同步”的机制之上，因此它的优点和边界与前文熔断传播完全一致。

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

## `group` 不只是 quota 缩放规则

如果只看计费代码，容易把 `group` 理解成“决定 quota 倍率的标签”。  
但从整个系统实现看，`group` 的职责明显更宽，它更像一个统一的业务分层抽象。

至少有下面几类作用：

1. 路由与资源池选择
   请求最终不是按“用户”直接选 channel，而是按 `usingGroup + model` 选候选 channel；`auto` 分组还支持在多个真实 group 之间切换与重试。

2. 权限与可用范围控制
   用户所属 group 会影响“他还能访问哪些 group”。也就是说，group 不只是计费标签，还是可访问资源池范围的一部分。

3. 限流策略分层
   模型请求限流可以按 group 覆盖默认阈值，因此不同 group 可以天然拥有不同速率上限。

4. 充值/支付定价
   用户充值金额也可以按 group 套不同倍率，而不只是调用模型时的 quota 结算才看 group。

5. 订阅/套餐升级
   某些订阅套餐会直接把用户升级到另一个 group，因此 group 同时承担“用户档位/套餐层级”的语义。

所以更准确地说，`group` 在这个项目里同时承载：

- 资源池路由标签
- 权限分层标签
- 定价标签
- 限流分层标签
- 套餐/订阅层级标签

`quota` 缩放只是其中一项，不是它唯一的存在理由。

## `group` 本身没有独立主表

这个项目当前并不存在一个专门的 `groups` 主表。

更准确地说，group 的“定义来源”是两层：

1. `options` 表中的若干配置项
2. 各业务表中对 group 名称的引用字段

其中系统当前“有哪些 group”最核心的来源并不是 `users.group`、`tokens.group` 或 `channels.group`，而是 `options` 里的 `GroupRatio` 配置项。

这也是为什么 `GetGroups()` 并不是去查某张 `groups` 表，而是直接遍历 `GroupRatio` 的 key 来返回分组名。

业务表中的这些字段：

- `users.group`
- `tokens.group`
- `channels.group`
- `abilities.group`

更像是在“引用某个已存在的 group 名”，而不是定义 group 本身。

## `user.group`、`token.group` 与请求使用分组

这里有一个很容易误解的点：请求运行时使用的 `group`，不一定等于用户默认分组。

当前实现里：

- `users.group` 表示用户默认所属分组
- `tokens.group` 表示该 token 希望请求默认落到哪个分组
- 真正选路使用的是请求上下文里的 `usingGroup`

`TokenAuth()` 的行为大致是：

1. 先取 `user.group`
2. 如果 `token.group` 非空，则检查这个分组是否属于该用户可用分组
3. 若校验通过，用 `token.group` 覆盖本次请求的 `usingGroup`
4. 后续 `Distribute()` 和 Relay 重试都使用这个 `usingGroup`

所以更准确地说：

- 一个用户有一个默认 group
- 一个 token 也可以单独绑定一个 group
- 单个 token 只能配置一个 group 值
- 但同一个用户可以持有多个 token，分别把请求导向不同 group

这也是为什么“请求使用哪个 group”必须放在运行时上下文里，而不是只看 `users.group`。

## `group` 和 `channel` 的关系是多对多

当前系统里，`channel` 与 `group` 不是严格一对一外键关系，而是多对多关系，只是建模方式比较轻量。

`channels.group` 本身就是一个逗号分隔字符串，例如：

- `default`
- `default,vip`
- `vip,internal,batch`

这意味着：

- 一个 `group` 可以对应多个 `channel`
- 一个 `channel` 也可以同时服务多个 `group`

因此运行时真正要解决的问题不是：

```text
group -> channel
```

而是：

```text
group + model -> candidate channels
```

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

### `group x model` 到 `channels` 的具体匹配过程

如果把选路过程展开，运行时逻辑可以近似理解成：

```text
请求进入
  -> TokenAuth 确定 usingGroup
  -> Distribute 解析 model
  -> 尝试命中 affinity 里的 preferred channel
     -> 如果命中，还要校验这个 channel 是否仍然属于当前 group + model
  -> 若 affinity 未命中或校验失败
     -> 按 usingGroup + model 查询候选 channels
  -> 在候选 channels 中按 priority / weight 选一个
```

其中“按 `usingGroup + model` 查询候选 channels”又分两种实现路径。

### 有内存缓存时：走 `group2model2channels`

开启 `MEMORY_CACHE_ENABLED` 时，系统会在后台把启用中的 channel 预展开成进程内索引：

```text
group2model2channels[group][model] = []channel_id
```

构建过程本质上是：

1. 遍历所有启用中的 `channel`
2. 把 `channel.Group` 按逗号拆成多个 group
3. 把 `channel.Models` 按逗号拆成多个 model
4. 对每个 `group x model` 组合，把 `channel.Id` 追加进去
5. 再按 `priority` 对候选 `channel_id` 列表排序

所以某条 channel 若配置为：

- `group = default,vip`
- `models = gpt-4o,gpt-4.1`

它会在内存中同时出现在四个索引位置：

- `default x gpt-4o`
- `default x gpt-4.1`
- `vip x gpt-4o`
- `vip x gpt-4.1`

运行时查询时，`GetRandomSatisfiedChannel(group, model, retry)` 的匹配顺序是：

1. 先精确查 `group2model2channels[group][model]`
2. 若没有结果，再把 model 名做一次归一化
3. 再查 `group2model2channels[group][normalizedModel]`
4. 仍无结果则视为“该 group 下无可用 channel”

这里的模型归一化主要用于兼容某些带动态后缀的模型名，例如：

- `gpt-4-gizmo-*`
- `gpt-4o-gizmo-*`
- 部分 Gemini thinking budget 变体

### 无内存缓存时：走 `abilities` 表

关闭内存缓存时，并不是改成另一套业务规则，而是回退到数据库索引层。

这时系统会基于 `abilities` 表做查询：

```text
WHERE group = ? AND model = ? AND enabled = true
```

然后：

1. 先根据 `retry` 决定应该使用哪一档 `priority`
2. 再取该优先级下的全部 `Ability`
3. 用 `weight` 在这些 `Ability` 对应的 channel 里做加权随机
4. 最终拿到 `channel_id` 并取回 `Channel`

因此：

- 有缓存：查内存索引 `group2model2channels`
- 无缓存：查数据库索引 `abilities`

但两者的目标语义并没有变，仍然都是：

```text
group + model
  -> 候选 channels
  -> priority 分层
  -> 层内按 weight 选择
```

所以它们不是“完全不同机制”，而是：

- 同一套路由语义
- 两种不同的数据索引实现

### 为什么看起来像两套机制

之所以容易误以为它们完全不同，是因为两条路径读取的数据层不一样：

- 内存缓存路径主要从 `channels.group + channels.models` 预展开
- 数据库回退路径主要直接查 `abilities`

这意味着如果出现极端一致性问题，例如：

- `channel` 已更新
- 但 `abilities` 尚未正确重建
- 或某节点内存缓存还没同步到最新状态

那么有缓存路径和无缓存路径在短时间内可能表现不一致。

但从设计目标看，`abilities` 是数据库中的持久化路由索引，`group2model2channels` 是它的热路径内存投影；两者不是为了表达两套不同规则，而是为了在不同运行模式下承载同一套路由语义。

## 一条完整的请求选路链路

如果把从入口到最终选中 channel 的主路径串起来，可以概括为：

```text
/v1/... 请求
  -> TokenAuth()
     -> 校验 token
     -> 读取 user.group
     -> 若 token.group 非空且用户有权使用，则覆盖 usingGroup
     -> 写入 token model limit / cross-group retry / specific channel 等上下文
  -> ModelRequestRateLimit()
  -> Distribute()
     -> 从请求体中解析 model
     -> 检查 token 的 model limit
     -> 读取 usingGroup
     -> 若是 Playground，可再按请求体覆盖 group
     -> 尝试命中 channel affinity
        -> 并校验 preferred channel 是否仍满足当前 group + model
     -> 若未命中
        -> 若 usingGroup != auto
           -> 直接按 group + model 选 channel
        -> 若 usingGroup == auto
           -> 取用户可用 autoGroups
           -> 依次尝试每个真实 group
           -> 必要时按 cross-group retry 切到下一个 group
     -> 把选中的 channel 信息写入 context
  -> controller.Relay()
  -> relay/channel/* 上游适配器
```

这条链上：

- `Channel` 是原始配置对象
- `Ability` 是展开后的数据库索引对象
- `group2model2channels` 是运行时内存索引
- `Distribute()` 是“把请求翻译成具体路由结果”的执行点

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

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

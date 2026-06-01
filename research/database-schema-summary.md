# new-api 数据库表结构总结

## 说明

这份文档基于当前代码中的迁移入口 `model/main.go` 和各个 `model/*.go` 结构体整理，目标是总结：

- 系统实际迁移创建了哪些表
- 每张表的大致用途
- 每个字段名的用途

## 范围说明

1. 主来源是 `model/main.go` 里的 `DB.AutoMigrate(...)` 和 `LOG_DB.AutoMigrate(...)`。
2. 没有 `TableName()` 覆写的模型，表名按 GORM 默认命名规则推断为“snake_case + 复数”。
3. 有少数字段带 `gorm:"-"`，这类字段不落库，不计入表字段。
4. 有些字段是 JSON/TEXT，文档会补充其中常见内部结构。
5. `logs` 表可能位于主库，也可能位于独立日志库：
   - 如果 `LOG_SQL_DSN` 为空，则与主库共用同一张 `logs` 表
   - 如果 `LOG_SQL_DSN` 不为空，则日志表会迁移到独立日志库
6. “直接外键”与“隐式外键”的判定依据：
   - 直接外键：结构体 `gorm` tag、显式 `FOREIGN KEY / REFERENCES` DDL、迁移 SQL 中真正声明的数据库外键约束
   - 隐式外键：代码通过 `user_id`、`channel_id`、`plan_id`、`vendor_id`、`provider_id` 等字段在应用层自行维护的逻辑关联

---

## 表速览

每个表一行，先看“这张表是干什么的”。

| 表名 | 用途 |
|---|---|
| `users` | 平台用户主表 |
| `tokens` | 平台 API 令牌 |
| `passkey_credentials` | Passkey / WebAuthn 凭证 |
| `two_fas` | 用户 2FA 状态与 TOTP 信息 |
| `two_fa_backup_codes` | 2FA 备用码 |
| `custom_oauth_providers` | 自定义 OAuth/OIDC 提供商配置 |
| `user_oauth_bindings` | 用户与自定义 OAuth 提供商的绑定关系 |
| `options` | 动态系统配置中心 |
| `setups` | 系统初始化状态 |
| `checkins` | 用户签到记录 |
| `redemptions` | 兑换码/充值码 |
| `top_ups` | 充值订单 |
| `channels` | 上游渠道配置 |
| `abilities` | 渠道在分组+模型维度上的可服务能力索引 |
| `models` | 平台模型元数据 |
| `vendors` | 模型供应商元数据 |
| `prefill_groups` | 前端可复用预填分组 |
| `logs` | 消费日志/请求日志 |
| `quota_data` | 数据看板聚合数据 |
| `tasks` | 异步任务记录 |
| `midjourneys` | Midjourney 任务记录 |
| `perf_metrics` | 模型性能指标聚合 |
| `subscription_plans` | 订阅套餐定义 |
| `subscription_orders` | 订阅购买订单 |
| `user_subscriptions` | 用户订阅实例 |
| `subscription_pre_consume_records` | 订阅额度预扣记录 |

---

## 表总览

### 账户、认证与授权

| 表名 | 对应模型 | 用途 |
|---|---|---|
| `users` | `User` | 平台用户主表 |
| `tokens` | `Token` | 平台 API 令牌 |
| `passkey_credentials` | `PasskeyCredential` | Passkey / WebAuthn 凭证 |
| `two_fas` | `TwoFA` | 用户 2FA 状态与 TOTP 信息 |
| `two_fa_backup_codes` | `TwoFABackupCode` | 2FA 备用码 |
| `custom_oauth_providers` | `CustomOAuthProvider` | 自定义 OAuth/OIDC 提供商配置 |
| `user_oauth_bindings` | `UserOAuthBinding` | 用户与自定义 OAuth 提供商的绑定关系 |

### 配置、初始化与运营

| 表名 | 对应模型 | 用途 |
|---|---|---|
| `options` | `Option` | 动态系统配置中心 |
| `setups` | `Setup` | 系统初始化状态 |
| `checkins` | `Checkin` | 用户签到记录 |
| `redemptions` | `Redemption` | 兑换码/充值码 |
| `top_ups` | `TopUp` | 充值订单 |

### 渠道、模型与路由

| 表名 | 对应模型 | 用途 |
|---|---|---|
| `channels` | `Channel` | 上游渠道配置 |
| `abilities` | `Ability` | 渠道在分组+模型维度上的可服务能力索引 |
| `models` | `Model` | 平台模型元数据 |
| `vendors` | `Vendor` | 模型供应商元数据 |
| `prefill_groups` | `PrefillGroup` | 前端可复用预填分组 |

### 使用、任务与日志

| 表名 | 对应模型 | 用途 |
|---|---|---|
| `logs` | `Log` | 消费日志/请求日志 |
| `quota_data` | `QuotaData` | 数据看板聚合数据 |
| `tasks` | `Task` | 异步任务记录 |
| `midjourneys` | `Midjourney` | Midjourney 任务记录 |
| `perf_metrics` | `PerfMetric` | 模型性能指标聚合 |

### 订阅与计费

| 表名 | 对应模型 | 用途 |
|---|---|---|
| `subscription_plans` | `SubscriptionPlan` | 订阅套餐定义 |
| `subscription_orders` | `SubscriptionOrder` | 订阅购买订单 |
| `user_subscriptions` | `UserSubscription` | 用户订阅实例 |
| `subscription_pre_consume_records` | `SubscriptionPreConsumeRecord` | 订阅额度预扣记录 |

---

## 关系设计总览

### 1. 直接外键（数据库约束）现状

基于本次对 `model/*.go`、`model/main.go` 以及项目内显式 SQL 的核对，当前项目**没有发现已声明的数据库层外键约束**：

- 没有搜到 `gorm:"foreignKey:..."`、`references:`、`constraint:` 等 GORM 外键声明
- 没有搜到手写的 `FOREIGN KEY` / `REFERENCES` 建表或迁移 SQL
- `AutoMigrate(...)` 主要负责建表、补列、建索引，不承担关系约束的统一声明

这意味着当前 DB 设计是一个**以应用层维护关系完整性为主**的方案。优点是兼容 SQLite / MySQL / PostgreSQL 更简单；代价是孤儿数据、级联删除、跨库一致性都要靠业务代码保证。

补充说明：

- `logs` 表可能位于独立日志库；即使未来希望对 `logs.user_id`、`logs.token_id`、`logs.channel_id` 加硬外键，也会受到“跨库不可直接外键约束”的天然限制。
- `subscription_plans` 在 SQLite 下是手写兼容 DDL 创建，也没有声明外键。

### 2. 隐式外键清单（指向实际表）

这些关系在业务上真实存在，但数据库没有硬约束。

| 来源字段 | 目标字段 | 关系类型 | 说明 |
|---|---|---|---|
| `users.inviter_id` | `users.id` | 隐式自关联 | 用户邀请关系，记录邀请人 |
| `tokens.user_id` | `users.id` | 隐式外键 | 一个用户可以持有多个平台 Token |
| `passkey_credentials.user_id` | `users.id` | 隐式外键 | 一个用户对应一个 Passkey 凭证记录 |
| `two_fas.user_id` | `users.id` | 隐式外键 | 一个用户对应一条 2FA 主记录 |
| `two_fa_backup_codes.user_id` | `users.id` | 隐式外键 | 一个用户对应多条 2FA 备用码 |
| `user_oauth_bindings.user_id` | `users.id` | 隐式外键 | OAuth 绑定归属某个用户 |
| `user_oauth_bindings.provider_id` | `custom_oauth_providers.id` | 隐式外键 | OAuth 绑定归属某个自定义提供商 |
| `checkins.user_id` | `users.id` | 隐式外键 | 签到记录归属用户 |
| `redemptions.user_id` | `users.id` | 隐式外键 | 兑换码创建者或归属用户 |
| `redemptions.used_user_id` | `users.id` | 隐式外键 | 实际使用兑换码的用户 |
| `top_ups.user_id` | `users.id` | 隐式外键 | 充值订单归属用户 |
| `abilities.channel_id` | `channels.id` | 隐式外键 | 能力索引从渠道表展开而来 |
| `models.vendor_id` | `vendors.id` | 隐式外键 | 模型元数据归属供应商 |
| `logs.user_id` | `users.id` | 隐式外键 | 日志归属用户 |
| `logs.channel_id` | `channels.id` | 隐式外键 | 日志关联调用渠道 |
| `logs.token_id` | `tokens.id` | 隐式外键 | 日志关联调用 Token |
| `quota_data.user_id` | `users.id` | 隐式外键 | 看板聚合数据归属用户 |
| `tasks.user_id` | `users.id` | 隐式外键 | 异步任务归属用户 |
| `tasks.channel_id` | `channels.id` | 隐式外键 | 异步任务由某个渠道提交 |
| `tasks.private_data.subscription_id` | `user_subscriptions.id` | 隐式外键（JSON 内） | 任务若走订阅计费，会把订阅实例 ID 存进 JSON |
| `tasks.private_data.token_id` | `tokens.id` | 隐式外键（JSON 内） | 任务若涉及令牌计费退款，会把 Token ID 存进 JSON |
| `midjourneys.user_id` | `users.id` | 隐式外键 | Midjourney 任务归属用户 |
| `midjourneys.channel_id` | `channels.id` | 隐式外键 | Midjourney 任务关联渠道 |
| `subscription_orders.user_id` | `users.id` | 隐式外键 | 订阅订单归属用户 |
| `subscription_orders.plan_id` | `subscription_plans.id` | 隐式外键 | 订阅订单购买的是某个套餐 |
| `user_subscriptions.user_id` | `users.id` | 隐式外键 | 订阅实例归属用户 |
| `user_subscriptions.plan_id` | `subscription_plans.id` | 隐式外键 | 订阅实例来源于某个套餐模板 |
| `subscription_pre_consume_records.user_id` | `users.id` | 隐式外键 | 预扣记录归属用户 |
| `subscription_pre_consume_records.user_subscription_id` | `user_subscriptions.id` | 隐式外键 | 预扣记录绑定某条用户订阅实例 |

### 3. 隐式业务关联（目标不是独立主表）

这一类字段不是传统外键，但在业务上共享同一套“字符串域”或“快照域”，也属于理解 DB 设计时必须关注的关系。

| 字段 | 共享域/关联对象 | 说明 |
|---|---|---|
| `users.group` | 用户组字符串域 | 用户所属逻辑分组 |
| `tokens.group` | 用户组字符串域 | Token 请求默认使用的逻辑分组 |
| `channels.group` | 用户组字符串域 | 渠道可服务的分组集合，逗号分隔 |
| `abilities.group` | 用户组字符串域 | 从 `channels.group` 展开得到的单个分组索引 |
| `logs.group` | 用户组字符串域 | 请求发生时使用的分组快照 |
| `tasks.group` | 用户组字符串域 | 异步任务提交时的分组快照 |
| `perf_metrics.group` | 用户组字符串域 | 性能指标聚合维度之一 |
| `user_subscriptions.upgrade_group` | 用户组字符串域 | 订阅生效后要把用户提升到的组 |
| `user_subscriptions.prev_user_group` | 用户组字符串域 | 订阅升级前用户原始分组快照 |
| `channels.models` | 模型名字符串域 | 渠道支持的模型集合，逗号分隔 |
| `abilities.model` | 模型名字符串域 | 从 `channels.models` 展开的单模型索引 |
| `models.model_name` | 模型名字符串域 | 平台模型主数据 |
| `logs.model_name` | 模型名字符串域 | 请求发生时的模型名快照 |
| `quota_data.model_name` | 模型名字符串域 | 看板聚合维度之一 |
| `perf_metrics.model_name` | 模型名字符串域 | 性能指标聚合维度之一 |
| `tasks.properties.upstream_model_name` | 上游模型名字符串域 | 任务提交时的上游模型快照 |
| `tasks.properties.origin_model_name` | 平台模型名字符串域 | 任务提交时的平台模型快照 |
| `logs.username` | `users.username` 快照 | 为了查询/展示方便保存在日志里，不是硬外键 |
| `quota_data.username` | `users.username` 快照 | 聚合时写入用户名快照 |
| `logs.token_name` | `tokens.name` 快照 | 令牌名快照，便于审计 |
| `logs.channel_name` | `channels.name` 只读关联结果 | 字段本身不落库，由查询 join/映射得到 |

### 4. 关系维护方式总结

从代码实现看，当前关系完整性主要通过下面几种手段维护：

- 先查主表再写从表，例如创建 `TwoFA` 时会先确认 `users` 中存在对应用户
- 在事务里同时更新关联表，例如签到时同时写 `checkins` 并增加 `users.quota`
- 通过唯一索引约束部分业务关系，例如 `checkins(user_id, checkin_date)`、`user_oauth_bindings` 两组联合唯一索引
- 通过级联业务代码而不是数据库级联删除，例如删除自定义 OAuth 提供商前先删 `user_oauth_bindings`

因此，读这套 DB 时要把“字段名像外键，但数据库不拦截脏数据”作为一个基本前提。

---

## 1. `users`

用途：平台用户主表，保存账户身份、额度、分组、第三方绑定、个人设置等信息。

| 字段名 | 用途 |
|---|---|
| `id` | 用户主键 |
| `username` | 登录用户名，唯一 |
| `password` | 登录密码哈希 |
| `display_name` | 显示名称 |
| `role` | 用户角色，如普通用户、管理员、Root |
| `status` | 用户状态，如启用、禁用 |
| `email` | 邮箱 |
| `github_id` | GitHub 绑定 ID |
| `discord_id` | Discord 绑定 ID |
| `oidc_id` | OIDC 绑定 ID |
| `wechat_id` | 微信绑定 ID |
| `telegram_id` | Telegram 绑定 ID |
| `access_token` | 管理端访问令牌 |
| `quota` | 用户钱包/主额度余额 |
| `used_quota` | 用户已消耗额度 |
| `request_count` | 请求次数累计 |
| `group` | 用户所属分组 |
| `aff_code` | 邀请码 |
| `aff_count` | 邀请人数或邀请次数统计 |
| `aff_quota` | 邀请奖励的剩余额度 |
| `aff_history` | 邀请累计历史额度 |
| `inviter_id` | 邀请人用户 ID |
| `deleted_at` | 软删除时间 |
| `linux_do_id` | LinuxDO 绑定 ID |
| `setting` | 用户设置 JSON 文本 |
| `remark` | 备注 |
| `stripe_customer` | Stripe 客户 ID |
| `created_at` | 创建时间 |
| `last_login_at` | 最后登录时间 |

---

## 2. `tokens`

用途：平台发给下游客户端使用的 API Token，不是上游厂商 Key。

| 字段名 | 用途 |
|---|---|
| `id` | 令牌主键 |
| `user_id` | 所属用户 ID |
| `key` | 令牌字符串 |
| `status` | 令牌状态 |
| `name` | 令牌名称 |
| `created_time` | 创建时间 |
| `accessed_time` | 最近访问时间 |
| `expired_time` | 过期时间，`-1` 表示永不过期 |
| `remain_quota` | 剩余额度 |
| `unlimited_quota` | 是否无限额度 |
| `model_limits_enabled` | 是否启用模型限制 |
| `model_limits` | 允许访问的模型列表/映射，文本存储 |
| `allow_ips` | 允许访问的 IP 白名单 |
| `used_quota` | 已使用额度 |
| `group` | 令牌使用的逻辑分组 |
| `cross_group_retry` | 是否允许 auto 分组跨组重试 |
| `deleted_at` | 软删除时间 |

---

## 3. `passkey_credentials`

用途：保存用户 Passkey / WebAuthn 凭证。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `user_id` | 所属用户 ID |
| `credential_id` | 凭证 ID，通常为 base64 编码 |
| `public_key` | 公钥数据 |
| `attestation_type` | 证明类型 |
| `aaguid` | Authenticator AAGUID |
| `sign_count` | 签名计数器 |
| `clone_warning` | 是否出现克隆告警 |
| `user_present` | 是否验证了用户在场 |
| `user_verified` | 是否完成用户验证 |
| `backup_eligible` | 是否支持备份 |
| `backup_state` | 当前是否处于备份状态 |
| `transports` | 传输方式列表，文本存储 |
| `attachment` | 附件类型，如平台型/漫游型 |
| `last_used_at` | 上次使用时间 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |
| `deleted_at` | 软删除时间 |

---

## 4. `two_fas`

用途：保存用户 2FA/TOTP 的启用状态和安全控制信息。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `user_id` | 所属用户 ID，唯一 |
| `secret` | TOTP 密钥 |
| `is_enabled` | 是否启用 2FA |
| `failed_attempts` | 连续失败次数 |
| `locked_until` | 锁定截止时间 |
| `last_used_at` | 最近一次成功使用时间 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |
| `deleted_at` | 软删除时间 |

---

## 5. `two_fa_backup_codes`

用途：保存用户的 2FA 备用恢复码。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `user_id` | 所属用户 ID |
| `code_hash` | 备用码哈希 |
| `is_used` | 是否已使用 |
| `used_at` | 使用时间 |
| `created_at` | 创建时间 |
| `deleted_at` | 软删除时间 |

---

## 6. `custom_oauth_providers`

用途：保存站点自定义 OAuth / OIDC 提供商配置。

表名来源：显式 `TableName()`，固定为 `custom_oauth_providers`。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `name` | 提供商显示名称 |
| `slug` | 路由标识，唯一 |
| `icon` | 图标标识 |
| `enabled` | 是否启用 |
| `client_id` | OAuth Client ID |
| `client_secret` | OAuth Client Secret |
| `authorization_endpoint` | 授权地址 |
| `token_endpoint` | Token 交换地址 |
| `user_info_endpoint` | 用户信息接口地址 |
| `scopes` | 授权 scopes |
| `user_id_field` | 从用户信息响应中提取用户 ID 的字段路径 |
| `username_field` | 提取用户名的字段路径 |
| `display_name_field` | 提取显示名的字段路径 |
| `email_field` | 提取邮箱的字段路径 |
| `well_known` | OIDC `.well-known` 地址 |
| `auth_style` | 认证方式配置 |
| `access_policy` | 用户信息访问策略 JSON |
| `access_denied_message` | 访问拒绝时的自定义提示 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

---

## 7. `user_oauth_bindings`

用途：保存用户与自定义 OAuth 提供商账号的绑定关系。

表名来源：显式 `TableName()`，固定为 `user_oauth_bindings`。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `user_id` | 平台用户 ID |
| `provider_id` | 自定义 OAuth 提供商 ID |
| `provider_user_id` | 上游 OAuth 提供商返回的用户唯一标识 |
| `created_at` | 创建时间 |

---

## 8. `options`

用途：运行时系统配置中心，保存动态配置项。

| 字段名 | 用途 |
|---|---|
| `key` | 配置键，主键 |
| `value` | 配置值，文本形式保存 |

---

## 9. `setups`

用途：记录系统是否已经完成初始化。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `version` | 初始化时的系统版本 |
| `initialized_at` | 初始化时间戳 |

---

## 10. `checkins`

用途：保存用户每日签到记录。

表名来源：显式 `TableName()`，固定为 `checkins`。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `user_id` | 用户 ID |
| `checkin_date` | 签到日期，格式 `YYYY-MM-DD` |
| `quota_awarded` | 本次签到奖励额度 |
| `created_at` | 创建时间 |

---

## 11. `redemptions`

用途：兑换码/充值码，供用户兑换额度。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `user_id` | 创建者或关联用户 ID |
| `key` | 兑换码字符串，唯一 |
| `status` | 状态 |
| `name` | 兑换码名称 |
| `quota` | 可兑换额度 |
| `created_time` | 创建时间 |
| `redeemed_time` | 被兑换时间 |
| `used_user_id` | 实际使用该兑换码的用户 ID |
| `deleted_at` | 软删除时间 |
| `expired_time` | 过期时间，`0` 表示不过期 |

说明：`count` 字段带 `gorm:"-:all"`，不落库。

---

## 12. `top_ups`

用途：用户充值订单。

表名按 GORM 默认规则推断为 `top_ups`。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `user_id` | 充值用户 ID |
| `amount` | 充值的额度数量 |
| `money` | 支付金额 |
| `trade_no` | 支付平台订单号，唯一 |
| `payment_method` | 支付方式 |
| `payment_provider` | 支付提供商 |
| `create_time` | 创建时间 |
| `complete_time` | 完成时间 |
| `status` | 订单状态 |

---

## 13. `channels`

用途：上游渠道配置主表，系统所有上游接入都从这里定义。

| 字段名 | 用途 |
|---|---|
| `id` | 渠道主键 |
| `type` | 渠道类型，如 OpenAI、Claude、Gemini、Azure 等 |
| `key` | 上游 API Key 或其集合 |
| `openai_organization` | OpenAI Organization |
| `test_model` | 渠道测试模型 |
| `status` | 渠道状态 |
| `name` | 渠道名称 |
| `weight` | 同优先级下的权重 |
| `created_time` | 创建时间 |
| `test_time` | 最近测试时间 |
| `response_time` | 响应时间统计 |
| `base_url` | 上游基础地址 |
| `other` | 其他补充信息 |
| `balance` | 渠道余额 |
| `balance_updated_time` | 余额更新时间 |
| `models` | 渠道支持的模型列表，逗号分隔 |
| `group` | 渠道所属分组列表 |
| `used_quota` | 渠道累计消耗额度 |
| `model_mapping` | 模型映射配置 |
| `status_code_mapping` | 状态码映射配置 |
| `priority` | 渠道优先级 |
| `auto_ban` | 是否自动封禁异常渠道 |
| `other_info` | 其他详细信息 |
| `tag` | 渠道标签 |
| `setting` | 渠道额外设置 |
| `param_override` | 请求参数覆盖配置 |
| `header_override` | 请求头覆盖配置 |
| `remark` | 备注 |
| `channel_info` | 多 Key 等运行时结构的 JSON |
| `settings` | 其他设置 JSON，常放 Azure 版本等非检索信息 |

### `channel_info` JSON 内部结构

| 键名 | 用途 |
|---|---|
| `is_multi_key` | 是否启用多 Key 模式 |
| `multi_key_size` | Key 数量 |
| `multi_key_status_list` | 每个 Key 的状态映射 |
| `multi_key_disabled_reason` | 每个 Key 的禁用原因 |
| `multi_key_disabled_time` | 每个 Key 的禁用时间 |
| `multi_key_polling_index` | 轮询模式下当前索引 |
| `multi_key_mode` | 多 Key 使用模式，如随机/轮询 |

说明：`keys` 字段带 `gorm:"-"`，不落库。

---

## 14. `abilities`

用途：渠道能力展开表，把 “渠道支持哪些模型、属于哪些分组” 预展开成索引。

| 字段名 | 用途 |
|---|---|
| `group` | 分组名，联合主键的一部分 |
| `model` | 模型名，联合主键的一部分 |
| `channel_id` | 渠道 ID，联合主键的一部分 |
| `enabled` | 该能力是否启用 |
| `priority` | 该能力的优先级 |
| `weight` | 同优先级内权重 |
| `tag` | 标签 |

---

## 15. `models`

用途：平台展示和管理用的模型元数据表，不等于上游原始模型列表。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `model_name` | 模型名称，唯一（配合软删除） |
| `description` | 模型描述 |
| `icon` | 图标 |
| `tags` | 模型标签 |
| `vendor_id` | 所属供应商 ID |
| `endpoints` | 端点配置 |
| `status` | 状态 |
| `sync_official` | 是否参与官方模型同步 |
| `created_time` | 创建时间 |
| `updated_time` | 更新时间 |
| `deleted_at` | 软删除时间 |
| `name_rule` | 模型名匹配规则 |

说明：`bound_channels`、`enable_groups`、`quota_types`、`matched_models`、`matched_count` 都不落库。

---

## 16. `vendors`

用途：模型供应商元数据，供 `models.vendor_id` 引用。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `name` | 供应商名称 |
| `description` | 描述 |
| `icon` | 图标 |
| `status` | 状态 |
| `created_time` | 创建时间 |
| `updated_time` | 更新时间 |
| `deleted_at` | 软删除时间 |

---

## 17. `prefill_groups`

用途：前端可复用的预填分组，如模型组、标签组、端点组。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `name` | 分组名称 |
| `type` | 分组类型，如 `model`、`tag`、`endpoint` |
| `items` | JSON 数组，保存分组条目 |
| `description` | 描述 |
| `created_time` | 创建时间 |
| `updated_time` | 更新时间 |
| `deleted_at` | 软删除时间 |

---

## 18. `logs`

用途：记录用户请求消费、令牌消耗、渠道调用等日志信息。

| 字段名 | 用途 |
|---|---|
| `id` | 日志主键 |
| `user_id` | 用户 ID |
| `created_at` | 创建时间 |
| `type` | 日志类型 |
| `content` | 日志内容 |
| `username` | 用户名快照 |
| `token_name` | 令牌名称快照 |
| `model_name` | 模型名 |
| `quota` | 本次消耗额度 |
| `prompt_tokens` | prompt token 数 |
| `completion_tokens` | completion token 数 |
| `use_time` | 耗时 |
| `is_stream` | 是否流式 |
| `channel` | 渠道 ID |
| `channel_name` | 渠道名称，只读查询字段 |
| `token_id` | 令牌 ID |
| `group` | 使用分组 |
| `ip` | 调用 IP |
| `request_id` | 平台请求 ID |
| `upstream_request_id` | 上游请求 ID |
| `other` | 额外 JSON/文本信息 |

说明：`channel_name` 带 `gorm:"->"`，主要作为只读查询字段使用。

---

## 19. `quota_data`

用途：为管理看板做按小时聚合的额度和调用统计。

表名来自代码中的显式 `DB.Table("quota_data")` 使用。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `user_id` | 用户 ID |
| `username` | 用户名快照 |
| `model_name` | 模型名 |
| `created_at` | 聚合时间桶，通常按小时截断 |
| `token_used` | 使用 token 总数 |
| `count` | 请求次数 |
| `quota` | 消耗额度总量 |

---

## 20. `tasks`

用途：通用异步任务表，用于视频、音乐、Suno 等长生命周期任务。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |
| `task_id` | 上游第三方任务 ID |
| `platform` | 任务平台类型 |
| `user_id` | 用户 ID |
| `group` | 分组，主要用于计费修正 |
| `channel_id` | 渠道 ID |
| `quota` | 该任务对应的额度 |
| `action` | 任务动作类型 |
| `status` | 任务状态 |
| `fail_reason` | 失败原因 |
| `submit_time` | 提交时间 |
| `start_time` | 开始时间 |
| `finish_time` | 完成时间 |
| `progress` | 进度 |
| `properties` | 任务基础属性 JSON |
| `private_data` | 内部私有 JSON，可能包含计费/上游信息 |
| `data` | 任务结果或原始响应 JSON |

### `properties` JSON 内部结构

| 键名 | 用途 |
|---|---|
| `input` | 用户输入内容 |
| `upstream_model_name` | 实际上游模型名 |
| `origin_model_name` | 平台原始模型名 |

### `private_data` JSON 内部结构

| 键名 | 用途 |
|---|---|
| `key` | 可能保存上游密钥等内部信息 |
| `upstream_task_id` | 上游真实任务 ID |
| `result_url` | 任务结果 URL |
| `billing_source` | 计费来源，如 wallet/subscription |
| `subscription_id` | 订阅 ID |
| `token_id` | 平台令牌 ID |
| `billing_context` | 计费上下文快照 |

### `billing_context` JSON 内部结构

| 键名 | 用途 |
|---|---|
| `model_price` | 模型单价 |
| `group_ratio` | 分组倍率 |
| `model_ratio` | 模型倍率 |
| `other_ratios` | 附加倍率，如时长、分辨率 |
| `origin_model_name` | 模型名 |
| `per_call_billing` | 是否按次计费 |

说明：`username` 字段带 `gorm:"-"`，不落库。

---

## 21. `midjourneys`

用途：Midjourney 专用任务表。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `code` | 业务状态码 |
| `user_id` | 用户 ID |
| `action` | Midjourney 动作类型 |
| `mj_id` | Midjourney 任务 ID |
| `prompt` | 原始提示词 |
| `prompt_en` | 英文提示词 |
| `description` | 描述 |
| `state` | 原始状态文本 |
| `submit_time` | 提交时间 |
| `start_time` | 开始时间 |
| `finish_time` | 完成时间 |
| `image_url` | 图片结果地址 |
| `video_url` | 视频结果地址 |
| `video_urls` | 多视频结果地址 |
| `status` | 状态 |
| `progress` | 进度 |
| `fail_reason` | 失败原因 |
| `channel_id` | 渠道 ID |
| `quota` | 消耗额度 |
| `buttons` | 可执行按钮/操作信息 |
| `properties` | 扩展属性 |

---

## 22. `perf_metrics`

用途：按模型+分组+时间桶聚合性能指标，供模型广场或监控面板使用。

表名来源：显式 `TableName()`，固定为 `perf_metrics`。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `model_name` | 模型名 |
| `group` | 分组 |
| `bucket_ts` | 时间桶时间戳 |
| `request_count` | 请求总数 |
| `success_count` | 成功请求数 |
| `total_latency_ms` | 总耗时 |
| `ttft_sum_ms` | 首 token 时间总和 |
| `ttft_count` | 首 token 时间样本数 |
| `output_tokens` | 输出 token 总量 |
| `generation_ms` | 生成阶段总耗时 |

---

## 23. `subscription_plans`

用途：订阅套餐定义表。

说明：SQLite 下有专门的兼容迁移逻辑，但逻辑表名仍为 `subscription_plans`。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `title` | 套餐标题 |
| `subtitle` | 套餐副标题 |
| `price_amount` | 套餐金额 |
| `currency` | 币种 |
| `duration_unit` | 时长单位，如 month |
| `duration_value` | 时长数值 |
| `custom_seconds` | 自定义时长秒数 |
| `enabled` | 是否启用 |
| `sort_order` | 排序 |
| `allow_balance_pay` | 是否允许余额支付 |
| `stripe_price_id` | Stripe 价格 ID |
| `creem_product_id` | Creem 产品 ID |
| `waffo_pancake_product_id` | Waffo Pancake 产品 ID |
| `max_purchase_per_user` | 单用户最大购买次数 |
| `upgrade_group` | 购买后升级到的用户分组 |
| `total_amount` | 套餐总额度 |
| `quota_reset_period` | 套餐额度重置周期 |
| `quota_reset_custom_seconds` | 自定义重置秒数 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

---

## 24. `subscription_orders`

用途：订阅购买订单。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `user_id` | 用户 ID |
| `plan_id` | 套餐 ID |
| `money` | 实付金额 |
| `trade_no` | 订单号/支付单号 |
| `payment_method` | 支付方式 |
| `payment_provider` | 支付提供商 |
| `status` | 订单状态 |
| `create_time` | 创建时间 |
| `complete_time` | 完成时间 |
| `provider_payload` | 支付提供商原始回执或补充数据 |

---

## 25. `user_subscriptions`

用途：用户实际生效中的订阅实例，不同于套餐模板。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `user_id` | 用户 ID |
| `plan_id` | 套餐 ID |
| `amount_total` | 订阅总额度 |
| `amount_used` | 已使用额度 |
| `start_time` | 生效开始时间 |
| `end_time` | 生效结束时间 |
| `status` | 订阅状态，如 active/expired/cancelled |
| `source` | 来源，如 order/admin |
| `last_reset_time` | 上次重置时间 |
| `next_reset_time` | 下次重置时间 |
| `upgrade_group` | 订阅带来的分组升级 |
| `prev_user_group` | 升级前原分组 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

---

## 26. `subscription_pre_consume_records`

用途：记录订阅额度的预扣状态，支持请求失败退款和幂等控制。

| 字段名 | 用途 |
|---|---|
| `id` | 主键 |
| `request_id` | 请求 ID，唯一，用于幂等 |
| `user_id` | 用户 ID |
| `user_subscription_id` | 用户订阅实例 ID |
| `pre_consumed` | 预扣额度 |
| `status` | 状态，如 consumed/refunded |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

---

## 补充说明

### 1. 哪些表最核心

如果只看系统运行主链路，最核心的是：

- `users`
- `tokens`
- `channels`
- `abilities`
- `logs`
- `options`
- `user_subscriptions`

### 2. 哪些字段最关键

如果只看请求路由和计费，最关键的是：

- `users.group`
- `tokens.group`
- `tokens.model_limits`
- `channels.models`
- `channels.group`
- `channels.priority`
- `abilities.(group, model, channel_id)`
- `logs.quota`
- `user_subscriptions.amount_total / amount_used`

### 3. 表名准确性说明

以下表名是代码中显式确认的：

- `custom_oauth_providers`
- `user_oauth_bindings`
- `perf_metrics`
- `checkins`
- `quota_data`
- `subscription_plans`

其余表名依据当前 GORM 默认命名规则推断，若后续项目全局改了 `NamingStrategy`，文档需要同步更新。

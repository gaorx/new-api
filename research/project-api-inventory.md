# 项目 API 清单

说明：以下清单基于 `router/api-router.go`、`router/relay-router.go`、`router/video-router.go`、`router/dashboard.go` 汇总，静态前端路由不计入；每个接口只用一句话说明用途。

## 一、公共与站点接口

- `GET /api/setup`：获取系统是否已初始化及初始化页面所需信息。
- `POST /api/setup`：提交系统首次初始化配置并创建初始管理员。
- `GET /api/status`：获取系统运行状态与基础配置信息。
- `GET /api/uptime/status`：获取 Uptime Kuma 可见性或健康检查状态。
- `GET /api/models`：获取当前登录用户可见的模型列表。
- `GET /api/status/test`：执行管理员状态测试接口以检查后端可用性。
- `GET /api/notice`：获取站点公告内容。
- `GET /api/user-agreement`：获取用户协议内容。
- `GET /api/privacy-policy`：获取隐私政策内容。
- `GET /api/about`：获取关于页面内容。
- `GET /api/home_page_content`：获取首页展示内容。
- `GET /api/pricing`：获取公开价格页或当前用户可见的定价信息。
- `GET /api/perf-metrics/summary`：获取性能指标摘要数据。
- `GET /api/perf-metrics`：获取完整性能指标明细。
- `GET /api/rankings`：获取排行榜数据。
- `GET /api/verification`：发送邮箱验证码用于注册或绑定等验证流程。
- `GET /api/reset_password`：发送密码重置邮件。
- `POST /api/user/reset`：使用验证码或令牌重置用户密码。
- `GET /api/oauth/state`：生成 OAuth 流程使用的状态码。
- `POST /api/oauth/email/bind`：绑定邮箱到当前账号或 OAuth 流程。
- `GET /api/oauth/wechat`：处理微信 OAuth 登录回调。
- `POST /api/oauth/wechat/bind`：将微信账号绑定到当前用户。
- `GET /api/oauth/telegram/login`：处理 Telegram 登录流程。
- `GET /api/oauth/telegram/bind`：将 Telegram 账号绑定到当前用户。
- `GET /api/oauth/:provider`：处理标准 OAuth 提供商的登录或回调流程。
- `GET /api/ratio_config`：获取模型倍率与价格相关配置。
- `POST /api/stripe/webhook`：接收 Stripe 支付回调。
- `POST /api/creem/webhook`：接收 Creem 支付回调。
- `POST /api/waffo/webhook`：接收 Waffo 支付回调。
- `POST /api/waffo-pancake/webhook/:env`：接收按环境区分的 Waffo Pancake 支付回调。
- `POST /api/verify`：执行统一安全验证流程。

## 二、用户与账户接口

- `POST /api/user/register`：注册新用户。
- `POST /api/user/login`：使用账号密码登录。
- `POST /api/user/login/2fa`：提交双因素验证码完成登录。
- `POST /api/user/passkey/login/begin`：开始 Passkey 登录挑战。
- `POST /api/user/passkey/login/finish`：完成 Passkey 登录验证。
- `GET /api/user/logout`：退出当前登录会话。
- `POST /api/user/epay/notify`：接收 Epay 充值支付回调。
- `GET /api/user/epay/notify`：兼容 Epay 的 GET 回调。
- `GET /api/user/groups`：获取当前上下文可用的用户分组列表。
- `GET /api/user/self/groups`：获取当前用户所属分组。
- `GET /api/user/self`：获取当前用户资料。
- `GET /api/user/models`：获取当前用户可用模型列表。
- `PUT /api/user/self`：更新当前用户资料。
- `DELETE /api/user/self`：删除当前用户账号。
- `GET /api/user/token`：为当前用户生成访问令牌或会话令牌。
- `GET /api/user/passkey`：查询当前用户 Passkey 状态。
- `POST /api/user/passkey/register/begin`：开始 Passkey 注册挑战。
- `POST /api/user/passkey/register/finish`：完成 Passkey 注册。
- `POST /api/user/passkey/verify/begin`：开始 Passkey 二次验证挑战。
- `POST /api/user/passkey/verify/finish`：完成 Passkey 二次验证。
- `DELETE /api/user/passkey`：删除当前用户的 Passkey 凭据。
- `GET /api/user/aff`：获取当前用户的邀请码或邀请信息。
- `GET /api/user/topup/info`：获取充值页面所需配置与说明。
- `GET /api/user/topup/self`：获取当前用户充值记录。
- `POST /api/user/topup`：执行余额充值。
- `POST /api/user/pay`：发起 Epay 支付订单。
- `POST /api/user/amount`：计算普通充值应付金额。
- `POST /api/user/stripe/pay`：发起 Stripe 支付订单。
- `POST /api/user/stripe/amount`：计算 Stripe 充值应付金额。
- `POST /api/user/creem/pay`：发起 Creem 支付订单。
- `POST /api/user/waffo/amount`：计算 Waffo 充值应付金额。
- `POST /api/user/waffo/pay`：发起 Waffo 支付订单。
- `POST /api/user/waffo-pancake/amount`：计算 Waffo Pancake 充值应付金额。
- `POST /api/user/waffo-pancake/pay`：发起 Waffo Pancake 支付订单。
- `POST /api/user/aff_transfer`：执行邀请额度转账。
- `PUT /api/user/setting`：更新当前用户设置。
- `GET /api/user/2fa/status`：获取当前用户双因素认证状态。
- `POST /api/user/2fa/setup`：初始化双因素认证配置。
- `POST /api/user/2fa/enable`：启用双因素认证。
- `POST /api/user/2fa/disable`：禁用双因素认证。
- `POST /api/user/2fa/backup_codes`：重新生成双因素备份码。
- `GET /api/user/checkin`：获取当前用户签到状态。
- `POST /api/user/checkin`：执行每日签到。
- `GET /api/user/oauth/bindings`：获取当前用户已绑定的 OAuth 账号列表。
- `DELETE /api/user/oauth/bindings/:provider_id`：解绑当前用户的指定 OAuth 账号。

### 补充：普通用户、`user.group` 与 `token.group`

- `user.group` 与 `token.group` 是两层不同概念：前者是用户自身所属分组，后者是某个 API Key/令牌在调用时使用的分组。
- 普通用户登录后可以通过 `GET /api/user/self` 看到自己的 `group`，但 `PUT /api/user/self` 的自助更新逻辑只覆盖 `username`、`display_name`、`password` 与部分设置，不提供主动修改 `user.group` 的能力。
- 管理员可以通过 `PUT /api/user` 更新用户资料，并且后端 `model.User.Edit()` 会实际写入 `group` 字段，因此普通用户的 `user.group` 主要由管理员被动设置。
- 订阅系统是一个例外：如果订阅套餐配置了 `upgrade_group`，购买或生效后会自动把用户切到对应 `user.group`，到期后再按逻辑回退。
- `GET /api/user/groups` / `GET /api/user/self/groups` 返回的不是“用户当前所属单一 group”，而是“当前用户可用的 group 列表及说明”，它是前端创建/编辑 API Key 时的候选来源。

### 补充：普通用户如何在前端设置 `token.group`

- 普通用户可在前端 `/_authenticated/keys` 页面管理自己的 API Key；页面入口对应 `web/default/src/routes/_authenticated/keys/index.tsx`。
- 页面右上角 `Create API Key` 按钮会打开 `ApiKeysMutateDrawer`，其 `Group` 下拉框就是 `token.group` 的设置入口。
- 该下拉框会先请求 `GET /api/user/self/groups` 获取“当前用户可用的分组”，然后允许用户在这些候选项中选择。
- 如果用户把某个 token 的 group 设为 `auto`，前端还会显示 `Cross-group retry` 选项，对应后端的 `cross_group_retry`。
- 因此，普通用户可以主动设置自己 token 的 `group`，但只能在系统判定“当前用户有权限使用”的 group 范围内选择。

## 三、用户管理接口

- `GET /api/user`：分页获取全部用户列表。
- `GET /api/user/topup`：获取全部用户充值记录。
- `POST /api/user/topup/complete`：由管理员手动完成一笔充值。
- `GET /api/user/search`：按条件搜索用户。
- `GET /api/user/:id/oauth/bindings`：查看指定用户的 OAuth 绑定情况。
- `DELETE /api/user/:id/oauth/bindings/:provider_id`：管理员解绑指定用户的某个 OAuth 账号。
- `DELETE /api/user/:id/bindings/:binding_type`：管理员清除用户某类绑定关系。
- `GET /api/user/:id`：获取指定用户详情。
- `POST /api/user`：创建新用户。
- `POST /api/user/manage`：批量或动作式管理用户状态与额度。
- `PUT /api/user`：更新用户信息。
- `DELETE /api/user/:id`：删除指定用户。
- `DELETE /api/user/:id/reset_passkey`：重置指定用户的 Passkey 状态。
- `GET /api/user/2fa/stats`：获取全站双因素认证统计数据。
- `DELETE /api/user/:id/2fa`：管理员强制关闭指定用户双因素认证。

## 四、订阅计费接口

- `GET /api/subscription/plans`：获取当前用户可购买的订阅套餐列表。
- `GET /api/subscription/self`：获取当前用户订阅状态。
- `PUT /api/subscription/self/preference`：更新当前用户的订阅偏好设置。
- `POST /api/subscription/balance/pay`：使用账户余额支付订阅订单。
- `POST /api/subscription/epay/pay`：发起订阅 Epay 支付。
- `POST /api/subscription/stripe/pay`：发起订阅 Stripe 支付。
- `POST /api/subscription/creem/pay`：发起订阅 Creem 支付。
- `POST /api/subscription/waffo-pancake/pay`：发起订阅 Waffo Pancake 支付。
- `GET /api/subscription/admin/plans`：获取全部订阅套餐配置。
- `POST /api/subscription/admin/plans`：创建订阅套餐。
- `PUT /api/subscription/admin/plans/:id`：更新指定订阅套餐。
- `PATCH /api/subscription/admin/plans/:id`：切换订阅套餐状态。
- `POST /api/subscription/admin/bind`：将订阅能力绑定到用户或资源。
- `GET /api/subscription/admin/users/:id/subscriptions`：查看指定用户的订阅记录。
- `POST /api/subscription/admin/users/:id/subscriptions`：为指定用户创建订阅。
- `POST /api/subscription/admin/user_subscriptions/:id/invalidate`：使指定用户订阅失效。
- `DELETE /api/subscription/admin/user_subscriptions/:id`：删除指定用户订阅记录。
- `POST /api/subscription/epay/notify`：接收订阅 Epay 异步回调。
- `GET /api/subscription/epay/notify`：兼容订阅 Epay 的 GET 回调。
- `GET /api/subscription/epay/return`：处理订阅 Epay 同步返回。
- `POST /api/subscription/epay/return`：兼容订阅 Epay 的 POST 返回。

## 五、系统选项与平台运维接口

- `GET /api/option`：获取全局系统选项。
- `PUT /api/option`：更新全局系统选项。
- `POST /api/option/payment_compliance`：确认支付合规配置。
- `GET /api/option/channel_affinity_cache`：查看渠道亲和缓存状态。
- `DELETE /api/option/channel_affinity_cache`：清空渠道亲和缓存。
- `POST /api/option/rest_model_ratio`：重置模型倍率配置。
- `POST /api/option/migrate_console_setting`：迁移旧版控制台配置键。
- `POST /api/option/waffo-pancake/catalog`：拉取或查看 Waffo Pancake 商品目录。
- `POST /api/option/waffo-pancake/pair`：创建 Waffo Pancake 商品映射关系。
- `POST /api/option/waffo-pancake/save`：保存 Waffo Pancake 配置。
- `POST /api/option/waffo-pancake/subscription-product`：创建 Waffo Pancake 订阅商品。
- `POST /api/option/waffo-pancake/subscription-product-options`：获取 Waffo Pancake 订阅商品候选项。
- `POST /api/custom-oauth-provider/discovery`：根据发现文档拉取自定义 OAuth 提供商配置。
- `GET /api/custom-oauth-provider`：获取全部自定义 OAuth 提供商。
- `GET /api/custom-oauth-provider/:id`：获取指定自定义 OAuth 提供商详情。
- `POST /api/custom-oauth-provider`：创建自定义 OAuth 提供商。
- `PUT /api/custom-oauth-provider/:id`：更新自定义 OAuth 提供商。
- `DELETE /api/custom-oauth-provider/:id`：删除自定义 OAuth 提供商。
- `GET /api/performance/stats`：获取服务性能统计信息。
- `DELETE /api/performance/disk_cache`：清理磁盘缓存。
- `POST /api/performance/reset_stats`：重置性能统计计数器。
- `POST /api/performance/gc`：触发一次手动 GC。
- `GET /api/performance/logs`：列出运行日志文件。
- `DELETE /api/performance/logs`：清理历史日志文件。
- `GET /api/ratio_sync/channels`：获取支持同步倍率的渠道列表。
- `POST /api/ratio_sync/fetch`：从上游抓取模型倍率或价格配置。

## 六、渠道管理接口

- `GET /api/channel`：获取全部渠道列表。
- `GET /api/channel/search`：按条件搜索渠道。
- `GET /api/channel/models`：获取渠道模型映射总览。
- `GET /api/channel/models_enabled`：获取已启用渠道的模型列表。
- `GET /api/channel/:id`：获取指定渠道详情。
- `POST /api/channel/:id/key`：安全地查看指定渠道密钥。
- `GET /api/channel/test`：批量测试全部渠道连通性。
- `GET /api/channel/test/:id`：测试指定渠道连通性。
- `GET /api/channel/update_balance`：批量刷新全部渠道余额。
- `GET /api/channel/update_balance/:id`：刷新指定渠道余额。
- `POST /api/channel`：新增渠道。
- `PUT /api/channel`：更新渠道配置。
- `DELETE /api/channel/disabled`：删除全部已禁用渠道。
- `POST /api/channel/tag/disabled`：批量禁用指定标签的渠道。
- `POST /api/channel/tag/enabled`：批量启用指定标签的渠道。
- `PUT /api/channel/tag`：批量修改渠道标签。
- `DELETE /api/channel/:id`：删除指定渠道。
- `POST /api/channel/batch`：批量删除渠道。
- `POST /api/channel/fix`：修复渠道能力或模型能力数据。
- `GET /api/channel/fetch_models/:id`：从指定渠道拉取上游模型列表。
- `POST /api/channel/fetch_models`：批量抓取渠道模型信息。
- `POST /api/channel/codex/oauth/start`：开始 Codex 渠道 OAuth 授权流程。
- `POST /api/channel/codex/oauth/complete`：完成 Codex 渠道 OAuth 授权流程。
- `POST /api/channel/:id/codex/oauth/start`：为指定渠道开始 Codex OAuth 授权。
- `POST /api/channel/:id/codex/oauth/complete`：为指定渠道完成 Codex OAuth 授权。
- `POST /api/channel/:id/codex/refresh`：刷新指定 Codex 渠道凭据。
- `GET /api/channel/:id/codex/usage`：获取指定 Codex 渠道用量。
- `POST /api/channel/ollama/pull`：拉取 Ollama 模型。
- `POST /api/channel/ollama/pull/stream`：以流式方式拉取 Ollama 模型。
- `DELETE /api/channel/ollama/delete`：删除 Ollama 模型。
- `GET /api/channel/ollama/version/:id`：获取指定 Ollama 渠道版本信息。
- `POST /api/channel/batch/tag`：批量设置渠道标签。
- `GET /api/channel/tag/models`：获取标签到模型的映射信息。
- `POST /api/channel/copy/:id`：复制指定渠道配置。
- `POST /api/channel/multi_key/manage`：管理渠道多密钥配置。
- `POST /api/channel/upstream_updates/apply`：应用指定渠道的上游模型更新。
- `POST /api/channel/upstream_updates/apply_all`：应用全部渠道的上游模型更新。
- `POST /api/channel/upstream_updates/detect`：检测指定渠道的上游模型变化。
- `POST /api/channel/upstream_updates/detect_all`：检测全部渠道的上游模型变化。

## 七、令牌、兑换码、日志与数据接口

- `GET /api/token`：获取当前用户全部令牌。
- `GET /api/token/search`：按条件搜索当前用户令牌。
- `GET /api/token/:id`：获取指定令牌详情。
- `POST /api/token/:id/key`：安全地查看指定令牌密钥。
- `POST /api/token`：创建新令牌。
- `PUT /api/token`：更新令牌配置。
- `DELETE /api/token/:id`：删除指定令牌。
- `POST /api/token/batch`：批量删除令牌。
- `POST /api/token/batch/keys`：批量查看多枚令牌密钥。
- `GET /api/usage/token`：使用只读令牌查询该令牌的用量。
- `GET /api/redemption`：获取全部兑换码列表。
- `GET /api/redemption/search`：按条件搜索兑换码。
- `GET /api/redemption/:id`：获取指定兑换码详情。
- `POST /api/redemption`：创建兑换码。
- `PUT /api/redemption`：更新兑换码。
- `DELETE /api/redemption/invalid`：删除失效兑换码。
- `DELETE /api/redemption/:id`：删除指定兑换码。
- `GET /api/log`：获取全站消费日志。
- `DELETE /api/log`：删除历史日志。
- `GET /api/log/stat`：获取全站日志统计。
- `GET /api/log/self/stat`：获取当前用户日志统计。
- `GET /api/log/channel_affinity_usage_cache`：获取渠道亲和用量缓存统计。
- `GET /api/log/search`：搜索全站日志。
- `GET /api/log/self`：获取当前用户日志。
- `GET /api/log/self/search`：搜索当前用户日志。
- `GET /api/log/token`：使用只读令牌查询对应日志。
- `GET /api/data`：获取全站按日期聚合的额度数据。
- `GET /api/data/users`：获取按用户聚合的额度日期数据。
- `GET /api/data/self`：获取当前用户按日期聚合的额度数据。

### 补充：`/api/token` 接口的 group 语义

- `POST /api/token` 与 `PUT /api/token` 都允许普通用户提交 `group` 字段，因此用户可以在创建或编辑 API Key 时主动设置 `token.group`。
- 但 `token.group` 的可写不等于可任意使用：真正请求模型时，`middleware/auth.go` 会检查该 `token.group` 是否属于当前用户可用分组；如果不在允许范围内，请求会被拒绝。
- 从实际效果看，普通用户拥有“为自己的 token 选择 group”的能力，但不拥有“越权声明任意 group 并成功使用”的能力。

## 八、分组、任务、供应商与模型管理接口

- `GET /api/group`：获取系统分组列表。
- `GET /api/prefill_group`：获取预设分组列表。
- `POST /api/prefill_group`：创建预设分组。
- `PUT /api/prefill_group`：更新预设分组。
- `DELETE /api/prefill_group/:id`：删除预设分组。
- `GET /api/mj/self`：获取当前用户的 Midjourney 任务记录。
- `GET /api/mj`：获取全站 Midjourney 任务记录。
- `GET /api/task/self`：获取当前用户的异步任务记录。
- `GET /api/task`：获取全站异步任务记录。
- `GET /api/vendors`：获取全部供应商元数据。
- `GET /api/vendors/search`：按条件搜索供应商元数据。
- `GET /api/vendors/:id`：获取指定供应商元数据详情。
- `POST /api/vendors`：创建供应商元数据。
- `PUT /api/vendors`：更新供应商元数据。
- `DELETE /api/vendors/:id`：删除供应商元数据。
- `GET /api/models/sync_upstream/preview`：预览上游模型同步差异。
- `POST /api/models/sync_upstream`：执行上游模型同步。
- `GET /api/models/missing`：获取缺失模型清单。
- `GET /api/models`：获取全部模型元数据。
- `GET /api/models/search`：按条件搜索模型元数据。
- `GET /api/models/:id`：获取指定模型元数据详情。
- `POST /api/models`：创建模型元数据。
- `PUT /api/models`：更新模型元数据。
- `DELETE /api/models/:id`：删除模型元数据。

## 九、部署管理接口

- `GET /api/deployments/settings`：获取模型部署功能相关设置。
- `POST /api/deployments/settings/test-connection`：测试部署设置中的 Io.Net 连接。
- `GET /api/deployments`：获取全部部署列表。
- `GET /api/deployments/search`：按条件搜索部署。
- `POST /api/deployments/test-connection`：测试部署服务连接。
- `GET /api/deployments/hardware-types`：获取可选硬件规格列表。
- `GET /api/deployments/locations`：获取可选部署地区列表。
- `GET /api/deployments/available-replicas`：获取当前可用副本容量。
- `POST /api/deployments/price-estimation`：估算部署价格。
- `GET /api/deployments/check-name`：检查部署名称是否可用。
- `POST /api/deployments`：创建新部署。
- `GET /api/deployments/:id`：获取指定部署详情。
- `GET /api/deployments/:id/logs`：获取指定部署日志。
- `GET /api/deployments/:id/containers`：列出指定部署下的容器。
- `GET /api/deployments/:id/containers/:container_id`：获取指定容器详情。
- `PUT /api/deployments/:id`：更新指定部署配置。
- `PUT /api/deployments/:id/name`：单独修改部署名称。
- `POST /api/deployments/:id/extend`：延长部署有效期。
- `DELETE /api/deployments/:id`：删除指定部署。

## 十、旧版 Dashboard 兼容接口

- `GET /dashboard/billing/subscription`：获取 OpenAI 风格 Dashboard 订阅信息。
- `GET /v1/dashboard/billing/subscription`：获取带 `v1` 前缀的 Dashboard 订阅信息。
- `GET /dashboard/billing/usage`：获取 OpenAI 风格 Dashboard 用量信息。
- `GET /v1/dashboard/billing/usage`：获取带 `v1` 前缀的 Dashboard 用量信息。

## 十一、Relay 通用模型与聊天接口

- `GET /v1/models`：按请求头协议返回 OpenAI、Anthropic 或 Gemini 风格模型列表。
- `GET /v1/models/:model`：按请求头协议返回指定模型详情。
- `GET /v1beta/models`：返回 Gemini 风格模型列表。
- `GET /v1beta/openai/models`：返回 OpenAI 兼容格式的模型列表。
- `POST /pg/chat/completions`：以 Playground 上下文发起聊天补全请求。
- `GET /v1/realtime`：发起 OpenAI Realtime WebSocket 代理连接。
- `POST /v1/messages`：转发 Anthropic Claude `messages` 请求。
- `POST /v1/completions`：转发 OpenAI `completions` 请求。
- `POST /v1/chat/completions`：转发 OpenAI `chat/completions` 请求。
- `POST /v1/responses`：转发 OpenAI `responses` 请求。
- `POST /v1/responses/compact`：转发 OpenAI `responses` 紧凑模式请求。
- `POST /v1/edits`：转发 OpenAI 图像编辑或编辑类请求。
- `POST /v1/images/generations`：转发图像生成请求。
- `POST /v1/images/edits`：转发图像编辑请求。
- `POST /v1/embeddings`：转发向量嵌入请求。
- `POST /v1/audio/transcriptions`：转发音频转写请求。
- `POST /v1/audio/translations`：转发音频翻译请求。
- `POST /v1/audio/speech`：转发文本转语音请求。
- `POST /v1/rerank`：转发重排序请求。
- `POST /v1/engines/:model/embeddings`：兼容旧式 Gemini 或特殊格式嵌入请求。
- `POST /v1/models/*path`：转发 Gemini 风格 `/models/{model}:{action}` 请求。
- `POST /v1/moderations`：转发内容审核请求。

## 十二、Relay 未实现兼容接口

- `POST /v1/images/variations`：预留图像变体接口但当前未实现。
- `GET /v1/files`：预留文件列表接口但当前未实现。
- `POST /v1/files`：预留文件上传接口但当前未实现。
- `DELETE /v1/files/:id`：预留文件删除接口但当前未实现。
- `GET /v1/files/:id`：预留文件详情接口但当前未实现。
- `GET /v1/files/:id/content`：预留文件内容接口但当前未实现。
- `POST /v1/fine-tunes`：预留微调创建接口但当前未实现。
- `GET /v1/fine-tunes`：预留微调列表接口但当前未实现。
- `GET /v1/fine-tunes/:id`：预留微调详情接口但当前未实现。
- `POST /v1/fine-tunes/:id/cancel`：预留微调取消接口但当前未实现。
- `GET /v1/fine-tunes/:id/events`：预留微调事件接口但当前未实现。
- `DELETE /v1/models/:model`：预留模型删除接口但当前未实现。

## 十三、Midjourney 与异步任务 Relay 接口

说明：以下 Midjourney 路由同时存在两套前缀，分别是 `/mj` 与 `/:mode/mj`，后者用于带模式参数的同构转发。

- `GET /mj/image/:id`：代理获取 Midjourney 生成图片内容。
- `POST /mj/submit/action`：提交 Midjourney 动作任务。
- `POST /mj/submit/shorten`：提交 Midjourney 提示词缩短任务。
- `POST /mj/submit/modal`：提交 Midjourney 模态交互任务。
- `POST /mj/submit/imagine`：提交 Midjourney 文生图任务。
- `POST /mj/submit/change`：提交 Midjourney 变体或放大等修改任务。
- `POST /mj/submit/simple-change`：提交简化版 Midjourney 修改任务。
- `POST /mj/submit/describe`：提交 Midjourney 反推描述任务。
- `POST /mj/submit/blend`：提交 Midjourney 混图任务。
- `POST /mj/submit/edits`：提交 Midjourney 编辑任务。
- `POST /mj/submit/video`：提交 Midjourney 视频任务。
- `GET /mj/task/:id/fetch`：查询 Midjourney 任务结果。
- `GET /mj/task/:id/image-seed`：获取 Midjourney 任务图像种子信息。
- `POST /mj/task/list-by-condition`：按条件查询 Midjourney 任务列表。
- `POST /mj/insight-face/swap`：提交 Midjourney 换脸任务。
- `POST /mj/submit/upload-discord-images`：上传 Midjourney 相关 Discord 图片素材。
- `POST /suno/submit/:action`：提交 Suno 异步音乐生成任务。
- `POST /suno/fetch`：按请求体查询 Suno 任务状态。
- `GET /suno/fetch/:id`：按任务 ID 查询 Suno 任务状态。
- `POST /v1beta/models/*path`：按 Gemini 原生格式转发 `v1beta` 模型动作请求。

## 十四、视频 Relay 接口

- `GET /v1/videos/:task_id/content`：代理下载视频任务生成的内容文件。
- `POST /v1/video/generations`：创建 OpenAI 风格视频生成任务。
- `GET /v1/video/generations/:task_id`：查询 OpenAI 风格视频生成任务状态。
- `POST /v1/videos/:video_id/remix`：提交视频二次混剪或重制任务。
- `POST /v1/videos`：创建 OpenAI 兼容视频任务。
- `GET /v1/videos/:task_id`：查询 OpenAI 兼容视频任务状态。
- `POST /kling/v1/videos/text2video`：提交 Kling 文生视频任务。
- `POST /kling/v1/videos/image2video`：提交 Kling 图生视频任务。
- `GET /kling/v1/videos/text2video/:task_id`：查询 Kling 文生视频任务状态。
- `GET /kling/v1/videos/image2video/:task_id`：查询 Kling 图生视频任务状态。
- `POST /jimeng`：按即梦官方协议提交异步视频或视觉任务。

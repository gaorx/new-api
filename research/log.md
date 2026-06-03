# 日志与 Relay 调用记录

## 1. 这套系统里，用户每次 Relay 调用是否都会留下记录

结论先说在前面：

- 绝大多数“成功完成并发生消费/计费”的 Relay 调用都会写一条消费日志到 `logs` 表。
- 失败调用是否写错误日志，取决于是否开启 `ERROR_LOG_ENABLED`。
- 不是所有请求都会留下“消费日志”。
- 纯查询、纯回调、任务轮询类接口通常不会写 `type=2` 消费日志。

更准确地说，这个系统里和 Relay 相关的记录分成三层：

1. 明细日志：写入 `logs` 表。
2. 聚合统计：写入 `quota_data` 表。
3. 特定能力任务记录：例如 `midjourneys`、`tasks`。

如果用户在问“每一次 Relay 调用历史在哪里看”，通常真正对应的是 `logs` 表中的明细日志。

## 1.1 “使用日志”和“绘图日志”在这个项目里分别指什么

这两个名字很容易被混淆，但它们在系统里不是同一套数据。

### 使用日志

“使用日志”对应的是通用 API 调用/消费/错误记录，核心是 `logs` 表。

- 更偏向“这次 API 调用消耗了什么、是否报错、计费多少”
- 前端接口是 `/api/log` 与 `/api/log/self`
- 管理/用户侧看到的通常是通用请求明细，而不是某个异步任务对象本身

适合回答的问题通常是：

- 这次调用有没有记账
- 用了哪个模型、哪个渠道
- 消耗了多少额度或 token
- 是否产生错误日志

### 绘图日志

“绘图日志”对应的是 Midjourney 专用任务记录，核心是 `midjourneys` 表，不是通用 `logs` 表。

- 更偏向“这次绘图任务后来怎么样了”
- 前端接口是 `/api/mj` 与 `/api/mj/self`
- 本质上更接近“生图任务台账”或“任务状态追踪”

典型字段包括：

- `mj_id`
- `prompt`
- `action`
- `status`
- `progress`
- `submit_time`
- `start_time`
- `finish_time`
- `image_url`
- `fail_reason`

适合回答的问题通常是：

- 这次绘图任务是否已经提交
- 当前在排队、执行中、成功还是失败
- 最终图片地址是什么
- 失败原因是什么

一句话区分：

- `使用日志` = API 调用台账
- `绘图日志` = 生图任务台账

所以如果有人问“绘图日志是不是生图请求调用 API 的日志”，更准确的回答是：

- 它和生图请求有关
- 但它不是通用请求明细日志
- 它主要保存 Midjourney 任务对象及其状态流转

## 2. 日志落在什么库、什么表

### 2.1 主表：`logs`

日志模型定义在 [model/log.go](../model/log.go:34)。

核心字段包括：

- `type`
- `user_id`
- `username`
- `model_name`
- `token_name`
- `quota`
- `prompt_tokens`
- `completion_tokens`
- `use_time`
- `is_stream`
- `channel_id`
- `token_id`
- `group`
- `ip`
- `request_id`
- `upstream_request_id`
- `other`

日志类型定义也在同一文件：

- `LogTypeConsume = 2`
- `LogTypeError = 5`

### 2.2 `LOG_DB` 与独立日志库

日志不是一定和主业务表在同一个数据库里。

初始化逻辑在 [model/main.go](../model/main.go:213)：

- 如果 `LOG_SQL_DSN` 为空，则 `LOG_DB = DB`
- 如果 `LOG_SQL_DSN` 非空，则日志可以落到独立日志库

因此：

- 默认情况下，`logs` 和主业务表同库
- 配置了 `LOG_SQL_DSN` 后，`logs` 可能位于独立日志库

### 2.3 聚合表：`quota_data`

`quota_data` 不是逐条调用明细，而是按小时聚合后的看板统计数据。

定义与写入逻辑见 [model/usedata.go](../model/usedata.go:12)。

它主要用于：

- 看板
- 按时间聚合用量
- 用户/模型维度统计

不是“某次请求发生了什么”的原始事实来源。

## 3. 哪些开关会影响日志

### 3.1 `LogConsumeEnabled`

消费日志总开关见 [common/constants.go](../common/constants.go:115)。

当它关闭时：

- `RecordConsumeLog(...)` 直接返回
- `LogTypeConsume` 不会写入

对应代码见 [model/log.go](../model/log.go:223)。

### 3.2 `ERROR_LOG_ENABLED`

错误日志总开关初始化见 [common/init.go](../common/init.go:150)。

同步 Relay 错误收口处见 [controller/relay.go](../controller/relay.go:481)。

开启时：

- 满足 `types.IsRecordErrorLog(err)` 的失败调用会写 `LogTypeError`

### 3.3 `DataExportEnabled`

当 `DataExportEnabled` 开启时，消费日志写入后会异步累计到 `quota_data` 聚合缓存。

见 [model/log.go](../model/log.go:268)。

## 4. Relay 日志写入的主入口

### 4.1 消费日志

核心函数是 [model/log.go](../model/log.go:223) 的：

- `RecordConsumeLog(c, userId, params)`

它会写入：

- `type = LogTypeConsume`
- `request_id`
- `upstream_request_id`
- `ip`（仅在用户开启 `RecordIpLog` 时）
- `other` JSON

### 4.2 错误日志

核心函数是 [model/log.go](../model/log.go:163) 的：

- `RecordErrorLog(...)`

它会写入：

- `type = LogTypeError`
- `content` 为错误文案
- `quota = 0`
- `prompt_tokens = 0`
- `completion_tokens = 0`
- `request_id`
- `upstream_request_id`
- `other` 中的错误诊断字段

### 4.3 普通操作日志

`RecordLog(...)`、`RecordLogWithAdminInfo(...)` 也写入 `logs` 表，但它们主要用于：

- 系统操作
- 充值
- 管理动作

不是 Relay 调用明细的主来源。

## 5. 同步 Relay 的主分派与日志落点

同步 Relay 总入口在 [controller/relay.go](../controller/relay.go:79)。

主分派逻辑在 [controller/relay.go](../controller/relay.go:38)：

- 图片：`ImageHelper`
- 音频：`AudioHelper`
- Rerank：`RerankHelper`
- Embeddings：`EmbeddingHelper`
- Responses：`ResponsesHelper`
- 其他默认走 `TextHelper`

日志最终落点主要分几类：

- 文本/Embedding/Rerank/Responses：最终走文本消费日志链
- 音频：走音频消费日志链
- 图片：走图片消费日志链
- Realtime：走 WebSocket/Realtime 消费日志链
- Midjourney：走 Midjourney 专用消费日志链
- 异步任务：任务提交成功后走任务消费日志链

## 6. 各类 Relay 的日志格式骨架

下面的例子都是“近似真实结构”，实际字段会因：

- 上游是否返回 usage
- 是否模型映射
- 是否订阅计费
- 是否命中 tiered billing
- 是否为流式

而略有增减。

### 6.1 文本类

典型来源：

- `/v1/chat/completions`
- `/pg/chat/completions`
- `/v1/completions`
- `/v1/responses`
- `/v1/responses/compact`
- 大多数 Gemini 文本生成
- 默认走 `TextHelper` 的其他文本能力

典型结构：

```json
{
  "type": 2,
  "model_name": "gpt-4.1",
  "prompt_tokens": 1200,
  "completion_tokens": 340,
  "quota": 8450,
  "content": "模型倍率 15.00，补全倍率 2.00，分组倍率 1.00",
  "other": {
    "model_ratio": 15,
    "group_ratio": 1,
    "completion_ratio": 2,
    "cache_tokens": 0,
    "cache_ratio": 0,
    "model_price": 0,
    "user_group_ratio": 1,
    "frt": 842,
    "request_path": "/v1/chat/completions",
    "billing_source": "wallet",
    "admin_info": {
      "use_channel": ["12"]
    }
  }
}
```

### 6.2 Claude 文本类补充

在文本类基础上，`other` 里经常多出：

```json
{
  "claude": true,
  "cache_creation_tokens": 4096,
  "cache_creation_ratio": 1.25
}
```

见 [service/log_info_generate.go](../service/log_info_generate.go:236)。

### 6.3 音频类

典型来源：

- `/v1/audio/speech`
- `/v1/audio/transcriptions`
- `/v1/audio/translations`

典型结构：

```json
{
  "type": 2,
  "model_name": "gpt-4o-mini-transcribe",
  "prompt_tokens": 0,
  "completion_tokens": 980,
  "quota": 2100,
  "content": "模型倍率 1.00，补全倍率 1.00，音频倍率 2.00，音频补全倍率 1.00，分组倍率 1.00",
  "other": {
    "audio": true,
    "audio_input": 0,
    "audio_output": 980,
    "text_input": 0,
    "text_output": 980,
    "audio_ratio": 2,
    "audio_completion_ratio": 1,
    "request_path": "/v1/audio/transcriptions"
  }
}
```

### 6.4 Realtime

典型来源：

- `/v1/realtime`

典型结构：

```json
{
  "type": 2,
  "is_stream": true,
  "content": "模型倍率 12.00，补全倍率 2.00，音频倍率 2.00，音频补全倍率 2.00，分组倍率 1.00",
  "other": {
    "ws": true,
    "audio_input": 120,
    "audio_output": 180,
    "text_input": 180,
    "text_output": 240,
    "audio_ratio": 2,
    "audio_completion_ratio": 2,
    "request_path": "/v1/realtime"
  }
}
```

### 6.5 图片类

典型来源：

- `/v1/images/generations`
- `/v1/images/edits`

典型结构：

```json
{
  "type": 2,
  "model_name": "gpt-image-1",
  "quota": 5000,
  "content": "模型价格 0.04，分组倍率 1.00",
  "other": {
    "model_price": 0.04,
    "group_ratio": 1,
    "request_path": "/v1/images/generations",
    "image_generation_call": true,
    "image_generation_call_price": 0.04
  }
}
```

### 6.6 Embeddings / Rerank

典型结构：

```json
{
  "type": 2,
  "model_name": "text-embedding-3-large",
  "prompt_tokens": 560,
  "completion_tokens": 0,
  "quota": 320,
  "content": "模型倍率 0.50，分组倍率 1.00",
  "other": {
    "model_ratio": 0.5,
    "group_ratio": 1,
    "request_path": "/v1/embeddings"
  }
}
```

### 6.7 异步任务提交类

成功提交异步任务后会记消费日志，逻辑见 [controller/relay.go](../controller/relay.go:737) 和 [service/task_billing.go](../service/task_billing.go:19)。

典型结构：

```json
{
  "type": 2,
  "model_name": "suno-v3",
  "quota": 20000,
  "content": "操作 submit，按次计费",
  "other": {
    "is_task": true,
    "request_path": "/task/submit",
    "model_price": 0.2,
    "group_ratio": 1.5
  }
}
```

### 6.8 Midjourney 提交类

典型结构：

```json
{
  "type": 2,
  "model_name": "midjourney",
  "quota": 50000,
  "content": "模型固定价格 0.10，分组倍率 1.00，操作 imagine，ID 0741798445574458",
  "other": {
    "model_price": 0.1,
    "group_ratio": 1,
    "request_path": "/mj/submit/imagine"
  }
}
```

`swap-face` 会更像：

```json
{
  "content": "模型固定价格 0.10，分组倍率 1.00，操作 swap-face",
  "other": {
    "request_path": "/mj/insight-face/swap"
  }
}
```

### 6.9 违规扣费

见 [service/violation_fee.go](../service/violation_fee.go:150)。

典型结构：

```json
{
  "type": 2,
  "content": "Violation fee charged",
  "other": {
    "violation_fee": true,
    "violation_fee_code": "violation_fee_grok_csam",
    "fee_quota": 100000,
    "base_amount": 100,
    "group_ratio": 1,
    "status_code": 400,
    "upstream_error_type": "invalid_request_error",
    "upstream_error_code": "content_policy",
    "violation_fee_marker": "csam"
  }
}
```

### 6.10 错误日志

见 [model/log.go](../model/log.go:163)。

典型结构：

```json
{
  "type": 5,
  "model_name": "gpt-4.1",
  "quota": 0,
  "content": "upstream returned 429: rate limit exceeded",
  "other": {
    "request_path": "/v1/chat/completions",
    "error_type": "upstream_error",
    "error_code": "bad_response_status_code",
    "status_code": 429,
    "channel_id": 12,
    "channel_name": "openai-main",
    "channel_type": 1,
    "admin_info": {
      "use_channel": ["12", "15"]
    }
  }
}
```

## 7. `request_path -> 日志格式` 对照

下面这张表聚焦“从请求路径推断会落什么日志”。

| `request_path` | 主要 Relay 类别 | 默认是否记 `type=2` consume log | 是否可能记 `type=5` error log | `content` 模板 | `other` 关键字段 |
| --- | --- | --- | --- | --- | --- |
| `/v1/chat/completions` | 文本 | 是 | 是 | `模型倍率 %.2f，补全倍率 %.2f，分组倍率 %.2f` | `model_ratio` `group_ratio` `completion_ratio` `request_path` |
| `/pg/chat/completions` | 文本 | 是 | 是 | 同上 | 同上，仅 `request_path` 不同 |
| `/v1/completions` | 文本 | 是 | 是 | 同上 | 同上 |
| `/v1/embeddings` | Embedding | 是 | 是 | `模型倍率 %.2f，分组倍率 %.2f` | `model_ratio` `group_ratio` `request_path` |
| `...embeddings` | Embedding | 是 | 是 | 同上 | 同上，路径为实际 suffix 命中的路径 |
| `/v1/moderations` | Moderation | 通常是 | 是 | 通常与文本类类似 | `request_path` |
| `/v1/rerank` | Rerank | 是 | 是 | 通常与 Embedding 类似 | `request_path` |
| `/v1/responses` | Responses | 是 | 是 | 文本类模板 | `request_path` `input_tokens_total` 以及可能的搜索/工具字段 |
| `/v1/responses/compact` | Responses Compact | 是 | 是 | 文本类模板 | 同上 |
| `/v1/images/generations` | 图片 | 是 | 是 | `模型价格 %.2f，分组倍率 %.2f` | `model_price` `group_ratio` `image_generation_call` `request_path` |
| `/v1/images/edits` | 图片 | 是 | 是 | 通常同上 | `request_path` |
| `/v1/audio/speech` | 音频 | 是 | 是 | `模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f` | `audio` `audio_input` `audio_output` `text_input` `text_output` `request_path` |
| `/v1/audio/transcriptions` | 音频 | 是 | 是 | 同上 | 同上 |
| `/v1/audio/translations` | 音频 | 是 | 是 | 同上 | 同上 |
| `/v1/realtime` | Realtime | 是 | 是 | Realtime/音频混合模板 | `ws` `audio_input` `audio_output` `text_input` `text_output` `request_path` |
| `/v1beta/models/...` | Gemini | 是 | 是 | 文本类或 Embedding 类模板 | `request_path` |
| `/v1/models/...` | Gemini | 是 | 是 | 文本类或 Embedding 类模板 | `request_path` |
| `/mj/submit/imagine` | Midjourney 提交 | 是 | 是 | `模型固定价格 %.2f，分组倍率 %.2f，操作 imagine，ID %s` | `model_price` `group_ratio` `request_path` |
| `/mj/submit/describe` | Midjourney 提交 | 是 | 是 | `...操作 describe，ID %s` | 同上 |
| `/mj/submit/blend` | Midjourney 提交 | 是 | 是 | `...操作 blend，ID %s` | 同上 |
| `/mj/submit/change` | Midjourney 提交 | 是 | 是 | `...操作 change，ID %s` | 同上 |
| `/mj/submit/modal` | Midjourney 提交 | 是 | 是 | `...操作 modal，ID %s` | 同上 |
| `/mj/submit/shorten` | Midjourney 提交 | 是 | 是 | `...操作 shorten，ID %s` | 同上 |
| `/mj/submit/video` | Midjourney 提交 | 是 | 是 | `...操作 video，ID %s` | 同上 |
| `/mj/submit/edits` | Midjourney 提交 | 是 | 是 | `...操作 edits，ID %s` | 同上 |
| `/submit/upload-discord-images` | Midjourney 上传 | 是 | 是 | `...操作 upload，ID %s` | 同上 |
| `/mj/insight-face/swap` | Midjourney swap face | 是 | 是 | `模型固定价格 %.2f，分组倍率 %.2f，操作 swap-face` | `model_price` `group_ratio` `request_path` |
| `/mj/notify` | Midjourney 回调 | 否 | 可能 | 无统一 consume log 模板 | 若失败则走错误日志 |
| `.../fetch` | 任务/MJ 查询 | 通常否 | 可能 | 无 | 若失败则走错误日志 |
| `.../list-by-condition` | Midjourney 查询 | 否 | 可能 | 无 | 若失败则走错误日志 |
| `.../image-seed` | Midjourney 查询 | 否 | 可能 | 无 | 若失败则走错误日志 |
| `/task/submit` | 异步任务提交 | 是 | 可能 | `操作 %s，按次计费` 或 `操作 %s, 计算参数：...` | `is_task` `request_path` `model_price` `group_ratio` |
| `/task/fetch` | 异步任务查询 | 否 | 可能 | 无 | 若失败则走错误日志 |

## 8. 哪些请求默认不会产生消费日志

以下类型默认更接近“查询态”而不是“消费态”：

- 任务轮询/查询
- Midjourney 回调
- Midjourney 条件查询
- image seed 查询

因此它们通常：

- 不记 `type=2`
- 但如果出错，仍可能记 `type=5`

## 9. `other` 字段中最值得关注的分类标记

排查日志时，下面这些字段非常有帮助：

- `request_path`：最直接的请求分类线索
- `claude=true`：最终上游请求格式是 Claude Messages
- `ws=true`：Realtime
- `audio=true`：音频类
- `is_task=true`：异步任务提交
- `image_generation_call=true`：图片按次计费
- `violation_fee=true`：违规扣费专用日志
- `request_conversion`：发生过协议格式转换
- `upstream_model_name`：模型映射后的实际上游模型名
- `billing_source`：`wallet` 或 `subscription`
- `stream_status`：流式结束状态
- `po`：参数覆盖审计信息

## 10. 与日志直接相关的代码入口

推荐从下面这些文件开始看：

- [model/log.go](../model/log.go)
- [model/usedata.go](../model/usedata.go)
- [model/main.go](../model/main.go)
- [controller/relay.go](../controller/relay.go)
- [service/text_quota.go](../service/text_quota.go)
- [service/quota.go](../service/quota.go)
- [service/log_info_generate.go](../service/log_info_generate.go)
- [service/task_billing.go](../service/task_billing.go)
- [service/violation_fee.go](../service/violation_fee.go)
- [relay/mjproxy_handler.go](../relay/mjproxy_handler.go)

## 11. 一句话总结

这个项目里的 Relay 日志体系，本质上是：

- 用 `logs` 保存请求级明细事实
- 用 `quota_data` 保存按小时聚合统计
- 用 `tasks` / `midjourneys` 保存特定异步任务或专用任务对象

其中真正回答“某次 Relay 调用发生了什么”的，是 `logs` 表里的消费日志和错误日志。

# Relay 转发格式调研

## 调研问题

`relay/` 包中的转发原理，究竟是：

1. 先把不同协议请求转换到一种统一的内部格式，再桥接到目标上游；
2. 还是各协议之间直接互相转换。

结论是：**两者都有，但主路径是“流程统一 + OpenAI 格式充当事实上的中间桥”，而不是完整的协议两两直连网状转换。**

## 总体结论

这个项目的 Relay 架构可以拆成两层来看：

- **运行时流程是统一的**
  入口统一经过校验、`RelayInfo` 组装、token 估算、计费、选路、重试、错误回写。
- **请求/响应 payload 不是完全中立的统一 Canonical Model**
  但在文本与聊天主链路里，`dto.GeneralOpenAIRequest` 和 OpenAI 风格响应结构，实际上承担了“中间桥格式”的作用。

所以更准确的说法是：

```text
统一的是 Relay 运行时框架
不完全统一的是协议 DTO
而 OpenAI DTO 在文本主链路里经常被当作中间桥
```

## 证据 1：入口统一，靠 `RelayFormat` 和 `RelayMode` 分流

同步请求统一从 `controller/relay.go` 的 `Relay()` 进入，先做：

- 请求校验
- `RelayInfo` 构建
- 敏感词检测
- token 预估
- 价格计算
- 预扣费
- 重试控制

之后再根据协议入口和业务模式分发到不同 helper：

- `relayHandler()` 按 `RelayMode` 分到 `TextHelper`、`ImageHelper`、`EmbeddingHelper`、`ResponsesHelper` 等
- Claude 入口走 `ClaudeHelper`
- Gemini 入口走 `GeminiHelper`

这说明系统首先统一的是“运行时执行框架”，而不是一进来就把所有请求转成某个中间 DTO。

关键文件：

- `controller/relay.go`
- `relay/channel/adapter.go`
- `relay/common/relay_info.go`

## 证据 2：Adaptor 接口明确区分多种入口协议

`relay/channel/adapter.go` 中的 `Adaptor` 接口同时定义了：

- `ConvertOpenAIRequest(...)`
- `ConvertClaudeRequest(...)`
- `ConvertGeminiRequest(...)`
- `ConvertOpenAIResponsesRequest(...)`
- 以及 image / audio / embedding / rerank 等转换方法

这说明设计层面并不是只有一个“统一 request DTO -> provider DTO”的单入口，而是允许：

- 同协议直通
- 特定协议单独处理
- 借道某个中间格式再转换

也就是说，框架支持多种转换路径，不强制唯一 canonical request model。

## 证据 3：文本主链路里，OpenAI 格式经常充当中间桥

### 3.1 Claude -> OpenAI -> 目标渠道

OpenAI 类型渠道的 adaptor 中，`ConvertClaudeRequest()` 会先调用：

- `service.ClaudeToOpenAIRequest(...)`

把 Claude 请求转成 `dto.GeneralOpenAIRequest`，然后继续调用：

- `ConvertOpenAIRequest(...)`

也就是：

```text
Claude 请求 -> OpenAI 请求 -> OpenAI 渠道自己的最终请求
```

相关位置：

- `relay/channel/openai/adaptor.go`
- `service/convert.go`

### 3.2 Gemini -> OpenAI -> 目标渠道

同样在 OpenAI 类型 adaptor 中，`ConvertGeminiRequest()` 会先调用：

- `service.GeminiToOpenAIRequest(...)`

再交给 `ConvertOpenAIRequest(...)`。

也就是：

```text
Gemini 请求 -> OpenAI 请求 -> OpenAI 渠道自己的最终请求
```

相关位置：

- `relay/channel/openai/adaptor.go`
- `service/convert.go`

### 3.3 Claude -> OpenAI -> Gemini

Gemini adaptor 的 `ConvertClaudeRequest()` 更能说明问题。

它不是直接写一套 Claude -> Gemini 的转换逻辑，而是：

1. 先借用 `openai.Adaptor{}.ConvertClaudeRequest(...)`
2. 得到 `*dto.GeneralOpenAIRequest`
3. 再调用 Gemini 自己的 `ConvertOpenAIRequest(...)`

链路是：

```text
Claude 请求 -> OpenAI 请求 -> Gemini 请求
```

相关位置：

- `relay/channel/gemini/adaptor.go`

这个实现方式很直接地说明：**项目没有优先做协议两两直连，而是倾向于借 OpenAI 格式做桥。**

## 证据 4：也存在直接转换或原样直通

虽然 OpenAI 经常作为桥，但项目并不是“所有情况都必须先转 OpenAI”。

### 4.1 Gemini 原生入口到 Gemini 上游可以直接走

`GeminiHelper` 中会调用目标 adaptor 的 `ConvertGeminiRequest(...)`。

如果目标 adaptor 本身就是 Gemini adaptor，那么它会基本保留 Gemini 结构，只做少量修正，例如：

- 第一条 content 没有 role 时补成 `user`
- 某些文件 URL 推断 MIME type

这属于“同协议小修正后直发”，不是经 OpenAI 中转。

相关位置：

- `relay/gemini_handler.go`
- `relay/channel/gemini/adaptor.go`

### 4.2 Claude 原生入口到 Claude 上游可以直接走

Claude adaptor 的 `ConvertClaudeRequest(...)` 直接返回原请求：

```text
Claude 请求 -> Claude 请求
```

相关位置：

- `relay/channel/claude/adaptor.go`

### 4.3 Pass-through 模式下可能完全不做结构转换

多个 helper 都支持：

- `PassThroughRequestEnabled`
- `PassThroughBodyEnabled`

开启后会直接复用原始请求体，而不是重新 marshal 转换后的 DTO。

这意味着某些场景下 relay 更接近“透明代理”，而不是“格式桥接器”。

相关位置：

- `relay/compatible_handler.go`
- `relay/claude_handler.go`
- `relay/gemini_handler.go`
- `relay/responses_handler.go`

## 证据 5：响应侧也偏向“先变成 OpenAI，再按入口协议回写”

不仅请求侧如此，响应侧也有类似模式。

### 5.1 OpenAI 上游响应 -> Claude / Gemini 客户端响应

OpenAI channel 在处理响应时，会根据 `info.RelayFormat` 决定如何回写：

- OpenAI 入口：保持 OpenAI 风格
- Claude 入口：调用 `service.ResponseOpenAI2Claude(...)`
- Gemini 入口：调用 `service.ResponseOpenAI2Gemini(...)`

相关位置：

- `relay/channel/openai/relay-openai.go`
- `relay/channel/openai/helper.go`

### 5.2 Gemini 上游响应 -> 先转 OpenAI，再视入口决定是否继续转 Claude

Gemini channel 里先有：

- `responseGeminiChat2OpenAI(...)`
- `streamResponseGeminiChat2OpenAI(...)`

把 Gemini 响应规整为 OpenAI 风格。

之后如果客户端入口原本是 Claude，则再：

- `service.ResponseOpenAI2Claude(...)`

所以这里的响应链路是：

```text
Gemini 响应 -> OpenAI 响应 -> Claude 响应
```

相关位置：

- `relay/channel/gemini/relay-gemini.go`

这进一步证明：**OpenAI 风格响应也是一个事实上的内部桥接层。**

## 不是“完整统一格式”的原因

尽管 OpenAI DTO 很像内部标准格式，但它又不是严格意义上的全局 canonical model，原因有几条：

- `Adaptor` 接口仍然保留了 `ConvertClaudeRequest`、`ConvertGeminiRequest` 等多入口方法
- 某些适配器直接处理原生协议，不经过 OpenAI
- 某些协议转换尚未完全实现，例如 Claude adaptor 的 `ConvertGeminiRequest()` 仍是 `not implemented`
- image / audio / embedding / responses 等能力并不完全共享同一套 OpenAI 中间模型

所以更准确的说法不是“系统有唯一统一内部格式”，而是：

**在文本聊天这条最核心的 relay 主链路上，OpenAI 结构被广泛复用为桥接层。**

## 最终判断

如果只用一句话概括：

> `relay/` 的总体原理是“统一运行时 + 目标渠道 adaptor 转换”，其中在文本/chat 主链路上，Claude、Gemini 等协议经常先归一化到 OpenAI 风格请求/响应，再桥接到目标 provider；但系统也保留同协议直通、局部直接转换以及 pass-through 旁路，因此它不是一个严格单一 canonical DTO 架构，也不是完整的协议两两直转架构。

## RelayFormat 数量与“互转格式”边界

如果从 `types.RelayFormat` 的定义数量来看，项目里并不只有 3 种格式，而是一共声明了 12 种：

- `openai`
- `claude`
- `gemini`
- `openai_responses`
- `openai_responses_compaction`
- `openai_audio`
- `openai_image`
- `openai_realtime`
- `rerank`
- `embedding`
- `task`
- `mj_proxy`

定义位置：

- `types/relay_format.go`

但是这里需要区分“**RelayFormat 的数量**”和“**真正参与协议互转的主格式数量**”。

### 哪些是主协议互转格式

如果问题是：

> relay 包里真正参与聊天/文本协议互转的格式有多少种？

那么核心答案仍然是 **3 种**：

- `openai`
- `claude`
- `gemini`

原因是 `Adaptor` 接口中专门为它们保留了三套入口转换方法：

- `ConvertOpenAIRequest(...)`
- `ConvertClaudeRequest(...)`
- `ConvertGeminiRequest(...)`

定义位置：

- `relay/channel/adapter.go`

同时，项目中也明确存在围绕这三种协议的桥接函数：

- `ClaudeToOpenAIRequest(...)`
- `GeminiToOpenAIRequest(...)`
- `ResponseOpenAI2Claude(...)`
- `ResponseOpenAI2Gemini(...)`
- `StreamResponseOpenAI2Claude(...)`
- `StreamResponseOpenAI2Gemini(...)`

相关位置：

- `service/convert.go`
- `relay/channel/openai/helper.go`
- `relay/channel/gemini/relay-gemini.go`

因此，从“协议互转”视角看，当前最核心的互转集合就是：

```text
OpenAI <-> Claude
OpenAI <-> Gemini
Claude <-> Gemini 通常借 OpenAI 做桥
```

### 哪些不是同层级的“聊天协议互转格式”

其他 RelayFormat 虽然也属于 relay 支持的输入/处理形态，但更多是：

- OpenAI 家族变体
- 某类专项能力格式
- 任务型或代理型入口

例如：

- `openai_responses`
- `openai_responses_compaction`
- `openai_audio`
- `openai_image`
- `openai_realtime`

这几种都更接近 **OpenAI 生态内部的不同 API 形态**，而不是和 Claude、Gemini 平级的独立聊天协议。

再比如：

- `embedding`
- `rerank`

它们属于 **任务类型格式**，不是通用对话协议。

而：

- `task`
- `mj_proxy`

更偏向 **异步任务/代理通道**，也不属于主文本互转协议集合。

### 为什么会容易看成“不止 3 种互转”

因为 `Adaptor` 接口除了三种文本协议，还定义了：

- `ConvertOpenAIResponsesRequest(...)`
- `ConvertEmbeddingRequest(...)`
- `ConvertAudioRequest(...)`
- `ConvertImageRequest(...)`
- `ConvertRerankRequest(...)`

这说明 relay 确实支持很多“格式处理入口”，但这些入口里有相当一部分是：

- 针对具体能力的转换
- 针对 OpenAI 家族不同 API 的兼容
- 针对非聊天业务的请求封装

它们不应和 `openai / claude / gemini` 这三个主聊天协议简单并列理解为“多方协议互转”。

## 关于“支持多少种格式互转”的准确表述

更准确的说法是：

- 按 `RelayFormat` 总数算，当前项目声明了 **12 种格式**
- 按“主聊天协议互转”算，核心是 **3 种：OpenAI、Claude、Gemini**
- 其他格式更多是 **OpenAI 变体、专项能力格式、任务型入口**，不是同层级的通用聊天协议互转对象

## Adaptor 接口体系

除了“格式互转”本身，`relay/` 还有一个很关键的实现层抽象：**大多数渠道并不是各写各的散装逻辑，而是实现共同接口。**

### 同步转发渠道实现 `channel.Adaptor`

`relay/channel/adapter.go` 中定义了主同步转发接口 `Adaptor`。

它要求一个普通渠道适配器至少提供这些能力：

- 初始化请求级上下文
- 生成上游请求 URL
- 设置上游请求头
- 处理 OpenAI / Claude / Gemini / Responses / embedding / image / audio / rerank 等请求转换
- 发起请求
- 解析响应并回写给客户端
- 返回模型列表与渠道名

从项目用法上看，这就是同步 relay 主链路的统一插件接口。

`relay.GetAdaptor()` 的返回类型就是 `channel.Adaptor`，说明像下面这些实现都会被当成统一接口来调度：

- `openai.Adaptor`
- `claude.Adaptor`
- `gemini.Adaptor`
- `jina.Adaptor`
- `aws.Adaptor`
- `vertex.Adaptor`
- 以及其他 provider adaptor

也就是说，helper 层并不关心具体是哪个厂商，而是依赖共同的方法集去调用。

### 异步任务渠道实现 `channel.TaskAdaptor`

对于视频、音乐、任务型图片生成等异步提交/轮询场景，项目没有继续复用普通 `Adaptor`，而是单独定义了 `TaskAdaptor`。

相比普通同步 adaptor，`TaskAdaptor` 除了构造请求和解析响应外，还额外覆盖了异步任务生命周期中的几个关键阶段：

- `ValidateRequestAndSetAction(...)`
- `EstimateBilling(...)`
- `AdjustBillingOnSubmit(...)`
- `AdjustBillingOnComplete(...)`
- `FetchTask(...)`
- `ParseTaskResult(...)`

这意味着它不只是“请求转换器”，还是“异步任务生命周期处理器”：

```text
提交任务 -> 预扣费 -> 解析 submit 响应 -> 轮询任务状态 -> 终态结算
```

这类实现主要位于：

- `relay/channel/task/ali`
- `relay/channel/task/gemini`
- `relay/channel/task/kling`
- `relay/channel/task/sora`
- `relay/channel/task/vertex`
- `relay/channel/task/suno`

### `OpenAIVideoConverter` 是更小的可选接口

在 `TaskAdaptor` 之外，项目还定义了一个更小的可选接口：

- `OpenAIVideoConverter`

它只有一个方法：

- `ConvertToOpenAIVideo(...)`

这个接口不是所有 task adaptor 都必须实现，而是给那些需要把异步任务结果再包装成 OpenAI 风格视频响应的实现使用。

因此它更像一个“能力补充接口”，不是主转发接口。

### 这套接口体系和统一框架的关系

如果把 relay 的执行过程再抽象一层，可以理解为：

```text
Relay / Helper
  -> 选择普通 Adaptor 或 TaskAdaptor
  -> 调用统一接口方法
  -> 由具体 provider 实现各自差异
```

所以这个项目的“统一”主要落在三层：

- 统一入口控制流程
- 统一上下文对象 `RelayInfo`
- 统一适配器接口 `Adaptor` / `TaskAdaptor`

而“非统一”的部分主要在：

- 各 provider 的请求/响应 DTO 细节
- 哪些协议需要借 OpenAI 做桥
- 哪些能力尚未实现某些 `Convert*` 方法

综合前面的调研，如果用一句更工程化的话概括：

> `relay/` 不是一个单纯的“格式转换包”，而是一个以 `RelayInfo + Adaptor 接口体系` 为骨架的统一转发框架；其中普通同步请求走 `channel.Adaptor`，异步任务走 `channel.TaskAdaptor`，而在文本协议转换上，OpenAI DTO 又承担了事实上的桥接层角色。

## `relay` 中的 `channel` 到底指什么

在这个项目里，`relay` 所说的 `channel`，不是单纯的“OpenAI / Anthropic / Google”这种抽象厂商名字，而是一个**可被路由命中的具体上游通道配置**。

它对应的是 `model.Channel` 这条数据库记录，通常包含：

- 渠道类型 `Type`
- API Key
- Base URL
- 支持模型列表
- 分组 `Group`
- 模型映射
- Header / Param override
- 渠道设置与其他设置
- 渠道累计消耗 `UsedQuota`

因此，更准确地说：

```text
channel = 某个上游供应方下的一条具体接入实例 / 路由目标
```

而不是“厂商概念”本身。

### `RelayInfo` 里的 channel 信息

`RelayInfo` 中的 `ChannelMeta` 会把当前请求命中的 channel 元信息带入整个 relay 生命周期，例如：

- `ChannelType`
- `ChannelId`
- `ChannelBaseUrl`
- `ApiType`
- `ApiKey`
- `ChannelSetting`
- `ChannelOtherSettings`
- `UpstreamModelName`

这说明 channel 在 relay 中承担的是“**当前请求实际走哪条上游通道**”的角色，而不只是分类标签。

### channel 和计价的关系

channel 和计价有关系，但不是“由 channel 自己定义模型价格”。

主计价逻辑的核心仍然是：

- 当前模型名 `OriginModelName`
- 当前使用分组 `UsingGroup`
- 用户分组 `UserGroup`
- 配置中的模型价格 `model price`
- 配置中的模型倍率 `model ratio`
- 配置中的分组倍率 `group ratio`

计价入口在：

- `relay/helper/price.go`

这里会：

1. 先查模型价格 `GetModelPrice(...)`
2. 若不是固定价格模式，则查模型倍率 `GetModelRatio(...)`
3. 再叠加分组倍率 `GetGroupRatio(...)`
4. 最终得到预扣额度 `QuotaToPreConsume`

所以更准确地说：

**价格主要由“模型 + 分组 + 计费配置”驱动，而不是由 channel 直接定价。**

### channel 和倍率/缩放的关系

如果把“缩放”理解为项目里的各种 ratio / multiplier，那么 channel 的关系是**间接但重要**的。

它主要体现在三层：

1. **channel 决定本次请求最终走哪个上游实例**
   也就是确定 `ChannelId / ChannelType / ChannelBaseUrl / UpstreamModelName` 这一组上下文。

2. **计价逻辑基于该上下文去套用模型与分组倍率**
   特别是 `UsingGroup` 可能因为自动分组或重试而变化，从而影响 group ratio。

3. **某些 task adaptor 会基于渠道支持的能力返回额外倍率**
   例如视频生成中的时长、分辨率、视频输入折扣等，会通过 `EstimateBilling(...)` 返回 `OtherRatios`。
   这部分通常与具体 channel/provider 的计费维度直接相关。

所以：

**channel 本身不是倍率表，但它决定了倍率应用的运行上下文。**

### channel 和通道用量统计的关系

相比“定价”，channel 和“通道维度统计”之间的关系更直接。

请求成功并完成结算后，系统会把本次消耗计入当前 channel，例如：

- `model.UpdateChannelUsedQuota(relayInfo.ChannelId, summary.Quota)`

这意味着 channel 同时也是：

- 运维统计维度
- 供应商成本归属维度
- 渠道健康度与消耗排行维度

换句话说，relay 里的 channel 既是“路由目标”，也是“计费归属与统计归属对象”。

### 一个更准确的理解方式

如果用一句更容易落地的话概括：

> 在 `relay` 里，channel 指的是“具体上游通道配置”，负责承载选路、认证、模型映射和请求发送；价格与倍率主要由模型/分组/计费配置决定，但它们会在选中的 channel 上下文中生效，并在结算后把消耗归集到该 channel。

## `relay` 的独立性与对 `service` / `model` 的依赖

如果从“能不能把 `relay` 单独抽出去复用”这个角度看，结论是：

**`relay` 的独立性中等偏低，它不是一个纯协议转发库，而是项目中的业务化网关核心。**

### `relay` 重度依赖 `service` 层

`relay` 在主流程中并不是只做请求转发，它大量依赖 `service` 层来完成业务动作，例如：

- 预扣费 `PreConsumeBilling(...)`
- 后结算 `SettleBilling(...)`
- 文本/图片/音频请求完成后的额度结算
- 错误处理与统一包装
- 协议转换辅助
- 异步任务生命周期处理

也就是说，从调用链上看，`relay` 并不是独立完成主要业务，而是把很多关键动作委托给 `service`。

### `service` 层会继续读写数据库

再往下一层看，`service` 并不只是纯内存逻辑，它有相当一部分关键路径会继续调用 `model` 层，从而读写数据库状态。

可以把这条依赖链概括为：

```text
relay -> service -> model -> DB
```

例如在文本请求结算完成后，会更新：

- 用户已使用额度
- channel 已使用额度

这类逻辑会通过 `model.UpdateUserUsedQuotaAndRequestCount(...)`、`model.UpdateChannelUsedQuota(...)` 落到数据库。

因此，`relay` 虽然不总是直接操作数据库，但它**通过 `service` 间接深度依赖数据库业务状态**。

### 不是所有 `service` 调用都会查库

这里也要避免说得过头。

`relay` 调用的 `service` 函数中，并不是每一个都会访问数据库。有一些只是做：

- 请求/响应格式转换
- token 估算
- 错误映射
- usage 计算
- 额度计算辅助

所以更准确的说法不是：

> `relay` 一依赖 `service`，就一定在查库或写库

而是：

> `relay` 对 `service` 是重依赖，而 `service` 中有不少关键路径会继续经由 `model` 层读写数据库，因此 `relay` 间接地深度绑定了数据库业务状态。`

### 为什么这会影响独立性

这意味着 `relay` 想要被独立抽离时，最难带走的不是协议转换本身，而是这些强业务耦合部分：

- `RelayInfo` 中的用户、token、subscription、channel、billing 状态
- `helper/price.go` 中的倍率、分组、tiered billing 计算
- `service` 中的预扣费、结算、退款逻辑
- `model` 中的 channel/task/user/log 等数据读写

所以从工程角度看：

- **相对容易抽离**：纯 adaptor、部分请求/响应转换逻辑、部分流式协议处理
- **最难抽离**：计费、额度、任务生命周期、通道统计、订阅结算

### 一个更准确的结论

如果把这点也纳入前面对 `relay` 的总判断，可以再补一句：

> `relay` 不只是“调用上游模型接口”的网络代理层，它本身已经深度嵌入了项目的计费、配额、通道统计、异步任务和持久化体系，因此在当前架构里更像业务核心引擎，而不是可轻量拆分的独立 SDK/库。`

## `relay` 的扣费机理

在这个项目里，`relay` 所说的“扣费”，**主要不是直接扣美元余额，而是扣 quota（额度单位）**。

更准确地说：

- 用户账户扣的是 quota
- token 扣的是 quota
- 订阅扣的是 quota / amount_used
- channel 记录的是 used quota

而美元价格、模型倍率、分组倍率等，主要是**计算最终应该扣多少 quota 的依据**。

### 扣的是 quota，不是直接扣美元

项目内部真正发生增减的，是各种额度字段，而不是某个实时美元余额。

价格如果是按固定价格模式，会先做类似这样的换算：

```text
模型价格 * QuotaPerUnit * 分组倍率 -> quota
```

如果是按倍率模式，则会基于 token 数量和倍率计算 quota。

所以从系统实现上看：

```text
美元价格 / 模型倍率 / 分组倍率
    -> 换算成 quota
    -> 实际扣减 quota
```

### 主机制是“预扣费 + 后结算 + 失败退款”

`relay` 的扣费不是“请求结束时一次性直接扣”，而是一个分阶段机制：

1. **请求开始前先预估**
   通过 `ModelPriceHelper(...)` 或对应 task 价格 helper，算出本次需要预扣的 `QuotaToPreConsume`。

2. **请求发送前预扣**
   通过 `PreConsumeBilling(...)` 创建 `BillingSession`，先预扣 token / wallet / subscription 对应额度。

3. **请求成功后按实际消耗结算**
   通过 `SettleBilling(...)` 根据实际 quota 和预扣 quota 的差值做补扣或返还。

4. **请求失败时退款**
   如果调用失败或中途中断，会走 `Refund(...)` 把预扣额度退回。

因此它的计费模型本质是：

```text
预扣 quota -> 根据实际消耗修正 -> 失败则退回
```

### Usage 在结算里非常重要，但不是唯一来源

对于文本、音频、部分图片等主同步请求，最终结算**大多数情况下主要依赖上游返回的 `usage` 信息**。

例如文本主流程最后会进入：

- `PostTextConsumeQuota(...)`

它内部会调用：

- `calculateTextQuotaSummary(ctx, relayInfo, usage)`

再基于这些字段计算最终 quota：

- `usage.PromptTokens`
- `usage.CompletionTokens`
- cache tokens
- audio tokens
- image tokens
- 以及额外工具调用信息

所以如果简单概括，可以说：

> 文本类 relay 的最终扣费，主要是拦截并解析每次 API 调用返回的 usage，再按模型/分组/倍率规则换算成 quota。

### 但它不是“完全只靠 usage”

这里也不能说得太绝对，因为项目已经考虑了很多上游“不返回 usage”或“返回不完整 usage”的情况。

当 usage 不完整时，系统会用一些回退策略，例如：

- 使用估算出的 prompt tokens
- 从响应文本反推 completion tokens
- 某些 provider 自行补 usage
- 某些图片或音频场景设置保底值

所以更准确的说法是：

> relay 的扣费主要依赖 usage，但不是完全依赖；当上游不给 usage 或 usage 不完整时，会用估算或 provider-specific fallback 补齐结算依据。

### 还有一些场景不完全按标准 usage 计费

除了常规文本 usage 计费外，项目里还有几类特殊结算路径：

- **按次计费模型**
  不完全按 token usage，而是按固定价格或固定倍率直接换算 quota。

- **异步 task**
  视频、音乐等异步任务通常先按请求参数预扣，等任务提交成功或终态后再调整。

- **额外工具/功能计费**
  某些场景会叠加 web search、file search、image generation call、audio input 等额外费用。

这说明 `relay` 的计费逻辑并不是“读取一个 usage.total_tokens 然后乘倍率”那么简单，而是一个多来源、多模式的统一结算框架。

### 一个更准确的总结

如果把这部分也浓缩成一句话：

> `relay` 的“扣费”本质上是扣 quota。美元价格、模型倍率、分组倍率、usage 信息和 task 参数，都是为了计算最终应该扣多少 quota；其主机制是“预扣 quota -> 根据 usage/结果结算 -> 失败退款”。`

## 预扣费是怎么计算的

`relay` 的预扣费**不是根据前几次实际扣费取平均值**，而是基于**本次请求的预计消耗即时计算**。

也就是说，它不是历史统计预测模型，而是一次请求一次估算。

### 常规按倍率模型的预扣逻辑

对于大多数按倍率计费的文本请求，预扣费的核心思路是：

1. 先估算本次请求的 prompt tokens
2. 取一个最小预扣下限 `PreConsumedQuota`
3. 如果请求里显式设置了 `max_tokens` / `max_completion_tokens`，则把这部分潜在输出也计入预估
4. 再乘以模型倍率和分组倍率，得到最终预扣 quota

可以抽象成：

```text
preConsumedTokens = max(估算 promptTokens, 最小预扣 token 下限)
if 请求设置了 maxTokens:
    preConsumedTokens += maxTokens

preConsumedQuota = preConsumedTokens * modelRatio * groupRatio
```

所以它本质上是在预估：

- 输入大概会消耗多少
- 输出最多可能消耗多少

然后先把这部分 quota 锁住。

### 固定价格模型的预扣逻辑

如果模型走的是固定价格模式，而不是倍率模式，那么预扣就不再依赖 token 估算，而是直接按价格换算：

```text
preConsumedQuota = modelPrice * QuotaPerUnit * groupRatio
```

这种情况下，本次调用在发送前就会按“单次价格”先预扣对应 quota。

### Tiered Billing / 表达式计费也不是按历史平均

如果模型启用了 tiered expression billing，预扣也不是看过去几次平均值，而是用：

- 当前请求的 prompt token 估算
- 当前请求的 estimated completion tokens
- 当前模型绑定的 billing expression

来实时计算本次预扣额度。

因此，哪怕是更复杂的计费模式，它依然是“**当前请求驱动的即时预估**”，不是“**历史消费驱动的平均预估**”。

### 异步 Task 的预扣逻辑

对于视频、音乐、任务型生成等异步请求，预扣方式也不是基于历史平均，而是基于**本次任务参数**估算。

常见估算维度包括：

- 时长
- 分辨率
- 图片/视频输入
- 特定 provider 的额外倍率参数

这些通常通过各个 task adaptor 的：

- `EstimateBilling(...)`

返回额外倍率，再参与最终预扣计算。

### 为什么不用“历史平均法”

从当前实现看，系统更关心的是：

- 本次请求的模型
- 本次请求的大小
- 本次请求允许生成的最大输出
- 本次请求的业务参数（如视频时长、图像尺寸）

因为这些信息对“这一次到底可能花多少”更直接、更可控。

所以项目采用的是：

- **静态配置 + 当前请求参数 + token 估算**

而不是：

- **过去 N 次调用均值**

### 一个更准确的总结

如果把这部分再压缩成一句话：

> `relay` 的预扣费主要基于“本次请求的估算 prompt token + max_tokens + 模型价格/倍率 + 分组倍率 + 特殊参数倍率”即时计算，而不是基于过去几次调用的平均扣费。`

## `relay` 是如何调用上游 LLM API 的

从调用链上看，`relay` 调上游 API 的主路径是：

```text
请求进入 relay
  -> 选中具体 channel
  -> 把 channel 信息装入 RelayInfo.ChannelMeta
  -> 由具体 adaptor 生成 URL / Header / Body
  -> 调用 DoApiRequest / DoFormRequest / DoWssRequest 发往上游
```

也就是说，真正发请求时并不是由 controller 或 helper 直接拼接 HTTP 请求，而是统一交给 adaptor。

### 上游请求的统一发送入口

普通同步 HTTP 请求最终会走：

- `channel.DoApiRequest(...)`

这个函数会按顺序执行：

1. `a.GetRequestURL(info)` 生成目标 URL
2. `a.SetupRequestHeader(c, &headers, info)` 设置鉴权头和公共头
3. 应用 channel header override
4. 调用底层 `doRequest(...)` 发给上游

这说明每个 provider 的差异，主要收敛在 adaptor 的：

- `GetRequestURL(...)`
- `SetupRequestHeader(...)`
- `Convert*Request(...)`

而不是分散在各个 handler 里。

### 凭证来源：大多数来自预先配置好的 channel key

当前请求命中的上游凭证，通常会先写入：

- `RelayInfo.ChannelMeta.ApiKey`

这个值来自当前选中的 channel 配置，而不是在请求现场临时人工传入。

因此，从总体设计上看：

```text
channel 配置中的凭证
  -> 写入 RelayInfo.ApiKey
  -> adaptor 用它构造上游鉴权
```

### 最常见模式：直接使用预配置 API Key

这是项目里最普遍的方式。

常见例子：

- OpenAI / 大多数兼容 OpenAI 的渠道：`Authorization: Bearer <ApiKey>`
- Azure OpenAI：`api-key: <ApiKey>`
- Claude：`x-api-key: <ApiKey>`
- Gemini / PaLM：`x-goog-api-key: <ApiKey>`

所以如果只看主流 provider，答案可以近似理解为：

> 大多数 relay 调上游时，确实就是直接使用预先配置好的 key。

### 不是所有渠道都用“静态 API Key 直传”

不过，项目里并不只有这一种鉴权模式。至少还能再分出三类。

### 类型 1：预存 OAuth access token，然后直接拿来调用

典型例子是 `codex` 渠道。

它的 `key` 不是普通字符串，而是一个 JSON，对象里会保存：

- `access_token`
- `refresh_token`
- `account_id`

请求发出时，adaptor 会先解析这段 JSON，然后直接把：

- `Authorization: Bearer <access_token>`
- `chatgpt-account-id: <account_id>`

写进上游请求头。

这说明它不是传统 API key，而是**把 OAuth 凭证本身当成 channel key 存储**。

需要注意的是：这里 relay 请求主链路本身并不会在每次转发时现场走一遍 OAuth 登录，而是直接消费已经存下来的 token。

### 类型 2：用预配置凭证，动态向上游换临时 access token

这类渠道也不少，典型有：

- Vertex AI
- 百度文心

#### Vertex AI

Vertex 的一种模式不是直接使用 API key，而是把 Google service account JSON 作为 channel key。

请求发出前，代码会：

1. 用服务账号私钥本地签一个 JWT
2. 调 Google OAuth token endpoint
3. 换取临时 `access_token`
4. 缓存这个 token
5. 再用 `Authorization: Bearer <access_token>` 调真正模型接口

因此它不是“直接保存一个永久 API key 再直传”，而是：

```text
预配置服务账号凭证 -> 动态换 access token -> 调上游
```

#### 百度文心

百度这边，`ApiKey` 实际是：

```text
client_id|client_secret
```

代码会先请求百度 OAuth 接口换取 `access_token`，缓存后再把这个 token 拼到上游请求 URL 上。

所以它同样属于：

```text
预配置原始凭证 -> 动态换 access token -> 调上游
```

### 类型 3：不换 access token，而是在本地签名/JWT 后调用

还有一些 provider 不是 OAuth，也不是静态 key 直传，而是要求客户端基于已配置密钥自行签名。

典型例子是：

- 智谱 `zhipu`

它会把配置的 `id.secret` 拆开，然后：

1. 组装 claims
2. 本地签出一个 JWT/token
3. 直接把这个签名结果放进 `Authorization`

这类模式的本质是：

```text
预配置原始密钥 -> 本地生成签名凭证 -> 调上游
```

和“动态向上游申请 access token”也不完全一样。

### `codex` 还有凭证刷新机制，但不是“每次请求都去申请新 key”

项目里还有一套 `codex` 凭证刷新逻辑，会：

- 读取 channel 中存储的 `refresh_token`
- 调刷新接口拿新的 `access_token` / `refresh_token`
- 把更新后的凭证写回数据库中的 channel `key`

这说明系统支持**后台维护 OAuth 凭证有效性**。

但它和“relay 每次转发时都动态向上游申请一个新的永久 key”不是一回事。

更准确地说，它是：

- 请求路径消费已保存的 OAuth token
- 后台或服务层按需要刷新该 token

### 所以到底是不是“全部依赖预先配置好的 KEY”

最准确的结论是：

- **是的，绝大多数上游调用都依赖预先配置在 channel 中的凭证**
- 但这个“凭证”不一定是狭义 API key
- 它可能是：
  - 静态 API key
  - OAuth access token / refresh token 组合
  - service account 凭证
  - `client_id|client_secret`
  - 用于本地签名的密钥对或组合密钥

所以如果要用一句话概括：

> `relay` 调上游时，本质上总是依赖“预先配置在 channel 中的凭证材料”；但不同 provider 会把这些材料用成静态 key、OAuth token、动态换取的临时 access token，或本地签名后的鉴权头。系统通常不会在请求现场去申请一个新的永久 API key。`

## 关键参考文件

- `controller/relay.go`
- `relay/channel/adapter.go`
- `relay/compatible_handler.go`
- `relay/claude_handler.go`
- `relay/gemini_handler.go`
- `relay/channel/openai/adaptor.go`
- `relay/channel/claude/adaptor.go`
- `relay/channel/gemini/adaptor.go`
- `relay/channel/openai/relay-openai.go`
- `relay/channel/openai/helper.go`
- `relay/channel/gemini/relay-gemini.go`
- `service/convert.go`
- `relay/channel/api_request.go`
- `relay/common/relay_info.go`
- `relay/channel/vertex/service_account.go`
- `relay/channel/vertex/adaptor.go`
- `relay/channel/baidu/adaptor.go`
- `relay/channel/baidu/relay-baidu.go`
- `relay/channel/codex/adaptor.go`
- `service/codex_credential_refresh.go`
- `relay/channel/zhipu/relay-zhipu.go`

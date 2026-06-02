# 14 - 模型列表接口与价格目录关系图

## 为什么补这一节

在这个项目里，和“模型列表”相关的接口至少有三类：

1. `/v1/models`
2. `/api/pricing`
3. `models` 元数据管理接口

它们名字都和“模型”有关，但职责并不一样。

最容易混淆的点通常是：

- `/v1/models` 返回的是不是 `models` 表里的所有模型
- `/v1/models` 和价格页展示的模型是不是同一套
- `abilities`、`models`、`pricing` 三者谁是主数据源

这一节只回答这几个问题，并把关键链路画成一张图。

如果你还想继续往下追两类相邻问题，可以配合阅读：

- `research/project-architecture-05-channel-routing.md`
  这一篇更偏运行时视角，重点解释 `group + model -> abilities -> channel` 的分发与重试。
- `research/project-architecture-13-model-metadata-pricing-and-request-sequence.md`
  这一篇更偏元数据、供应商、价格来源和请求时序的整体串联。

## 一句话结论

最简化地说：

- `/v1/models` 关注“当前 token / 用户实际可调用哪些模型”
- `/api/pricing` 关注“当前用户可以看到哪些模型目录和价格”
- `models` 表不是 `/v1/models` 的主来源，它更像是模型元数据和展示增强层

## 核心关系图

```text
                           +------------------+
                           | channels.models  |
                           | 渠道配置的模型列表 |
                           +---------+--------+
                                     |
                                     | AddAbilities / UpdateAbilities
                                     v
                           +------------------+
                           |    abilities     |
                           | group + model    |
                           | + channel_id     |
                           | + enabled        |
                           +----+---------+---+
                                |         |
                                |         |
                 /v1/models     |         |      /api/pricing
                                |         |
                                v         v
                    +----------------+   +----------------------+
                    | ListModels     |   | GetPricing           |
                    | controller     |   | controller           |
                    +--------+-------+   +----------+-----------+
                             |                      |
                             |                      |
         当前 token / 用户分组过滤                    基于 abilities 汇总启用模型
         token model limit 过滤                     再按 usable groups 过滤
         默认要求有计费配置                         补充价格、vendor、描述、端点
                             |                      |
                             v                      v
                 +---------------------+   +--------------------------+
                 | /v1/models 返回结果 |   | /api/pricing 返回结果     |
                 | 当前实际可调用模型   |   | 当前可展示的模型价格目录 |
                 +---------------------+   +--------------------------+
```

## 更完整的数据来源图

```text
+------------------+
| channels 表      |
| - group          |
| - models         |
| - status         |
| - type           |
+---------+--------+
          |
          | 生成 / 更新能力
          v
+------------------+
| abilities 表     |
| - group          |
| - model          |
| - channel_id     |
| - enabled        |
| - priority       |
| - weight         |
+----+---------+---+
     |         |
     |         +----------------------------------+
     |                                            |
     v                                            v
+---------------------------+          +------------------------------+
| model.GetGroupEnabled...  |          | model.GetAllEnableAbility... |
+------------+--------------+          +---------------+--------------+
             |                                             |
             v                                             v
+---------------------------+                  +-----------------------+
| controller.ListModels     |                  | model.updatePricing() |
| /v1/models                |                  | 生成 pricingMap       |
+------------+--------------+                  +-----------+-----------+
             |                                             |
             | 用当前 token / user group 过滤               | abilities 里出现的模型名
             | token 限模过滤                               | + models 表元数据补充
             | 默认要求 HasModelBillingConfig              | + ratio_setting
             |                                             | + billing_setting
             v                                             |
+---------------------------+                              |
| OpenAI / Anthropic /      |                              |
| Gemini 兼容格式模型列表   |                              |
+---------------------------+                              |
                                                            v
                                            +------------------------------+
                                            | pricingMap                   |
                                            | - model_name                 |
                                            | - enable_groups              |
                                            | - vendor / icon / tags       |
                                            | - supported_endpoint_types   |
                                            | - model_price / model_ratio  |
                                            | - billing_expr               |
                                            +---------------+--------------+
                                                            |
                                                            | controller.GetPricing
                                                            | /api/pricing
                                                            | 按 usable_group 再过滤
                                                            v
                                            +------------------------------+
                                            | 前端价格页 /pricing          |
                                            | 展示模型目录与价格            |
                                            +------------------------------+
```

## `/v1/models` 到底返回什么

`/v1/models` 走的是 relay 路由，不是后台模型元数据接口。

它的入口在：

- `router/relay-router.go`
- `controller.ListModels`

它的返回逻辑可以概括成：

```text
当前请求 token / 用户
  -> 解析可用 group
  -> 从 abilities 里找该 group 已启用模型
  -> 如果 token 开了模型白名单，则按 token 白名单收缩
  -> 默认过滤掉“没有计费配置”的模型
  -> 组装成 OpenAI / Anthropic / Gemini 兼容返回格式
```

所以它不是：

```text
models 表里的所有模型
```

而是：

```text
当前 token / 用户在当前分组下实际可用的模型列表
```

### 更贴近代码的一句话理解

如果只想快速判断它的主来源，可以直接记成：

```text
/v1/models 主要是从 abilities 中，
取出这个 token 当前可用 group 下的 model 列表
```

这句话基本是对的，但要补两个边界条件：

1. 不一定只看 `token.Group`
2. 不一定把 `abilities` 里的模型原样全部返回

### 为什么说“不一定只看 `token.Group`”

如果 `token.Group` 是普通分组，那么理解成：

```text
token.group -> abilities[group] -> models
```

基本没问题。

但如果 `token.Group == auto`，系统不会只看一个分组，而是会展开为当前用户可用的一组自动分组，然后把这些分组下的模型做并集。

所以更准确的表达应该是：

```text
token / user context
  -> 解析当前可用 group 或 auto groups
  -> 从 abilities 中取这些 group 下 enabled 的 model
```

### 为什么说“不是原样全部返回”

即使模型已经存在于 `abilities` 中，也可能在 `/v1/models` 阶段被继续过滤掉。

常见的二次过滤包括：

1. `token` 开启了 `ModelLimits`
2. 当前系统不接受“未配置计费”的模型

也就是说，实际过程更接近：

```text
abilities 中当前 group 可用模型
  -> token model limit 过滤
  -> 计费配置过滤
  -> /v1/models 最终返回
```

### 最准确的记忆方式

如果你想用一句话既保留直觉、又尽量不失真，可以记成：

```text
/v1/models 主要是从 abilities 中获取当前 token / 用户上下文下可用 group 的模型，
再经过 token 限模和计费配置过滤后返回
```

## `/api/pricing` 到底返回什么

`/api/pricing` 的职责不是“返回当前请求能调用的模型”，而是“返回当前用户可见的价格目录”。

它的主要链路是：

```text
abilities 中所有 enabled 模型
  -> model.updatePricing() 聚合
  -> 结合 models 表补充元数据
  -> 结合 ratio / billing 配置补充价格
  -> GetPricing 按 usable_group 做展示过滤
  -> 返回前端价格页
```

因此它更接近：

```text
面向展示和价格说明的模型目录
```

而不是：

```text
一次具体请求下的最终可调用模型集合
```

## `models` 表扮演什么角色

`models` 表很重要，但它不是 `/v1/models` 的主数据源。

更准确地说，它承担的是这些职责：

- 模型描述、图标、标签、供应商归属
- 模型状态控制
- 模型端点定义覆盖
- 规则模型的批量元数据匹配

因此可以把它理解为：

```text
模型元数据层 / 展示增强层
```

而不是：

```text
所有运行时模型的唯一事实来源
```

## 三者的边界

把 `abilities`、`models`、`pricing` 拆开看，会更清楚：

### 1. `abilities`

它解决的是：

- 某个 group 下有哪些模型可用
- 某个模型能走哪些渠道
- 请求分发时有哪些候选渠道

它更偏运行时能力层。

### 2. `models`

它解决的是：

- 这个模型展示成什么样
- 属于哪个 vendor
- 有哪些描述、标签、图标、端点定义
- 是否通过规则匹配覆盖一批模型变体

它更偏元数据与展示层。

### 3. `pricing`

它解决的是：

- 这个模型怎么计费
- 按 token 还是按次计费
- 是否有 cache / image / audio 扩展价格
- 是否使用 tiered expression

它更偏计费与目录聚合层。

## 两个最常见的误解

### 误解 1：`/v1/models` 就是 `models` 表的所有模型

不对。

`/v1/models` 的主来源是 `abilities`，不是 `models` 表。

### 误解 2：价格页模型和 `/v1/models` 永远完全相同

也不对。

两者高度相关，但不保证完全一致，因为：

- `/v1/models` 带有当前 token / 用户视角
- `/api/pricing` 带有价格目录和展示视角
- 两者的过滤条件不完全相同

## 最终记忆方法

如果只记一句话，可以记成：

```text
channels.models -> abilities -> /v1/models
abilities + models(meta) + pricing config -> /api/pricing
```

这基本就是这几个接口之间最重要的边界。

## 延伸阅读

如果你准备继续追代码，推荐按下面顺序读：

1. `research/project-architecture-14-model-list-and-pricing-interfaces.md`
2. `research/project-architecture-05-channel-routing.md`
3. `research/project-architecture-13-model-metadata-pricing-and-request-sequence.md`

这样会比较容易把：

- 模型列表接口
- 运行时选路
- 元数据与价格聚合

三条线拼成一个完整脑图。

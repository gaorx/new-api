# 16 - 内置模型常量与分布位置

## 为什么要单独补这一节

在这个项目里，“系统认识哪些模型”并不只来自数据库或上游同步。

还有一大块信息直接写在代码常量里，常见形式包括：

- 各渠道的 `ModelList`
- 某些渠道的“模型名 -> 上游模型 ID”映射
- 默认倍率、默认固定价格、默认补全倍率等内置表
- 少数任务适配器里直接返回的模型列表

如果你在排查“某个模型是不是内置支持”“这个模型名在哪些地方被硬编码了”“为什么 UI/渠道/计费默认认识它”，这一节就是索引。

## 先说结论

这个项目里确实有很多内置模型常量，而且它们分散在几类位置：

1. `relay/channel/**/constants.go` 或 `constant.go`
2. `relay/channel/task/**/constants.go`
3. `setting/ratio_setting/model_ratio.go`
4. `setting/ratio_setting/cache_ratio.go`
5. 个别 adaptor / provider 文件中的模型映射表

但要注意：

- 代码里有 `ModelList`，不代表这个列表一定完整覆盖上游最新模型
- 某个模型不在 `ModelList`，也不一定完全不能转发，具体还要看渠道适配器是否做了强校验
- 计费默认表里的模型集合，和渠道 `ModelList` 并不是同一层概念

## 一、各渠道的内置 `ModelList`

下面这些文件里直接定义了渠道级别的 `ModelList`。

### 1. OpenAI 系

- `OpenAI`：
  [relay/channel/openai/constant.go](/Users/gaorx/Works/my/new-api/relay/channel/openai/constant.go:3)
  典型模型：`gpt-3.5-turbo`、`gpt-4`、`gpt-4o`、`gpt-5`、`gpt-image-1`
- `Claude`：
  [relay/channel/claude/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/claude/constants.go:3)
  典型模型：`claude-3-*`、`claude-sonnet-4*`、`claude-opus-4*`
- `Gemini`：
  [relay/channel/gemini/constant.go](/Users/gaorx/Works/my/new-api/relay/channel/gemini/constant.go:3)
  典型模型：`gemini-2.5-*`、`gemini-3-*`、`gemini-robotics-er-1.5-preview`
- `Cohere`：
  [relay/channel/cohere/constant.go](/Users/gaorx/Works/my/new-api/relay/channel/cohere/constant.go:3)
- `Mistral`：
  [relay/channel/mistral/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/mistral/constants.go:3)
- `Perplexity`：
  [relay/channel/perplexity/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/perplexity/constants.go:3)
- `Moonshot`：
  [relay/channel/moonshot/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/moonshot/constants.go:3)
  典型模型：`kimi-k2.5`、`kimi-k2-thinking`
- `DeepSeek`：
  [relay/channel/deepseek/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/deepseek/constants.go:3)
  典型模型：`deepseek-chat`、`deepseek-reasoner`、`deepseek-v4-*`
- `Codex`：
  [relay/channel/codex/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/codex/constants.go:8)
  这里先定义 `baseModelList`，再派生出带 compact 后缀的 `ModelList`

### 2. 国内 / 聚合渠道

- `VolcEngine`：
  [relay/channel/volcengine/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/volcengine/constants.go:3)
  典型模型：`Doubao-*`、`seedream-*`、`seedance-*`
- `Ali`：
  [relay/channel/ali/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/ali/constants.go:3)
  典型模型：`qwen-turbo`、`qwen-plus`、`qwen-max`
- `Baidu`：
  [relay/channel/baidu/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/baidu/constants.go:3)
- `Baidu v2`：
  [relay/channel/baidu_v2/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/baidu_v2/constants.go:3)
- `Tencent`：
  [relay/channel/tencent/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/tencent/constants.go:3)
- `Xunfei`：
  [relay/channel/xunfei/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/xunfei/constants.go:3)
- `Zhipu`：
  [relay/channel/zhipu/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/zhipu/constants.go:3)
- `Zhipu 4V`：
  [relay/channel/zhipu_4v/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/zhipu_4v/constants.go:3)
- `360`：
  [relay/channel/ai360/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/ai360/constants.go:3)
- `Lingyiwanwu`：
  [relay/channel/lingyiwanwu/constrants.go](/Users/gaorx/Works/my/new-api/relay/channel/lingyiwanwu/constrants.go:5)
- `MiniMax`：
  [relay/channel/minimax/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/minimax/constants.go:5)

### 3. 其他渠道 / 平台型来源

- `Submodel`：
  [relay/channel/submodel/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/submodel/constants.go:3)
  典型模型：`NousResearch/Hermes-*`、`Qwen/*`、`openai/gpt-oss-120b`
- `Cloudflare`：
  [relay/channel/cloudflare/constant.go](/Users/gaorx/Works/my/new-api/relay/channel/cloudflare/constant.go:3)
- `Replicate`：
  [relay/channel/replicate/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/replicate/constants.go:10)
- `Jina`：
  [relay/channel/jina/constant.go](/Users/gaorx/Works/my/new-api/relay/channel/jina/constant.go:3)
- `Xinference`：
  [relay/channel/xinference/constant.go](/Users/gaorx/Works/my/new-api/relay/channel/xinference/constant.go:3)
- `MokaAI`：
  [relay/channel/mokaai/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/mokaai/constants.go:3)
- `Ollama`：
  [relay/channel/ollama/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/ollama/constants.go:3)
- `PaLM`：
  [relay/channel/palm/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/palm/constants.go:3)
- `Coze`：
  [relay/channel/coze/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/coze/constants.go:3)
- `Vertex`：
  [relay/channel/vertex/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/vertex/constants.go:3)
- `Dify`：
  [relay/channel/dify/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/dify/constants.go:1)
  这里声明了 `ModelList`，但当前是空切片。

## 二、视频 / 异步任务渠道的内置模型

这类模型大多不走普通文本 relay，而是走 `task adaptor` 链路。

- `Doubao Video`：
  [relay/channel/task/doubao/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/task/doubao/constants.go:3)
  典型模型：`doubao-seedance-*`
- `Ali Video`：
  [relay/channel/task/ali/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/task/ali/constants.go:3)
  典型模型：`wan2.5-i2v-preview`、`wan2.2-i2v-flash`
- `Hailuo Video`：
  [relay/channel/task/hailuo/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/task/hailuo/constants.go:7)
- `Sora`：
  [relay/channel/task/sora/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/task/sora/constants.go:3)

另有少数任务 adaptor 不是在 `constants.go` 里放模型，而是直接在方法里返回：

- `Kling`：
  [relay/channel/task/kling/adaptor.go](/Users/gaorx/Works/my/new-api/relay/channel/task/kling/adaptor.go:255)
- `Vidu`：
  [relay/channel/task/vidu/adaptor.go](/Users/gaorx/Works/my/new-api/relay/channel/task/vidu/adaptor.go:216)
- `Jimeng`：
  [relay/channel/task/jimeng/adaptor.go](/Users/gaorx/Works/my/new-api/relay/channel/task/jimeng/adaptor.go:264)

## 三、默认计费里也内置了大量模型名

如果你关注的是“模型默认倍率 / 默认价格是在哪里写死的”，核心不在渠道目录，而在设置目录。

### 1. 默认模型倍率

- 文件：
  [setting/ratio_setting/model_ratio.go](/Users/gaorx/Works/my/new-api/setting/ratio_setting/model_ratio.go:26)
- 结构：
  `defaultModelRatio map[string]float64`
- 典型内容：
  `gpt-*`、`claude-*`、`gemini-*`、`deepseek-*`、`qwen-*`、`glm-*`

### 2. 默认固定价格

- 文件：
  [setting/ratio_setting/model_ratio.go](/Users/gaorx/Works/my/new-api/setting/ratio_setting/model_ratio.go:279)
- 结构：
  `defaultModelPrice map[string]float64`
- 这类常见于：
  图片、音频、任务型模型、部分按次计费模型

### 3. 默认补全倍率 / 图像倍率 / 音频倍率

- `defaultCompletionRatio`：
  [setting/ratio_setting/model_ratio.go](/Users/gaorx/Works/my/new-api/setting/ratio_setting/model_ratio.go:335)
- `defaultAudioCompletionRatio`：
  [setting/ratio_setting/model_ratio.go](/Users/gaorx/Works/my/new-api/setting/ratio_setting/model_ratio.go:321)
- `defaultImageRatio`：
  [setting/ratio_setting/model_ratio.go](/Users/gaorx/Works/my/new-api/setting/ratio_setting/model_ratio.go:661)
- `defaultAudioRatio`：
  同文件更前面的默认表定义

### 4. 默认缓存倍率

- `defaultCacheRatio`：
  [setting/ratio_setting/cache_ratio.go](/Users/gaorx/Works/my/new-api/setting/ratio_setting/cache_ratio.go:7)
- `defaultCreateCacheRatio`：
  [setting/ratio_setting/cache_ratio.go](/Users/gaorx/Works/my/new-api/setting/ratio_setting/cache_ratio.go:76)

## 四、Provider 专用模型映射

有些 provider 不只是列出模型名，还维护“平台模型名 -> 上游实际模型 ID”的映射。

### 1. AWS Bedrock

- 文件：
  [relay/channel/aws/constants.go](/Users/gaorx/Works/my/new-api/relay/channel/aws/constants.go:5)
- 结构：
  `awsModelIDMap`
- 例子：
  `claude-3-5-sonnet-20241022 -> anthropic.claude-3-5-sonnet-20241022-v2:0`

### 2. Vertex Claude 映射

- 文件：
  [relay/channel/vertex/adaptor.go](/Users/gaorx/Works/my/new-api/relay/channel/vertex/adaptor.go:34)
- 例子：
  `claude-3-5-sonnet-20241022 -> claude-3-5-sonnet-v2@20241022`

## 五、这些常量的作用边界

这些内置模型常量通常会影响下面几类行为：

- 某个渠道对外暴露的默认支持模型列表
- 某些前端或 API 返回时的模型展示集合
- 默认计费倍率、默认价格、默认补全倍率
- 某些 provider 的模型名转换

但它们不一定直接决定：

- 数据库里最终存了哪些模型元数据
- 用户实际能否调用某个模型
- 某个模型是否已经被管理员配置为可路由

也就是说，更准确的理解应该是：

```text
内置模型常量
  = 代码内的默认认知 / 默认映射 / 默认价格基础
不等于
  运行时最终可用模型全集
```

## 六、从哪个入口继续看

如果你想继续追不同维度，可以接着看：

- 模型元数据、价格和请求时序：
  [project-architecture-13-model-metadata-pricing-and-request-sequence.md](/Users/gaorx/Works/my/new-api/research/project-architecture-13-model-metadata-pricing-and-request-sequence.md:1)
- 模型列表与定价接口：
  [project-architecture-14-model-list-and-pricing-interfaces.md](/Users/gaorx/Works/my/new-api/research/project-architecture-14-model-list-and-pricing-interfaces.md:1)
- 计费倍率与 quota 换算：
  [project-architecture-06-billing-system.md](/Users/gaorx/Works/my/new-api/research/project-architecture-06-billing-system.md:1)

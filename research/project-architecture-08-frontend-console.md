# 08 - 前端控制台架构

## 技术栈

新版前端在 `web/default/`，技术栈是 React 19 + TanStack Router + React Query + Base UI + Tailwind。

项目目前同时保留两套前端：

- `web/default/`：当前主前端
- `web/classic/`：经典前端/兼容旧界面

可以把整体前端栈概括为：

- 主前端：`React 19 + TypeScript + Rsbuild + TanStack Router/Query + Base UI + Tailwind v4`
- 经典前端：`React 18 + Vite + react-router-dom + Semi UI + Tailwind v3`

## 前端技术栈纵览表

| 层次 | `web/default` 主前端 | `web/classic` 经典前端 |
|---|---|---|
| 定位 | 当前主用前端 | 旧版/兼容前端 |
| 核心框架 | `react` 19、`react-dom` 19 | `react` 18、`react-dom` 18 |
| 语言 | `TypeScript` | 以 `JS/JSX` 为主，带 `TypeScript` 依赖 |
| 构建工具 | `rsbuild`、`@rsbuild/plugin-react` | `vite`、`@vitejs/plugin-react` |
| 路由 | `@tanstack/react-router` | `react-router-dom` |
| 数据请求 | `axios` | `axios` |
| 服务端状态 | `@tanstack/react-query` | 主要依赖页面与 Hook 自行管理 |
| 本地状态 | `zustand` | React Context + Hook |
| UI 组件基础 | `@base-ui/react` | `@douyinfe/semi-ui`、`@douyinfe/semi-icons` |
| 样式体系 | `tailwindcss` v4、`tailwind-merge`、`tw-animate-css` | `tailwindcss` v3 + Semi UI 风格体系 |
| 表单 | `react-hook-form`、`@hookform/resolvers` | 主要依赖组件和自定义逻辑 |
| 校验 | `zod` | 未看到统一 schema 校验主栈 |
| 表格 | `@tanstack/react-table` | 主要依赖页面组件封装 |
| 虚拟滚动 | `@tanstack/react-virtual` | 未见统一虚拟列表主库 |
| 动画 | `motion` | 少量组件级效果，未见统一动画主库 |
| 图表 | `recharts`、`@visactor/react-vchart`、`@visactor/vchart` | `@visactor/react-vchart`、`@visactor/vchart`、`@visactor/vchart-semi-theme` |
| 国际化 | `i18next`、`react-i18next`、`i18next-browser-languagedetector` | 同样使用 `i18next` 方案 |
| Markdown/富文本 | `react-markdown`、`remark-gfm`、`rehype-raw`、`shiki` | `react-markdown`、`marked`、`remark-gfm`、`remark-math`、`rehype-katex`、`rehype-highlight`、`katex`、`mermaid` |
| 交互组件 | `cmdk`、`sonner`、`vaul` | `react-toastify` 等 |
| 日期处理 | `dayjs`、`date-fns` | `dayjs` |
| 图标 | `lucide-react`、`react-icons`、`@hugeicons/react`、`@lobehub/icons` | `lucide-react`、`react-icons`、`@lobehub/icons`、Semi Icons |
| 其他能力 | `qrcode.react`、`input-otp`、`react-day-picker`、`react-resizable-panels`、`react-top-loading-bar` | `qrcode.react`、`react-dropzone`、`react-turnstile`、`react-telegram-login`、`react-fireworks` |
| 代码规范 | `eslint`、`prettier`、`knip` | `eslint`、`prettier` |
| UI 工程化 | `shadcn` | 无明显对应体系 |

## 分类视角的前端栈

### 当前主栈

- `react` 19
- `typescript`
- `rsbuild`
- `@tanstack/react-router`
- `@tanstack/react-query`
- `@base-ui/react`
- `tailwindcss` v4
- `react-hook-form` + `zod`
- `zustand`

### 数据展示相关

- 表格主力：`@tanstack/react-table`
- 图表主力：`recharts`、`@visactor/react-vchart`
- Markdown/代码展示：`react-markdown`、`shiki`

### 国际化

前端 i18n 统一方案是：

- `i18next`
- `react-i18next`
- `i18next-browser-languagedetector`

支持语言包括：

- `en`
- `zh`
- `fr`
- `ru`
- `ja`
- `vi`

### 经典前端栈

- `react` 18
- `vite`
- `react-router-dom`
- `@douyinfe/semi-ui`
- `tailwindcss` v3

相较于主前端，经典前端更偏传统页面式组织，状态与数据层的统一程度较低。

## 前端入口

主入口在：

- `web/default/src/main.tsx`
- `web/default/src/routes/__root.tsx`
- `web/default/src/routes/_authenticated/route.tsx`

可以看出前端的几个明显特点：

- 路由采用文件路由
- QueryClient 统一管理请求状态
- 认证状态依赖本地 store + `getSelf()` 会话校验
- 系统名称、Logo 等运行时从后端 status 接口加载

## 功能分区

`web/default/src/features/` 基本就是后端能力的镜像：

- `channels`
- `models`
- `usage-logs`
- `users`
- `subscriptions`
- `wallet`
- `playground`
- `system-settings`
- `dashboard`
- `profile`

这说明前端不是按“页面类型”组织，而是按“领域功能”组织。

## 模型部署在前端中的位置

“模型部署”在前端里横跨两个模块：

- `features/system-settings`
  用于配置 `io.net` 部署开关和 API Key
- `features/models`
  用于查看部署列表、创建部署、查看详情/日志、修改配置、延长时长等

这说明它不是单纯的“模型列表”子页面，而是一条完整的后台管理能力链路：

```text
先在系统设置中启用 io.net deployment
  -> 再进入 models/deployments 管理实际部署
  -> 部署成功后可继续同步成渠道供网关使用
```

从产品视角看，这个功能服务的是管理员，而不是普通 API 调用用户。

## 这个前端功能实际管理的是什么

这里的“模型部署”并不是把前后端服务发布上线，而是管理一类外部 GPU 容器实例。

当前实现里：

- 资源侧来自 `io.net`
- 默认内置镜像偏向 `Ollama`
- 管理动作包括选硬件、选地区、估价、创建容器、看容器日志、拿容器 `public_url`

因此这个页面更像“模型运行实例控制台”，而不是“站点部署面板”。

## 前后端的对应关系

有一个很明显的映射：

- 后端 `controller/channel.go` <-> 前端 `features/channels`
- 后端 `controller/user.go` <-> 前端 `features/users`
- 后端 `controller/option.go` + `setting/*` <-> 前端 `features/system-settings`
- 后端 `controller/deployment.go` + `pkg/ionet` <-> 前端 `features/models` + `features/system-settings`
- 后端 `controller/relay.go` <-> 前端 `features/playground`

也就是说，前端控制台本质上是后端概念模型的可视化管理层。

## 这对理解项目意味着什么

如果你想理解后端某个能力在产品上怎么被使用，通常可以直接去 `web/default/src/features/` 找对应模块。

这会比从路由层盲查更快。

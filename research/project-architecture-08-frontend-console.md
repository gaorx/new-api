# 08 - 前端控制台架构

## 技术栈

新版前端在 `web/default/`，技术栈是 React 19 + TanStack Router + React Query + Base UI + Tailwind。

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

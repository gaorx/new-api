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

## 前后端的对应关系

有一个很明显的映射：

- 后端 `controller/channel.go` <-> 前端 `features/channels`
- 后端 `controller/user.go` <-> 前端 `features/users`
- 后端 `controller/option.go` + `setting/*` <-> 前端 `features/system-settings`
- 后端 `controller/relay.go` <-> 前端 `features/playground`

也就是说，前端控制台本质上是后端概念模型的可视化管理层。

## 这对理解项目意味着什么

如果你想理解后端某个能力在产品上怎么被使用，通常可以直接去 `web/default/src/features/` 找对应模块。

这会比从路由层盲查更快。

# 管理后台：用户管理（admin_users）

## 概述

管理后台（SPA）用户管理页面位于 `/admin/users`（仅 `root` 可访问），提供用户基础信息管理（创建/编辑/重置密码/删除），并支持为指定用户手动增加余额（USD）。

## 权限与入口

- 页面入口（SPA）：`/admin/users`
- API 权限：`/api/admin/*` 由 `router/auth_middleware.go:requireRootSession` 保护（cookie session + `Realms-User` header），仅 `root` 可调用。

## 余额数据

- 数据表：`user_balances`
  - 字段：`usd DECIMAL(20,6)`（按量计费余额/额度）
  - 行不存在时视为 `0`（并在入账时自动初始化）

## 路由与行为

### API（当前）

- `GET /api/admin/users`：列出用户（包含 `balance_usd`）
- `POST /api/admin/users`：创建用户（JSON：`email/username/password/role/groups[]`）
- `PUT /api/admin/users/{user_id}`：更新用户（JSON：`email/role/status/groups[]`；`username` 不可修改）
- `POST /api/admin/users/{user_id}/password`：重置密码（JSON：`password`），并强制登出该用户
- `POST /api/admin/users/{user_id}/balance`：手动加余额（JSON：`amount_usd`、`note`）
- `DELETE /api/admin/users/{user_id}`：删除用户（含关联数据清理）

通用响应：
- `success: true|false`
- `message: string`
- `data: ...`（按接口不同返回）

> 备注：`note` 字段当前为 best-effort 预留（用于未来审计/备注展示），暂不写入审计事件表。

## 相关代码

- SPA：
  - `web/src/pages/admin/UsersPage.tsx`
  - `web/src/api/admin/users.ts`
- API：
  - `router/admin_users_api_routes.go`
  - `router/admin_api_routes.go`
- Store：
  - `internal/store/user_balances.go`

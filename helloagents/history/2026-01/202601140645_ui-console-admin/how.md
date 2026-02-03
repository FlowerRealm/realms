# 技术设计: UI 补全（订阅/模型列表/用户管理/后台入口）

## 技术方案

### 核心技术
- Go `net/http`（`ServeMux` method pattern）
- `html/template`（SSR）
- 复用现有中间件：`SessionAuth` / `RequireRoles` / `CSRF` / `AccessLog`

### 实现要点

1. **Web 控制台导航与页面**
   - 扩展 `internal/web/templates/base.html`：登录后展示导航（控制台/模型列表/订阅用量/管理后台）。
   - 新增模板：
     - `internal/web/templates/models.html`
     - `internal/web/templates/subscription.html`
   - `internal/web/server.go` 新增 handler：
     - `ModelsPage`：从上游拉取 `/v1/models` 并渲染列表。
     - `SubscriptionPage`：读取 `usage_events` 汇总最近 5h/7d/30d 已结算成本并展示。

2. **模型列表获取方式**
   - 复用 `scheduler` 与 `upstream.Executor` 的选择/调用逻辑，避免在 Web 层手写上游选择。
   - 优先选择 `openai_compatible`；若当前分组没有可用 OpenAI 上游，则页面提示“未配置可用 OpenAI 上游，无法展示模型列表”。

3. **管理后台用户/分组管理**
   - `internal/admin/server.go` 新增页面与动作：
     - `GET /admin/users`：用户列表（root 支持 query `group_id` 过滤）。
     - `POST /admin/users`：创建用户（root 可指定 group；group_admin 固定本组）。
     - `POST /admin/users/{user_id}`：更新用户（启用/禁用、角色、分组；带权限与跨组校验）。
     - `GET /admin/groups`：分组列表（root 全量；group_admin 只读本组）。
     - `POST /admin/groups`：创建分组（仅 root）。
   - 新增模板：
     - `internal/admin/templates/users.html`
     - `internal/admin/templates/groups.html`

4. **Codex OAuth 订阅状态展示**
   - 在 `CodexAccounts` handler 中对每个账号读取（解密）`id_token` 并解析 JWT payload（不做签名校验，仅用于展示 claim）。
   - 展示字段：`plan_type`、`chatgpt_subscription_active_start`、`chatgpt_subscription_active_until`（字段缺失则 `-`）。

5. **路由与中间件链调整**
   - `internal/server/app.go` 增加 web 页面路由，并修正 Web 控制台的 POST 动作不应要求管理员角色：
     - 新增 `webChain`（登录即可）与 `webCSRFChain`（登录 + CSRF）。
     - `POST /tokens/new`、`POST /tokens/revoke`、`POST /logout` 使用 `webCSRFChain`。
     - `/admin/*` 保持 `adminChain`（RequireRoles + CSRF）。

## 安全与性能

- 管理动作严格校验：
  - group_admin 只能操作本组用户；
  - 禁止将任意用户提升为 `root`（除 root 自身的操作外）。
- 订阅与模型列表页面不展示任何明文上游凭据；不输出 id_token 全量，仅提取字段。
- 上游 `/v1/models` 拉取增加读取大小限制，避免异常响应占用内存。

## 测试与部署

- 执行 `go test ./...`。
- 手动验证页面：
  - `/dashboard` 导航可见，模型列表/订阅用量可访问。
  - `/admin` 新增入口与页面可访问（root/group_admin），权限限制正确。


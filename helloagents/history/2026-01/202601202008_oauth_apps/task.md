# 任务清单: OAuth Apps（授权码换取 Realms API Token）

目录: `helloagents/plan/202601202008_oauth_apps/`

---

## 1. 数据模型与 store 层
- [√] 1.1 在 `internal/store/migrations/` 新增 OAuth 表迁移（apps/redirect_uris/auth_codes/grants/app_tokens），验证 why.md#核心场景-需求-redirect_uri-安全校验-场景-非白名单回调必须拒绝
- [√] 1.2 在 `internal/store/` 增加 OAuth Apps 与 Auth Code 的 CRUD（含 code_hash、单次消费、TTL），验证 why.md#核心场景-需求-外部应用发起授权并回调成功-场景-外部应用用授权码换取-api-token

## 2. 登录回跳（next）
- [√] 2.1 在 `internal/web/server.go` 为 `GET/POST /login` 增加 `next` 支持（仅允许站内相对路径），验证 why.md#核心场景-需求-外部应用发起授权并回调成功-场景-用户完成登录并同意授权
- [√] 2.2 更新登录模板（`internal/web/templates/login.html`）加入隐藏字段 next，并校验 open redirect 风险

## 3. OAuth Provider 路由与页面
- [√] 3.1 新增 `internal/oauthapps/`（或等价包）实现 `GET/POST /oauth/authorize` 与 `POST /oauth/token`
- [√] 3.2 在 `internal/server/app.go` 注册 OAuth 路由（public + session/csrf 链），并补充错误返回策略（不可信 redirect_uri 时不得回跳）
- [√] 3.3 新增授权同意页模板（`internal/web/templates/oauth_consent.html` 或新模板目录），包含 app 信息、scope 展示与“记住授权”选项

## 4. 管理后台 OAuth Apps 管理
- [√] 4.1 新增管理后台页面与路由：`/admin/oauth-apps*`（创建/编辑/禁用/轮换 secret/维护 redirect_uris）
- [√] 4.2 在 `helloagents/wiki/api.md` 增加 OAuth Provider 与管理路由文档

## 5. 安全检查
- [√] 5.1 执行安全检查（state/redirect_uri/csrf/code 单次使用/敏感信息日志），并在 how.md 的“安全与性能”补充最终策略

## 6. 测试
- [-] 6.1 新增 OAuth 流程测试（含未登录→登录→授权→token→调用 `/v1/models`）
  > 备注: 当前仓库的测试未集成可用的 DB/迁移环境；已补齐关键单元测试（next 回跳、scope/redirect_uri 规范化）并通过 `go test ./...`（含 `-tags no_webchat`）。
- [-] 6.2 新增 redirect_uri 不匹配/授权码复用/过期的失败用例测试
  > 备注: 同上；当前以单元测试覆盖参数规范化与 SessionAuth 跳转语义，端到端流程建议在接入可复用的测试 DB（或 sqlmock）后补齐。
- [√] 6.3 运行 `go test ./...`

# 任务清单: Web 对话功能构建期开关（no_webchat）

目录: `helloagents/plan/202601201727_webchat_build_tag/`

---

## 1. server 路由注册拆分
- [√] 1.1 在 `internal/server/app.go` 中抽取 Web Chat 路由注册点（调用 `registerWebChatRoutes`）
- [√] 1.2 新增 `internal/server/webchat_routes_enabled.go`（`!no_webchat`）注册 `/chat` 与 `/api/chat/*` 路由，验证 why.md#核心场景-需求-可在编译期剔除-web-对话界面-场景-默认构建保持不变
- [√] 1.3 新增 `internal/server/webchat_routes_disabled.go`（`no_webchat`）空实现，验证 why.md#核心场景-需求-可在编译期剔除-web-对话界面-场景-禁用构建完全不编译

## 2. web handler 与模板嵌入拆分
- [√] 2.1 为 `internal/web/chat.go` 增加 `//go:build !no_webchat`，确保禁用构建时不编译该文件
- [√] 2.2 拆分 `templatesFS` 到 build-tag 文件：新增 `internal/web/templates_fs_enabled.go` 与 `internal/web/templates_fs_disabled.go`，禁用构建时不嵌入 `templates/chat.html`
- [√] 2.3 调整 `internal/web/server.go` 的 imports（移除不再需要的 `embed`），并确保模板解析仍正常

## 3. store FeatureState/FeatureGate 统一表达
- [√] 3.1 新增 `internal/store/webchat_build_flag_enabled.go` / `internal/store/webchat_build_flag_disabled.go`（build-tag 常量）
- [√] 3.2 在 `internal/store/features.go` 中，当禁用构建时强制 `WebChatDisabled=true`（隐藏 UI 入口）
- [√] 3.3 在 `internal/store/feature_gate_effective.go` 中，当禁用构建且 key 为 `feature_disable_web_chat` 时强制返回 true（gate 语义一致）

## 4. 文档与构建支持
- [√] 4.1 更新 `README.md`：增加 `-tags no_webchat` 构建说明
- [√] 4.2 更新 `Dockerfile`：支持 `ARG REALMS_BUILD_TAGS`（默认空，传入 `no_webchat` 可剔除 Web 对话）

## 5. 测试
- [√] 5.1 新增 `internal/server/webchat_routes_enabled_test.go`（`!no_webchat`）断言 `GET /chat` 为 302
- [√] 5.2 新增 `internal/server/webchat_routes_disabled_test.go`（`no_webchat`）断言 `GET /chat` 为 404
- [√] 5.3 运行 `go test ./...` 与 `go test -tags no_webchat ./...`

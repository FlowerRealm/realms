# 任务清单: 上游配置增强（OpenAI API Key / Codex OAuth 自动授权）

目录: `helloagents/history/2026-01/202601141229_oauth_upstreams/`

---

## 1. OpenAI 兼容上游
- [√] 1.1 在 `internal/upstream/executor.go` 修复 base_url 带 `/v1` 时的路径拼接（避免 `/v1/v1/*`），验证 why.md#需求-自定义-url--api-key-可用-场景-base_url-带-v1-的-openai-兼容上游仍可正常调用
- [√] 1.2 为上述逻辑补齐单元测试（建议新增 `internal/upstream/executor_test.go`），覆盖带 `/v1` 与不带两类

## 2. Codex OAuth 自动授权
- [√] 2.1 新增 `internal/codexoauth/*`（或等价位置）实现 PKCE/state、授权 URL 生成与 token 交换
- [√] 2.2 在 `internal/admin/server.go` 增加发起授权接口：`POST /admin/endpoints/{endpoint_id}/codex-oauth/start`
- [√] 2.3 在 `cmd/codex/main.go` 启动本机回调监听（默认 127.0.0.1:1455），并实现回调处理：换 token → 解析 id_token → 加密入库
- [√] 2.4 更新 `internal/admin/templates/codex_accounts.html` 增加“发起授权”按钮与提示，验证 why.md#需求-codex-oauth-自动授权闭环-场景-管理后台生成授权链接并自动入库

## 3. 安全检查
- [√] 3.1 执行安全检查（按G9: 输入验证、敏感信息处理、权限控制、EHRB风险规避）

## 4. 文档更新
- [√] 4.1 更新 `helloagents/wiki/api.md` 补齐新增 admin 授权接口与回调说明
- [√] 4.2 更新 `helloagents/CHANGELOG.md`（Unreleased）记录本次上游配置增强

## 5. 测试
- [√] 5.1 运行 `go test ./...`

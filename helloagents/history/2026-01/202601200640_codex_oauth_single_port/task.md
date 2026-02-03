# 任务清单: Codex OAuth 单端口回调

目录: `helloagents/plan/202601200640_codex_oauth_single_port/`

---

## 1. 回调端口策略
- [√] 1.1 在 `internal/server/app.go` 中将 `GET /auth/callback` 注册到主 HTTP Server（不依赖第二个端口）
- [√] 1.2 在 `cmd/realms/main.go` 中调整启动逻辑：仅当 `codex_oauth.callback_listen_addr` 非空且端口与主服务不同，才启动独立回调监听

## 2. 默认配置与示例配置
- [√] 2.1 在 `internal/config/config.go` 中更新 Codex OAuth 默认值：默认不再设置 `callback_listen_addr`，`redirect_uri` 指向主服务端口
- [√] 2.2 更新 `config.example.yaml`：补充“单端口/双端口/粘贴回调 URL”三种使用方式说明
- [√] 2.3 更新 `internal/admin/templates/endpoints.html`：回调 URL 粘贴提示与默认示例对齐新默认端口

## 3. 文档与脚本同步
- [√] 3.1 更新知识库：`helloagents/wiki/api.md`、`helloagents/wiki/modules/realms.md`
- [√] 3.2 更新 `helloagents/CHANGELOG.md`：记录回调端口策略调整
- [√] 3.3 更新 `register.py`：回调 URL 识别不再硬编码 `localhost:1455`

## 4. 安全检查
- [√] 4.1 执行安全检查：确认 OAuth 回调不引入越权/CSRF 风险（仅依赖 session cookie + state 绑定 + root 权限校验）

## 5. 测试
- [√] 5.1 运行 `go test ./...` 验证构建与单测通过

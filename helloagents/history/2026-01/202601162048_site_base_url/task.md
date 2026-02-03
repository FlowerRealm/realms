# 任务清单: 站点地址（Site Base URL）统一

目录: `helloagents/plan/202601162048_site_base_url/`

---

## 1. 配置与设置项
- [√] 1.1 在 `internal/store/app_settings.go` 增加 `site_base_url` 设置项常量
- [√] 1.2 在 `internal/config/config.go` 增加 `NormalizeHTTPBaseURL` 并复用到 `server.public_base_url` 解析

## 2. 管理后台（系统设置 + URL 生成）
- [√] 2.1 在 `internal/admin/templates/settings.html` 增加“站点地址”配置入口（支持保存/恢复默认）
- [√] 2.2 在 `internal/admin/server.go` 增加 settings 读写逻辑，并让 `baseURLFromRequest` 优先使用 `site_base_url`

## 3. Web 控制台（前端展示）
- [√] 3.1 在 `internal/web/server.go` 让 `baseURLFromRequest` 优先使用 `site_base_url`

## 4. 回调回跳地址
- [√] 4.1 在 `internal/codexoauth/flow.go` 生成回跳 URL 时优先使用 `site_base_url`

## 5. 安全检查 / 文档 / 测试
- [√] 5.1 安全检查：站点地址校验仅允许 http/https 且 host 非空
- [√] 5.2 更新文档：`README.md`、`config.example.yaml`、`helloagents/wiki/modules/realms.md`、`helloagents/CHANGELOG.md`
- [√] 5.3 运行测试：`go test ./...`

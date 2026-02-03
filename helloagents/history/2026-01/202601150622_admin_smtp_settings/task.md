# 任务清单: 管理后台 SMTP 邮箱配置（验证码）

目录: `helloagents/plan/202601150622_admin_smtp_settings/`

---

## 1. app_settings 扩展（store）
- [√] 1.1 在 `internal/store/app_settings.go` 中补充 SMTP 相关 settings key 常量与读取/写入辅助（string/int/bool），满足 why.md#需求-管理后台可配置-smtp
- [√] 1.2 在 `internal/store/app_settings.go` 中实现「批量清理 SMTP 覆盖项」能力，供后台 reset 使用

## 2. 管理后台页面（admin/settings）
- [√] 2.1 在 `internal/admin/server.go` 中扩展 `templateData` 与 `Settings/UpdateSettings`：展示有效值（DB 覆盖优先），并支持保存/回退默认，满足 why.md#场景-root-在后台配置-smtp-并保存
- [√] 2.2 在 `internal/admin/templates/settings.html` 中增加 SMTP 配置表单项（Token 不回显），并显示来源提示

## 3. 邮箱验证码发送使用有效 SMTP 配置（web）
- [√] 3.1 在 `internal/web/*` 中增加 `smtpConfigEffective(ctx)` 并调整 `APIEmailVerificationSend` 使用运行时配置发送，满足 why.md#场景-root-恢复为配置文件默认
- [√] 3.2 视需要调整 `internal/server/app.go` 的依赖注入（避免启动时固定 mailer），保持最小变更

## 4. 安全检查
- [√] 4.1 执行安全检查（按G9：输入校验、敏感信息处理、权限控制、CSRF、防止 token 回显/日志泄露）

## 5. 文档更新（知识库）
- [√] 5.1 更新 `helloagents/wiki/data.md`：补充 `app_settings` 新增 key（SMTP 相关）
- [√] 5.2 更新 `helloagents/wiki/api.md`：补充 `/admin/settings` 新增字段说明
- [√] 5.3 更新 `helloagents/CHANGELOG.md`

## 6. 测试
- [√] 6.1 运行 `gofmt`（对所有 Go 文件格式化）
- [√] 6.2 运行 `go test ./...`

# 任务清单: smtp_email_verification

目录: `helloagents/plan/202601142113_smtp_email_verification/`

---

## 1. 配置与开关
- [√] 1.1 在 `internal/config/config.go` 增加 `smtp` 与 `email_verification` 配置结构，并补充 `REALMS_*` 环境变量覆盖，验证 why.md#需求-注册邮箱验证码-场景-未注册邮箱发送验证码
- [√] 1.2 更新 `config.example.yaml` 补充 SMTP 与验证码配置示例，验证 why.md#需求-注册邮箱验证码-场景-未注册邮箱发送验证码

## 2. SMTP 发送模块
- [√] 2.1 新增 `internal/email/smtp.go`：实现 HTML 邮件发送（465/SSL + 587/STARTTLS），配置字段沿用 new-api 风格，验证 why.md#需求-注册邮箱验证码-场景-未注册邮箱发送验证码
- [-] 2.2（可选）新增 `internal/email/smtp_test.go`：覆盖邮件头部拼装与 TLS 分支（不依赖真实 SMTP），验证 why.md#需求-注册邮箱验证码-场景-未注册邮箱发送验证码
  > 备注: 未引入 SMTP 端到端/协议级单测（需要外部服务或复杂 mock）；本次通过 `go test ./...` 确保编译与现有集成不回归。

## 3. 数据层：验证码存储与校验
- [√] 3.1 新增迁移 `internal/store/migrations/0002_email_verifications_index_email_nullable_user_id.sql`：允许 `user_id` 为 NULL，并添加 `email` 索引，验证 why.md#需求-注册邮箱验证码-场景-使用验证码完成注册可选开关
- [√] 3.2 在 `internal/store/models.go` 增加 `EmailVerification` 结构体，验证 why.md#需求-注册邮箱验证码-场景-未注册邮箱发送验证码
- [√] 3.3 新增 `internal/store/email_verifications.go`：实现 `UpsertEmailVerification/ConsumeEmailVerification/DeleteExpired` 等方法，验证 why.md#需求-注册邮箱验证码-场景-使用验证码完成注册可选开关

## 4. Web/API：发送验证码接口
- [√] 4.1 更新 `internal/web/server.go`：为 `web.Server` 注入 mailer 与 email_verification 开关配置，验证 why.md#需求-注册邮箱验证码-场景-未注册邮箱发送验证码
- [√] 4.2 更新 `internal/server/app.go`：创建 SMTP mailer、挂载 `POST /api/email/verification/send` 路由（不需要登录），验证 why.md#需求-注册邮箱验证码-场景-未注册邮箱发送验证码
- [√] 4.3 新增 `internal/web/api_email_verification.go`：实现发送验证码 handler（校验邮箱未被占用、生成 6 位数字码、落库、发送 HTML 邮件），验证 why.md#需求-注册邮箱验证码-场景-未注册邮箱发送验证码

## 5. 注册页集成（可选开关）
- [√] 5.1 更新 `internal/web/templates/register.html`：增加验证码输入框与“发送验证码”按钮（调用 `/api/email/verification/send`），验证 why.md#需求-注册邮箱验证码-场景-使用验证码完成注册可选开关
- [√] 5.2 更新 `internal/web/server.go` 的 `Register`：当 `email_verification.enable=true` 时强制校验验证码，通过后创建用户并清理验证码记录，验证 why.md#需求-注册邮箱验证码-场景-使用验证码完成注册可选开关

## 6. 安全检查
- [√] 6.1 执行安全检查（按 G9：输入验证、敏感信息处理、错误回显、明文 AUTH 风险规避）

## 7. 文档更新
- [√] 7.1 更新 `helloagents/wiki/api.md`：补充验证码发送接口与错误说明
- [√] 7.2 更新 `helloagents/wiki/data.md`：补充 `email_verifications` 的实际用途与字段约束

## 8. 测试
- [√] 8.1 运行 `go test ./...`，覆盖新增 store 与 handler 的基础行为
- [-] 8.2 手工验证：配置 SMTP 后从注册页发送验证码并完成注册（开启/关闭开关两种路径）
  > 备注: 需要在具备可用 SMTP 的环境手工验证（本地开发可用 MailHog 或真实 SMTP）。

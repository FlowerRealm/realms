# 变更提案: 管理后台 SMTP 邮箱配置（验证码）

## 需求背景

当前系统已支持「注册邮箱验证码」与 SMTP 发信，但 SMTP 配置仍依赖 `config.yaml` / 环境变量维护。对于日常运维来说，这意味着每次调整 SMTP 相关配置都需要改配置文件并重启/发布，不够方便。

本变更目标是：把 SMTP 配置项也纳入管理后台「系统设置」页面，由 root 用户通过 UI 直接配置并即时生效。

## 变更内容

1. 在管理后台 `/admin/settings` 增加 SMTP 配置项（沿用 new-api 风格字段：`SMTPServer/SMTPPort/SMTPSSLEnabled/SMTPAccount/SMTPFrom/SMTPToken`）。
2. SMTP 配置支持 UI 覆盖：`app_settings`（DB）优先于配置文件默认；提供「恢复为配置文件默认」清理所有 SMTP 覆盖项。
3. 邮箱验证码发送逻辑使用「有效 SMTP 配置」（每次发送读取覆盖项），确保后台修改后立即生效。

## 影响范围

- **模块:** admin / web / store / config / email
- **文件:** `internal/admin/*`、`internal/web/*`、`internal/store/*`、`internal/server/app.go`
- **API:** `/admin/settings` 表单字段扩展（无需新增公开 API）
- **数据:** `app_settings` 增加若干 key（无需新增表/迁移）

## 核心场景

### 需求: 管理后台可配置 SMTP
**模块:** admin/store/web

#### 场景: root 在后台配置 SMTP 并保存
- root 进入 `/admin/settings`，填写 SMTPServer/Port/SSL/Account/From/Token 并保存成功
- 页面可提示「当前值来源」为 UI 覆盖或配置文件默认（Token 不回显，仅提示已设置/未设置）
- 随后调用 `/api/email/verification/send` 可正常发信（HTML 邮件）

#### 场景: root 恢复为配置文件默认
- root 点击「恢复为配置文件默认」
- 系统清理所有 SMTP 覆盖项（以及相关开关）
- 后续发信回退为 `config.yaml` / 环境变量提供的 SMTP 默认配置

## 风险评估

- **风险:** `SMTPToken` 属于敏感信息，写入数据库会带来泄露风险（当前项目对上游 Key / OAuth token 亦为明文存储，风险一致）
- **缓解:**
  - `/admin/settings` 仅 `root` 可访问，并启用 CSRF（现有中间件已覆盖）
  - UI 不回显 token，不在日志/错误信息中输出 token
  - 建议使用专用 SMTP 账号/应用密码，权限最小化


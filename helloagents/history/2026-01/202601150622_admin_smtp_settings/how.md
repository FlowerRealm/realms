# 技术设计: 管理后台 SMTP 邮箱配置（验证码）

## 技术方案

### 核心技术
- Go `net/http`（现有 SSR 管理后台）
- MySQL `app_settings`（key/value 持久化少量运行期配置）
- SMTP 发信（465 implicit TLS / 587 STARTTLS）

### 实现要点

#### 1) `app_settings` 存储模型（按字段拆分）

采用按字段拆分的 key/value 存储方式，便于「部分覆盖」与「回退默认」：

- `email_verification_enable`（已存在）
- `smtp_server`
- `smtp_port`
- `smtp_ssl_enabled`
- `smtp_account`
- `smtp_from`
- `smtp_token`

读取策略：
- 以配置文件默认（含环境变量覆盖）为 base
- 对每个字段：若 DB 有对应 key，则覆盖 base

写入策略（表单提交）：
- `SMTPServer/SMTPAccount/SMTPFrom`：若为空，则删除对应 key（回退到默认）；否则 upsert
- `SMTPPort`：若为空，则删除对应 key；否则解析为 int 并 upsert
- `SMTPSSLEnabled`：checkbox bool，upsert
- `SMTPToken`：不回显；若表单为空则保持原值不变（避免误清空）；如需清空，通过「恢复为配置文件默认」统一清理

#### 2) 管理后台 UI（/admin/settings）

扩展现有 `internal/admin/templates/settings.html`：
- SMTPServer（text）
- SMTPPort（number）
- SMTPSSLEnabled（switch）
- SMTPAccount（text）
- SMTPFrom（text）
- SMTPToken（password，不回显，显示“已设置/未设置”提示）

同时在页面显示：
- 每个字段的来源（UI 覆盖 / 配置文件默认）
- 「恢复为配置文件默认」会清理所有 SMTP 覆盖 key

#### 3) 发信逻辑改为使用“有效 SMTP 配置”

当前验证码发送直接调用 `s.mailer.SendHTML(...)`，而 mailer 在启动时用配置文件创建，无法感知 UI 的运行期修改。

改造为运行时获取有效 SMTP 配置：
- 在 `web.Server` 增加 `smtpConfigEffective(ctx)`：从 `app_settings` 读取覆盖项并与默认配置合并，返回 `config.SMTPConfig`
- 发送验证码时按需构造 `email.NewSMTPMailer(effectiveCfg)` 并发送（避免 `email` 包依赖 `store`）

## 架构决策 ADR

### ADR-006: SMTP 配置存储方式选择
**上下文:** 需要将 SMTP 配置放入管理后台并可持久化；要求最小侵入，且支持部分字段覆盖与回退默认。
**决策:** 使用 `app_settings` 按字段拆分存储（每个字段一个 key），运行时按字段 merge（DB override > config default）。
**理由:** 最小 schema 变更、实现简单、与现有 `email_verification_enable` 模式一致，便于逐字段回退默认。
**替代方案:** 单 key JSON blob → 不利于逐字段覆盖/回退；新增专用表（多列）→ 需要迁移与更多代码。
**影响:** `app_settings` key 数量增加；需要补充类型解析（int/bool）与表单校验。

## API 设计

不新增公开 API；仅扩展管理后台 `/admin/settings` 的表单字段。

## 数据模型

不新增表；复用 `app_settings(key, value)` 存储上述 keys。

## 安全与性能

- **安全:**
  - 所有设置页访问权限保持 root-only（现有 `RequireRoles(root)`）
  - `SMTPToken` 不回显，不进入日志；错误信息不包含 token
  - 解析 SMTPPort 时做范围校验（1-65535）；SMTPServer/Account/From 做 trim
- **性能:**
  - 验证码发送每次读取若干 `app_settings` key（量小、频率低）；无需额外缓存

## 测试与部署

- **测试:**
  - `go test ./...`
  - 手工回归：后台设置 SMTP → 注册页发送验证码成功；切换/恢复默认后行为符合预期
- **部署:**
  - 数据库已存在 `app_settings` 表即可（已包含迁移 0003）
  - 新增 key 不影响旧版本读取；回滚仅会忽略新增 key


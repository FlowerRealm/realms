# 变更提案: SMTP 邮箱验证码

## 需求背景

Realms 目前提供 Web 注册/登录，但缺少“邮箱验证码”能力；代码库里已预留 `email_verifications` 表（`internal/store/migrations/0001_init.sql`），尚未落地完整流程。

本变更希望：
1. 增加 **SMTP 邮件发送能力**（配置字段沿用 `new-api` 风格：`SMTPServer/SMTPPort/SMTPSSLEnabled/SMTPAccount/SMTPFrom/SMTPToken`）。
2. 提供 **邮箱验证码发送**：6 位数字码，**10 分钟有效**，邮件内容为 **HTML**。
3. 不做频控/风控（按需求明确要求）。

## 变更内容
1. **配置层**：补充 SMTP 与邮箱验证码开关/参数配置（含 env 覆盖）。
2. **邮件发送模块**：新增 SMTP sender（支持 465/SSL 与 587/STARTTLS）。
3. **验证码数据层**：落库保存验证码哈希与过期时间；提供校验与清理能力。
4. **Web/API 接入**：
   - 提供“发送验证码”接口供注册页调用。
   - 注册页可选集成验证码校验（建议用开关避免破坏现有注册流程）。

## 影响范围
- **模块:** `internal/config`、`internal/web`、`internal/store`、（新增）`internal/email`
- **文件:** `config.example.yaml`、`internal/config/config.go`、`internal/server/app.go`、`internal/web/*`、`internal/store/*`、`internal/store/migrations/*`
- **API:** 新增公开接口（用于注册场景的验证码发送/校验）
- **数据:** 使用并完善 `email_verifications` 表的实际用途（可能需要一条新 migration）

## 核心场景

### 需求: 注册邮箱验证码
**模块:** web / store / email

#### 场景: 未注册邮箱发送验证码
用户在注册页输入邮箱，点击“发送验证码”。
- 系统校验邮箱格式，且邮箱未被占用
- 生成 6 位数字验证码（10 分钟有效）
- 写入 `email_verifications`（仅存 hash + expires_at）
- 通过 SMTP 发送 HTML 邮件
- 返回发送结果（前端可提示“已发送”）

#### 场景: 使用验证码完成注册（可选开关）
用户提交注册表单（邮箱 + 密码 + 验证码）。
- 若开启邮箱验证码校验：必须校验通过才能创建用户并登录
- 校验通过后清理验证码记录（或标记 verified_at）
- 若未开启校验：保持现有行为不变（兼容）

## 风险评估
- **滥用风险（明确不做频控）**：公开发送接口可被滥用导致邮件轰炸/资源消耗；本次按需求不做，但需要在文档中明确风险与后续可加的限流点位。
- **凭据泄露风险**：SMTP 账号/Token 为敏感信息；必须仅通过配置注入，避免日志打印与错误回显泄露。
- **传输安全**：需要优先使用 TLS（465/SSL 或 587/STARTTLS）；避免在明文连接上进行 AUTH。


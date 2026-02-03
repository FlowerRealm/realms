# 技术设计: SMTP 邮箱验证码

## 技术方案

### 核心技术
- **邮件发送:** Go 标准库 `net/smtp` + `crypto/tls`
- **验证码安全:** 仅存 `sha256` 哈希（复用 `internal/crypto.TokenHash`）
- **配置:** 读取 `config.yaml`，并支持 `REALMS_*` 环境变量覆盖

### 实现要点
1. **SMTP 配置字段沿用 new-api 风格**
   - `SMTPServer`：SMTP 主机
   - `SMTPPort`：端口（常见 465/587）
   - `SMTPSSLEnabled`：是否使用 implicit TLS（等价于 new-api 的“SSL enabled”）
   - `SMTPAccount`：SMTP 用户名
   - `SMTPFrom`：发件人邮箱（为空则回退到 `SMTPAccount`）
   - `SMTPToken`：SMTP 密码/Token

2. **TLS 策略（比照 new-api，但做安全收敛）**
   - 当 `SMTPPort==465` 或 `SMTPSSLEnabled=true`：使用 `tls.Dial` + `smtp.NewClient`（implicit TLS）。
   - 否则：先 `smtp.Dial`，若服务端支持 `STARTTLS` 则 `StartTLS` 后再 `Auth`。
   - 若既非 implicit TLS 且服务端不支持 STARTTLS：返回错误并提示调整配置（避免明文 AUTH）。

3. **验证码生成**
   - 生成 **6 位数字**（`000000`~`999999`，需补零）。
   - 过期时间：`now + 10m`。
   - 存储：`code_hash=sha256(code)`。

4. **验证码存储与校验（DB）**
   - 发送时：删除该 email 的未验证/已过期记录后插入新记录（保证“最新码唯一有效”）。
   - 校验时：按 email 查询“未过期且未验证”的最新记录，比较 hash；成功则标记 `verified_at` 或直接删除记录。
   - 清理：提供按时间清理过期记录的方法（可在发送/校验路径顺手清理；不引入定时任务也可接受）。

5. **Web/API 接入**
   - 新增公开接口：`POST /api/email/verification/send`
     - 请求：`email=...`（form 或 JSON；实现上选一个即可，建议 form 简化注册页接入）
     - 响应：JSON `{ "sent": true }`；失败使用 `http.Error` 返回可读错误
   - 注册页集成（可选开关）：
     - `email_verification.enable=true` 时注册必须提供 `code`
     - 保持默认关闭，避免破坏现有开放注册体验

## 架构决策 ADR

### ADR-001: 验证码存储采用 DB 而非内存 Map
**上下文:** new-api 采用进程内 Map（或 Redis）保存验证码；Realms 已预留 `email_verifications` 表且为单体部署可自举。  
**决策:** 使用 MySQL `email_verifications` 保存验证码 hash 与过期时间。  
**理由:** 重启不丢失；不引入 Redis；复用既有 schema。  
**替代方案:** 进程内 Map（new-api 风格） → 拒绝原因: 重启丢码、横向扩展困难。  
**影响:** 需要补充 store 方法与（可能的）迁移以支持“按 email 存储”。  

### ADR-002: SMTP 配置字段命名沿用 new-api
**上下文:** 用户明确要求“沿用 new-api 风格字段”。  
**决策:** 在 `smtp:` 配置块内使用 `SMTPServer/SMTPPort/SMTPSSLEnabled/SMTPAccount/SMTPFrom/SMTPToken` 字段。  
**影响:** YAML key 命名与现有 snake_case 略不一致，但可通过块级隔离减少影响。  

## API设计

### [POST] /api/email/verification/send
- **描述:** 向指定邮箱发送 6 位数字验证码（10 分钟有效，HTML 邮件）
- **请求:** `email`（必填，email 格式）
- **响应:**
  - 成功：`{"sent": true}`
  - 失败：HTTP 4xx/5xx + 文本错误

## 数据模型

### email_verifications
现有字段：
```sql
id, user_id, email, code_hash, expires_at, verified_at, created_at
```

建议补充：
- 允许 `user_id` 为 NULL（注册前验证码无需 user_id）
- 增加按 `email` 查询的索引（发送/校验按 email 查）

## 安全与性能
- **安全:**
  - SMTP Token 属敏感信息，禁止日志打印；错误信息避免回显 Token。
  - 仅保存验证码 hash（不落明文码）；校验时对输入做 trim。
  - SMTP AUTH 必须在 TLS 下进行（implicit TLS 或 STARTTLS）。
- **性能:**
  - 发送邮件为外部 IO；建议设置 dial/写入超时（可复用现有 limits 或在 mailer 内部使用固定超时）。
  - DB 操作为单行写入/查询，成本可控。

## 测试与部署
- **测试:**
  - 单元测试：验证码生成（6 位数字、含前导 0）与 store 校验逻辑（可用集成 DB 或最小 mock）。
  - 手工测试：本地配置 SMTP（或使用 MailHog/本地 SMTP 捕获器）验证注册页完整流程。
- **部署:**
  - 配置 `smtp.*` 与 `email_verification.enable` 后重启服务。
  - 首次运行会自动应用新 migration（`store.ApplyMigrations`）。


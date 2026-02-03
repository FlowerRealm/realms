# 任务清单: 工单系统（工单 + 消息线程 + 附件）

目录: `helloagents/plan/202601152028_tickets/`

---

## 1. 数据库与 Store（工单/消息/附件）
- [√] 1.1 新增迁移 `internal/store/migrations/0020_tickets.sql`：创建 `tickets/ticket_messages/ticket_attachments` 表与索引，验证 why.md#核心场景-需求-用户创建工单-场景-创建工单并上传附件
- [√] 1.2 在 `internal/store/models.go` 增加 Ticket/TicketMessage/TicketAttachment 结构体，字段与表一致
- [√] 1.3 在 `internal/store/` 实现 tickets CRUD：
  - 用户侧：按 `user_id` 查询列表/详情、创建工单、追加消息、创建附件记录
  - 管理侧：全量列表/详情、关闭/恢复工单
  - 验证 why.md#需求-用户查看工单 与 why.md#需求-管理员处理工单
- [√] 1.4 实现过期附件查询与删除：`ListExpiredTicketAttachments` + `DeleteTicketAttachmentsByIDs`，验证 why.md#需求-附件生命周期-场景-附件-7-天过期自动清理

## 2. 附件本地存储（上传/下载/清理）
- [√] 2.1 新增附件存储模块（建议 `internal/tickets/storage.go` 或 `internal/storage/`）：生成随机文件名、落盘、返回相对路径；实现下载打开（仅通过 DB path），验证 why.md#需求-用户创建工单-场景-创建工单并上传附件
- [√] 2.2 实现上传限制：单文件 ≤ 100MB，总请求大小设定合理上限（例如 ≤ 120MB），并在读取 body 之前设置 `http.MaxBytesReader`，验证 why.md#需求-用户创建工单-场景-创建工单并上传附件
- [√] 2.3 实现定时清理任务：在 `internal/server/app.go` 增加 `ticketAttachmentsCleanupLoop`，调用 store 查到期附件并删除文件+DB记录（best-effort），验证 why.md#需求-附件生命周期-场景-附件-7-天过期自动清理

## 3. 用户控制台（Web）
- [√] 3.1 在 `internal/server/app.go` 增加用户路由：
  - `GET /tickets` / `GET /tickets/new` / `POST /tickets/new`
  - `GET /tickets/{ticket_id}` / `POST /tickets/{ticket_id}/reply`
  - `GET /tickets/{ticket_id}/attachments/{attachment_id}`
  并确保上传路由的中间件顺序为 `SessionAuth -> MaxBytesReader -> CSRF -> handler`
- [√] 3.2 在 `internal/web/` 增加 handler：列表/创建/详情/回复/下载附件，且用户仅能访问自己的工单，验证 why.md#需求-用户查看工单
- [√] 3.3 在 `internal/web/templates/` 增加页面模板：`tickets.html`（列表）、`ticket_new.html`（创建）、`ticket_detail.html`（详情与回复）
- [√] 3.4 在 `internal/web/templates/base.html` 增加侧边栏入口“工单”

## 4. 管理后台（Admin）
- [√] 4.1 在 `internal/server/app.go` 增加管理员路由：
  - `GET /admin/tickets` / `GET /admin/tickets/{ticket_id}`
  - `POST /admin/tickets/{ticket_id}/reply`
  - `POST /admin/tickets/{ticket_id}/close` / `POST /admin/tickets/{ticket_id}/reopen`
  - `GET /admin/tickets/{ticket_id}/attachments/{attachment_id}`
- [√] 4.2 在 `internal/admin/` 增加 handler：列表/详情/回复/关闭/恢复/下载附件，验证 why.md#需求-管理员处理工单
- [√] 4.3 在 `internal/admin/templates/` 增加页面模板：`tickets.html`（列表）、`ticket_detail.html`（详情）
- [√] 4.4 在 `internal/admin/templates/base.html` 增加侧边栏入口“工单”

## 5. 邮件通知
- [√] 5.1 新增邮件通知封装（建议 `internal/tickets/notify.go`）：统一生成邮件标题/HTML 内容，避免 web/admin 重复
- [√] 5.2 用户新建/回复时邮件通知 root 管理员；管理员回复/状态变更时邮件通知用户，验证 why.md#产品分析-价值主张与成功指标

## 6. 安全检查
- [√] 6.1 输入校验：标题/正文长度限制、空值校验、禁止超长文件名；下载 header 安全（Content-Disposition filename 处理）
- [√] 6.2 权限控制：用户仅能访问自己的 ticket/message/attachment；管理员仅 root；附件下载必须与 ticket 绑定且可见
- [√] 6.3 文件安全：禁止路径穿越；清理任务仅删除在 attachments_dir 下的相对路径文件（可加前缀校验）

## 7. 文档更新
- [√] 7.1 更新 `helloagents/wiki/data.md`：补充 tickets/message/attachments 表说明与 TTL 清理策略
- [√] 7.2 更新 `helloagents/wiki/modules/realms.md`：补充“工单系统”入口、附件目录持久化说明
- [√] 7.3 （如新增配置）更新 `config.example.yaml` 与 `README.md`：附件目录/大小/TTL 的配置说明

## 8. 测试
- [-] 8.1 store 层单测：工单创建、列表过滤（用户/管理员）、关闭/恢复、过期附件查询与删除
  > 备注: 当前仓库未内置 MySQL 集成测试基建；本次优先补齐了附件存储与路径校验的单测（`internal/tickets/*_test.go`）。
- [-] 8.2 handler 层测试（`net/http/httptest`）：用户越权访问拒绝；管理员权限；multipart + CSRF 正常；超限上传返回 413
  > 备注: 未新增 handler 级测试（需构造完整 session/CSRF + multipart 场景）；已通过 `go test ./...` 覆盖编译与核心单测。
- [√] 8.3 端到端手动验证清单：创建工单→管理员收到邮件→回复→用户收到邮件→下载附件→关闭/恢复→过期清理

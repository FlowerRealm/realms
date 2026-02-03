# 任务清单: 模型管理（白名单 / 别名重定向 / 上游绑定 / 定价）

目录: `helloagents/plan/202601141634_model_management/`

---

## 1. 数据库迁移（schema）
- [√] 1.1 新增 `internal/store/migrations/0007_managed_models.sql` 创建 `managed_models` 表，验证 why.md#变更内容
- [√] 1.2 在 `internal/store/migrate.go` 确认迁移按文件名顺序自动执行（无需改动代码），验证本地迁移可跑通

## 2. Store（模型目录 + 定价表 CRUD）
- [√] 2.1 在 `internal/store/models.go` 新增 `ManagedModel` 结构体字段定义，验证编译通过
- [√] 2.2 新增 `internal/store/managed_models.go` 实现模型目录 CRUD（List/Get/Create/Update/Delete + 按 public_id 查询），验证 why.md#需求-模型目录管理（管理员）
- [√] 2.3 扩展 `internal/store/pricing.go` 支持 Update/Delete/List（含 status 变更），验证 why.md#需求-定价表管理（管理员）

## 3. Scheduler（按模型约束选择上游）
- [√] 3.1 在 `internal/scheduler/scheduler.go` 增加 SelectByType/SelectInChannel（或统一 SelectWithConstraints）能力，复用现有 cooldown/RPM/binding，验证 why.md#需求-上游绑定模型（按模型路由）
- [√] 3.2 更新 `internal/scheduler/scheduler_test.go` 覆盖“固定 channel / 限定 type”选择与 binding 冲突处理

## 4. OpenAI 兼容 Handler（白名单 + 重写 + 绑定）
- [√] 4.1 在 `internal/api/openai/handler.go` 的 `proxyJSON` 增加：查模型目录、拒绝名单外、别名重写、计算调度约束（type/channel），验证 why.md#需求-模型白名单强制（数据面）
- [√] 4.2 在 `internal/api/openai/handler.go` 处理 `codex_oauth` 绑定模型在 `/v1/chat/completions` 的显式拒绝，验证 why.md#需求-上游绑定模型（按模型路由）
- [√] 4.3 在 `internal/api/openai/handler.go` 改造 `GET /v1/models`：输出本地 `managed_models`（启用模型），验证 why.md#变更内容

## 5. 管理后台（SSR）：模型目录管理
- [√] 5.1 在 `internal/server/app.go` 增加路由：`/admin/models`（GET/POST）与 `/admin/models/{id}`（GET/POST/delete），验证 why.md#需求-模型目录管理（管理员）
- [√] 5.2 在 `internal/admin/server.go` 实现对应 handler：列表/创建/更新/删除，并做输入校验与 channel/type 一致性校验
- [√] 5.3 新增 `internal/admin/templates/models.html`（或拆分 list/edit）并在 `internal/admin/templates/base.html` 增加导航入口，验证页面渲染

## 6. 管理后台（SSR）：定价表管理
- [√] 6.1 在 `internal/server/app.go` 增加路由：`/admin/pricing-models`（GET/POST/update/delete）
- [√] 6.2 在 `internal/admin/server.go` 实现 pricing_models 的管理 handler（最小 CRUD + status）
- [√] 6.3 新增 `internal/admin/templates/pricing_models.html` 并在 `internal/admin/templates/base.html` 增加导航入口，验证 why.md#需求-定价表管理（管理员）

## 7. Web 控制台（SSR）：用户模型列表展示
- [√] 7.1 在 `internal/web/server.go` 的 `ModelsPage` 改为从 `managed_models` 读取并展示（只展示启用模型），验证 why.md#需求-用户可见模型信息（只读）
- [√] 7.2 更新 `internal/web/templates/models.html` 增加描述/owned_by 等展示字段，并处理“未配置模型目录”的提示

## 8. 安全检查
- [√] 8.1 执行安全检查：管理面仅 root、模型字段校验、拒绝名单外模型不泄露敏感信息、routeKey 绑定不越界（按G9）

## 9. 文档更新（知识库）
- [√] 9.1 更新 `helloagents/wiki/api.md`：补充 `/admin/models`、`/admin/pricing-models` 与 `/v1/models` 语义变更说明
- [√] 9.2 更新 `helloagents/wiki/data.md`：新增 `managed_models` 表结构与字段说明
- [√] 9.3 更新 `helloagents/CHANGELOG.md`：记录模型管理上线与 `/v1/models` 行为变更

## 10. 测试
- [√] 10.1 新增/更新单测：模型白名单拒绝、别名改写、按模型绑定选择上游、`GET /v1/models` 输出，执行 `go test ./...` 通过

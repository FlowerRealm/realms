# 任务清单: 移除模型 updated_at 字段（managed_models）

目录: `helloagents/plan/202601151226_managed_models_drop_updated_at/`

---

## 1. 数据库迁移
- [√] 1.1 新增迁移 `internal/store/migrations/0016_managed_models_drop_updated_at.sql`，删除 `managed_models.updated_at`，验证 why.md#场景-数据库迁移

## 2. Store 层（数据结构 + SQL）
- [√] 2.1 在 `internal/store/models.go` 中移除 `ManagedModel.UpdatedAt` 字段，验证 why.md#场景-数据库迁移
- [√] 2.2 在 `internal/store/managed_models.go` 中移除对 `updated_at` 的 SELECT/INSERT/UPDATE（包含 NOW() 写入），验证 why.md#场景-管理后台新增编辑模型

## 3. 管理后台（admin）
- [√] 3.1 在 `internal/admin/models.go` 中移除 view 的 Updated 字段与填充逻辑，验证 why.md#场景-管理后台模型列表
- [√] 3.2 在 `internal/admin/templates/models.html` 中移除 Updated 列展示，验证 why.md#场景-管理后台模型列表

## 4. 安全检查
- [√] 4.1 执行安全检查（按G9: 输入验证、敏感信息处理、权限控制、EHRB风险规避）

## 5. 文档更新
- [√] 5.1 更新 `helloagents/CHANGELOG.md` 记录本次移除
- [√] 5.2 更新 `helloagents/history/index.md`（迁移方案包后写入索引）

## 6. 测试与格式化
- [√] 6.1 运行 `go test ./...`
- [√] 6.2 运行 `gofmt`（或 `make fmt`）确保格式一致

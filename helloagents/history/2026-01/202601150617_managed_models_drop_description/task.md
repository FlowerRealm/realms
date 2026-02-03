# 任务清单: 移除模型 description 字段（managed_models）

目录: `helloagents/plan/202601150617_managed_models_drop_description/`

---

## 1. 数据库迁移
- [√] 1.1 新增迁移 `internal/store/migrations/0014_managed_models_drop_description.sql`，删除 `managed_models.description`，验证 why.md#场景-数据库迁移

## 2. Store 层（数据结构 + SQL）
- [√] 2.1 在 `internal/store/models.go` 中移除 `ManagedModel.Description` 字段，验证 why.md#场景-数据库迁移
- [√] 2.2 在 `internal/store/managed_models.go` 中移除 `ManagedModelCreate/ManagedModelUpdate.Description`，并更新所有 SELECT/INSERT/UPDATE，验证 why.md#场景-管理后台模型管理

## 3. Web 控制台（用户侧）
- [√] 3.1 在 `internal/web/server.go` 中移除 `ModelView.Description` 与填充逻辑，验证 why.md#场景-模型列表展示
- [√] 3.2 在 `internal/web/templates/models.html` 中移除 Description 列展示，验证 why.md#场景-模型列表展示

## 4. 管理后台（admin）
- [√] 4.1 在 `internal/admin/models.go` 中移除 `managedModelView.Description` 与相关表单字段读取/写入逻辑（如仍存在），验证 why.md#场景-管理后台模型管理

## 5. 安全检查
- [√] 5.1 执行安全检查（按G9: 输入验证、敏感信息处理、权限控制、EHRB风险规避）

## 6. 文档更新
- [√] 6.1 更新 `helloagents/wiki/data.md`，同步删除 `managed_models.description` 说明
- [√] 6.2 更新 `helloagents/CHANGELOG.md` 记录本次移除

## 7. 测试与格式化
- [√] 7.1 运行 `go test ./...`
- [√] 7.2 运行 `gofmt`（或 `make fmt`）确保格式一致

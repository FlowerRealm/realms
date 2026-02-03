# 技术设计: 移除模型 updated_at 字段（managed_models）

## 技术方案

### 核心技术
- Go（Store/管理后台）
- MySQL（内置迁移 `internal/store/migrations/*.sql`）

### 实现要点
- 新增迁移删除 `managed_models.updated_at` 列
- Store 层移除 `UpdatedAt` 字段，并同步修改所有读取/写入该列的 SQL
- 管理后台移除 “Updated” 展示列，避免模板仍引用旧字段

## 数据模型

```sql
ALTER TABLE managed_models
  DROP COLUMN updated_at;
```

说明：
- `managed_models.created_at` 仍保留，用于基础创建时间记录
- 其他表（如 `channel_models`）的 `updated_at` 不在本次范围内

## 安全与性能
- **安全:** 无新增外部输入面；仅删除字段/列，不引入敏感信息处理风险
- **性能:** 轻微收益（减少写入与读取列），总体影响可忽略

## 测试与部署
- **测试:** 运行 `go test ./...`
- **部署:** 发布代码后确保启动阶段迁移执行成功；若数据库账号无 `ALTER TABLE` 权限需提前处理


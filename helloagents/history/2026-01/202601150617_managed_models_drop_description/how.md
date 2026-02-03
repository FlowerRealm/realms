# 技术设计: 移除模型 description 字段（managed_models）

## 技术方案

### 核心技术
- Go（Store/Handler/SSR 模板）
- MySQL（内置迁移 `internal/store/migrations/*.sql`）

### 实现要点
- 数据库层通过新增迁移删除 `managed_models.description` 列
- Store 层移除 `Description` 字段并同步修改所有 SQL（SELECT/INSERT/UPDATE）
- Web 控制台与管理后台同步移除 view 与模板渲染中对 description 的引用，确保编译期即可发现遗漏

## 数据模型

```sql
ALTER TABLE managed_models
  DROP COLUMN description;
```

说明：
- 历史迁移 `0011_managed_models_pricing.sql` 仍依赖 `AFTER description` 做列顺序；本次通过新增迁移在其后删除列，不需要回改历史迁移文件

## 安全与性能
- **安全:** 无新增外部输入面；仅删除字段/列，不引入敏感信息处理风险
- **性能:** 轻微收益（SELECT 列减少），总体影响可忽略

## 测试与部署
- **测试:** 运行 `go test ./...`；必要时补充 `go test` 针对 store/sql 的最小覆盖
- **部署:** 发布代码后确保启动阶段迁移执行成功；若数据库权限不足以 `ALTER TABLE`，需提前处理（或手动执行迁移）


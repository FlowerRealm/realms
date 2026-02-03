# 任务清单: cached-tokens-pricing-split

目录: `helloagents/plan/202601222052_cached-tokens-pricing-split/`

---

## 任务状态符号说明

| 符号 | 状态 | 说明 |
|------|------|------|
| `[ ]` | pending | 待执行 |
| `[√]` | completed | 已完成 |
| `[X]` | failed | 执行失败 |
| `[-]` | skipped | 已跳过 |
| `[?]` | uncertain | 待确认 |

---

## 执行状态
```yaml
总任务: 13
已完成: 13
完成率: 100%
```

---

## 任务列表

### 0. 方案确认（阻断）

- [√] 0.1 确认字段命名与迁移策略（拆分两列 + 回填 + 删除旧列）
  - 验证: proposal.md 的 D001 已明确

### 1. 数据库迁移（store/migrations）

- [√] 1.1 新增迁移：managed_models 增加 `cache_input_usd_per_1m`、`cache_output_usd_per_1m`，回填并删除 `cache_usd_per_1m`
  - 文件: `internal/store/migrations/0040_managed_models_split_cache_pricing.sql`（文件名待定，以实际序号为准）
  - 验证: 本地启动/测试环境可跑迁移；新列存在且值已回填

### 2. Store 层与数据结构（internal/store）

- [√] 2.1 更新 `store.ManagedModel` 与相关扫描/写入逻辑（替换 `CacheUSDPer1M` 为 `CacheInputUSDPer1M`/`CacheOutputUSDPer1M`）
  - 文件: `internal/store/models.go`, `internal/store/managed_models.go`
  - 验证: `go test ./...` + 关键查询/写入路径编译通过

- [√] 2.2 同步更新管理后台导入/导出字段（JSON key 与表字段一致）
  - 文件: `internal/store/admin_export_import.go`
  - 依赖: 2.1

### 3. 管理后台表单与导入（internal/admin + templates）

- [√] 3.1 新增/编辑模型表单改为两个缓存单价输入框
  - 文件: `internal/admin/templates/models.html`
  - 验证: 页面渲染正常；提交表单能带上两个字段

- [√] 3.2 更新后台 handler 解析字段并写入 store
  - 文件: `internal/admin/models.go`
  - 依赖: 2.1, 3.1

- [√] 3.3 更新“导入价格表”示例与解析逻辑为新字段
  - 文件: `internal/admin/templates/models.html`, `internal/admin/model_pricing_import.go`
  - 依赖: 2.2

- [√] 3.4 如启用 models.dev 查询填充：同步返回并填充两个字段
  - 文件: `internal/admin/model_library_lookup.go`, `internal/modellibrary/modelsdev.go`
  - 依赖: 3.1

### 4. 计费逻辑（internal/quota）

- [√] 4.1 更新 `estimateCostUSD`：四段计费 + cached tokens 子集裁剪 + 6 位小数截断
  - 文件: `internal/quota/quota.go`
  - 依赖: 2.1

- [√] 4.2 补充/更新单元测试覆盖 cached_output_tokens 与裁剪边界
  - 文件: `internal/quota/cost_test.go`
  - 依赖: 4.1

### 5. 展示与回归（internal/web + tests + KB）

- [√] 5.1 同步更新模型价格展示字段（如页面展示 cache 单价）
  - 文件: `internal/web/templates/models.html`, `internal/web/server.go`
  - 依赖: 2.1

- [√] 5.2 回归：运行测试并做格式化
  - 验证: `go test ./...`、`gofmt`（如项目有约定）

- [√] 5.3 知识库同步：更新 `helloagents/wiki/modules/realms.md` 并记录到 `helloagents/CHANGELOG.md`
  - 依赖: 5.2

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 5.2 | completed | 已运行 `go test -count=1 ./...` |

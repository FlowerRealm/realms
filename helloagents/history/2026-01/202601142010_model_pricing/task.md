# 任务清单: 模型定价（input/output/cache）与移除 legacy 上游字段

目录: `helloagents/plan/202601142010_model_pricing/`

---

## 1. 数据库迁移
- [√] 1.1 新增 `internal/store/migrations/0011_managed_models_pricing.sql`：删除 `managed_models.upstream_type/upstream_channel_id`，新增 `input/output/cache` 单价字段并提供默认值

## 2. Store（数据结构与查询）
- [√] 2.1 更新 `internal/store/models.go`：`ManagedModel` 增加 `InputUSDPer1M/OutputUSDPer1M/CacheUSDPer1M`，移除 legacy 上游字段
- [√] 2.2 更新 `internal/store/managed_models.go`：所有 SELECT/INSERT/UPDATE 同步列变更

## 3. 管理后台（模型管理）
- [√] 3.1 更新 `internal/admin/templates/models.html`：新增/编辑表单加入 input/output/cache 单价输入，移除 legacy 上游字段展示
- [√] 3.2 更新 `internal/admin/models.go`：解析并校验定价字段，写入 store

## 4. 数据面路由（OpenAI 兼容）
- [√] 4.1 更新 `internal/api/openai/handler.go`：移除对 legacy 上游字段的回退调度，统一以 `channel_models` 绑定为调度依据

## 5. 计费（成本换算）
- [√] 5.1 更新 `internal/quota/quota.go`：优先使用 `managed_models` 的三类单价，并对 cached tokens 按 cache 单价结算（无模型时回退 `pricing_models`）

## 6. 安全检查
- [√] 6.1 输入校验：表单定价字段必须为非负整数；避免成本计算溢出

## 7. 文档更新
- [√] 7.1 更新 `helloagents/wiki/data.md` 同步 `managed_models` 新增定价字段与 `pricing_models` 兜底定位
- [√] 7.2 更新 `helloagents/CHANGELOG.md` 记录本次变更

## 8. 测试
- [√] 8.1 运行 `go test ./...`

# 任务清单: upstream-vendor-realtime-stats

目录: `helloagents/plan/202601272128_upstream-vendor-realtime-stats/`

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
总任务: 8
已完成: 8
完成率: 100%
```

---

## 任务列表

### 0. 口径确认
- [√] 0.1 “请求数/成功率”按最终选中 upstream 归因（A：usage_events 口径）

### 1. Store 聚合查询
- [√] 1.1 新增 vendor 聚合结构与查询接口（requests/success/success_rate）
- [√] 1.2 校验 SQLite/MySQL 的 SQL 兼容性与索引命中（必要时补充索引）

### 2. Admin API
- [√] 2.1 新增 `GET /admin/upstream-vendors/stats`（root-only），支持 `window` 参数（默认 5m）

### 3. Admin UI
- [√] 3.1 `/admin/channels` 增加“供应商实时统计”卡片与表格
- [√] 3.2 增加刷新逻辑（fetch + loading/error），支持下拉选择窗口（1m/5m/15m/1h/6h/24h）

### 4. 测试与验证
- [√] 4.1 Store 聚合查询单元测试（覆盖 success/request 口径）
- [√] 4.2 运行测试：`go test ./...`

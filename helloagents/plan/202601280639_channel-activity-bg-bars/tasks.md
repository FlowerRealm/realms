# 任务清单: channel-activity-bg-bars

目录: `helloagents/plan/202601280639_channel-activity-bg-bars/`

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
总任务: 6
已完成: 6
完成率: 100%
```

---

## 任务列表

### 1. Store 聚合
- [√] 1.1 新增按渠道 + 分桶聚合查询（request_count/failure_count）
- [√] 1.2 SQLite 单元测试覆盖分桶与过滤条件

### 2. Admin API
- [√] 2.1 新增 `GET /admin/channels/activity` 并接入路由

### 3. Admin UI
- [√] 3.1 渠道行内增加背景容器与样式（不影响交互）
- [√] 3.2 JS 拉取活动数据并渲染 SVG 柱形背景（与窗口下拉联动）

### 4. 验证
- [√] 4.1 运行测试：`go test ./...`


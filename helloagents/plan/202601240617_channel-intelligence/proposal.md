# 变更提案: channel-intelligence

## 元信息
```yaml
类型: 新功能
方案类型: implementation
优先级: P1
状态: 已完成
创建: 2026-01-24
```

---

## 1. 需求

在“渠道智能调度管理”相关页面，前端需要支持：
- 显示哪些渠道被封禁（ban）以及剩余封禁时间
- 支持手动将某个渠道设置为“渠道指针”（运行态覆盖）
- 页头仅展示“渠道指针”概览（不展示“最近成功”概览）

---

## 2. 方案

### Scheduler（`internal/scheduler/`）
- 增加渠道指针（树级 SSOT）覆盖，排序优先级：`pointer > promotion > affinity > priority`

### 管理后台（`internal/admin/`）
- `/admin/channels`：
  - 页头展示智能调度概览（渠道指针）
  - 渠道列表行内展示：封禁状态与剩余时间
  - 新增“一键设置渠道指针”按钮（并清除该渠道封禁）
- `/admin/channels/{channel_id}/endpoints`：页头补齐“渠道指针/封禁剩余时间”展示

### 路由
- 新增：`POST /admin/channels/{channel_id}/promote`

---

## 3. 验收标准
- [√] `/admin/channels` 能直观看到封禁渠道与剩余时间
- [√] 可一键将某渠道设置为渠道指针，并影响调度顺序
- [√] `go test ./...` 通过

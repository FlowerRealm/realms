# 变更提案: usage-privacy-hide-upstream-detail

## 元信息
```yaml
类型: 修复
方案类型: implementation
优先级: P0
状态: 已完成
创建: 2026-02-04
```

---

## 1. 需求

### 背景
普通用户在“用量统计 / 请求明细”中展开某条请求详情时，仍可能看到与上游渠道相关的信息（例如通过上游明细 body 间接暴露渠道），不符合“仅管理员后台可见”的隐私要求。

### 目标
- 用户侧（`/usage` + `/api/usage/*`）不展示、也不返回任何可用于识别“具体上游渠道”的信息。
- 管理后台（`/admin/usage` + `/api/admin/usage/*`）保持可见性不变：仍可查看渠道字段与上游明细。

### 约束条件
```yaml
时间约束: 无
性能约束: 用户侧用量列表与明细接口不得引入明显额外开销
兼容性约束: 允许用户侧不再获取上游请求/响应明细（字段缺失需前端兼容）
业务约束: 需要 Playwright 覆盖失败请求场景回归
```

### 验收标准
- [ ] `GET /api/usage/events` 返回中不包含任何上游渠道/端点/凭据标识字段（包括但不限于 `upstream_*_id`）。
- [ ] `GET /api/usage/events/:event_id/detail`（用户侧）返回中不包含 `upstream_request_body` / `upstream_response_body`。
- [ ] 用户侧 `/usage` 展开明细时，不出现上游渠道名称（例如 `pw-e2e-upstream`），并提示“仅管理员可查看”。
- [ ] Playwright E2E 覆盖失败请求：上游返回 400 且响应体包含渠道名字段时，用户侧仍不泄露。

---

## 2. 方案

### 技术方案
服务端做权限分层的数据裁剪：
- 用户侧用量列表：不返回任何 upstream endpoint/credential 等可定位到渠道的字段。
- 用户侧用量明细：仅返回 `downstream_request_body`（下游原始请求体）；上游请求/响应明细仅供管理后台接口返回。

前端对“上游明细缺失”做显式展示：当明细接口不返回 upstream body 字段时，在 UI 中显示“（仅管理员可查看）”。

### 影响范围
```yaml
涉及模块:
  - Router/API: 用户侧用量接口数据裁剪
  - Web/Frontend: 用量页明细展示逻辑
  - Web/E2E: 增强失败请求隐私回归
预计变更文件: 5
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| 用户侧排障能力下降（不再展示上游请求/响应体） | 中 | UI 明确提示“仅管理员可查看”；失败原因仍保留 `error_message` 摘要 |
| 回归测试覆盖不足导致再次泄露 | 中 | 增强 fake upstream：可构造失败响应体包含渠道名字段，并由 Playwright 覆盖 |

---

## 3. 技术设计（可选）

### API 设计

#### GET /api/usage/events（用户侧）
- **变更**: 响应中移除 `upstream_endpoint_id` / `upstream_credential_id`（以及任何可能推断上游渠道的 upstream 标识字段）。

#### GET /api/usage/events/:event_id/detail（用户侧）
- **变更**: 响应中仅保留 `event_id` / `available` / `downstream_request_body`；不返回上游明细字段（`upstream_request_body` / `upstream_response_body`）。

---

## 4. 核心场景

### 场景: 用户查看失败请求明细不泄露渠道
**模块**: Web/Usage + Router/Usage API  
**条件**: 上游返回非 2xx 且保存了 `usage_event_details`  
**行为**: 用户在 `/usage` 点击一行展开详情，按需加载 `/api/usage/events/:id/detail`  
**结果**: 详情中不展示任何上游渠道信息；上游请求/响应明细位置显示“（仅管理员可查看）”

---

## 5. 技术决策

本次为明确的权限裁剪修复，不涉及额外架构/选型决策。

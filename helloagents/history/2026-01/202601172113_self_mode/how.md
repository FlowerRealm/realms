# 技术设计: 自用模式（禁用计费与工单）

## 技术方案

### 核心技术
- Go `net/http` 路由注册裁剪（未注册即 404）
- 配置驱动（YAML + 环境变量覆盖）
- quota Provider 抽象复用：新增 self_mode 专用 Provider

### 实现要点
- **配置层:** 在 `internal/config` 增加 `self_mode.enable`，默认关闭；并提供示例配置。
- **路由层:** 在 `internal/server/app.go` 按 `self_mode` 分组注册路由：
  - `billing/payment`：订阅、订单、充值、支付页、Stripe/EPay webhook、后台订阅/订单/支付渠道
  - `tickets`：用户工单与后台工单、附件上传/下载
  - 其余路由保持现状
- **quota 层:** 自用模式下替换为 `SelfModeProvider`：
  - `Reserve`: 直接写入 `usage_events(reserved)`（`subscription_id=nil`），不检查订阅/余额
  - `Commit/Void`: 复用现有成本估算逻辑与 `CommitUsage/VoidUsage`
  - 目标是：不阻断请求 + 保留用量可观测性
- **UI 层（SSR）:** 在 `internal/web/templates/base.html` 与 `internal/admin/templates/base.html` 隐藏入口，避免误点与困惑。

## 架构决策 ADR

### ADR-006: 以 self_mode 作为运行时功能开关
**上下文:** 当前功能域覆盖通用 SaaS 场景，但自用场景需要显著裁剪，且必须保持默认模式完全兼容。
**决策:** 引入 `self_mode.enable`，通过“路由未注册 + quota Provider 切换 + UI 隐藏”实现硬禁用。
**理由:**
- 逻辑直观、可验证（404 即禁用）
- 不引入编译矩阵，不增加发布复杂度
- 默认行为不变，符合“Never Break Userspace”
**替代方案:** build tags 编译裁剪 → 拒绝原因: 维护两套构建/测试与部署路径，收益不匹配当前需求。
**影响:**
- 自用模式下会出现“同一路径在不同模式行为不同”，需要文档明确说明
- 需要补充路由级测试，避免后续新增路由时误开放

## API 设计
本变更不新增新 API；通过模式开关改变可达性与配额策略：
- 自用模式下：计费/支付/工单相关路径返回 404（或统一“已禁用”）
- 自用模式下：数据面不再因 `订阅未激活/订阅额度不足` 直接拒绝（仍受 TokenInflightLimiter/BodyLimit 等限制）

## 数据模型
无新增迁移。沿用现有表：
- `usage_events` 继续用于记录请求与用量（自用模式下 `subscription_id=NULL`）
- 计费/工单相关表保留但不再通过路由入口操作

## 安全与性能
- **安全:** 显著减少对外暴露的路由与表单入口（支付回调、上传入口等），降低攻击面。
- **性能:** 无显著性能变化；自用模式可选关闭 tickets 清理 loop，减少无意义的定时 DB 扫描。

## 测试与部署
- **测试:**
  - 路由级测试：自用模式下被禁用路径应返回 404；默认模式保持原行为
  - quota 测试：自用模式下无订阅也能通过 Reserve/Commit 流程
- **部署:**
  - 默认不变
  - 自用模式：在 `config.yaml` 启用 `self_mode.enable=true` 后重启即可生效


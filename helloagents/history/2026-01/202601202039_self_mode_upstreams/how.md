# 技术设计: 自用模式（self_mode）+ 多上游管理硬化

## 技术方案

### 核心技术
- Go `net/http`（现有路由体系保持不变）
- MySQL（现有 schema 保持不变）
- Feature Gate：`FeatureDisabledEffective/FeatureStateEffective` + `middleware.FeatureGateEffective`

### 实现要点
- **唯一判定源:** 统一以 `self_mode.enable` 与 `feature_disable_*` 的最终状态为准；在路由层“少注册”，在 handler 链路“再兜底拒绝”。
- **自用默认策略:** self_mode 下默认隐藏/禁用计费与工单，避免把商业域暴露给自用部署。
- **导出/导入（自用优先）:** 提供最小可用的“结构化备份”能力，默认不导出敏感字段（上游 key/token、支付密钥），导入时需要管理员二次补全。
- **多上游稳定性:** 不改变现有调度与 SSE 约束；只补齐可观测与配置体验，避免对数据面请求路径做破坏性改动。

## 架构决策 ADR

### ADR-006: 双形态开关与功能域隔离
**上下文:** 同一服务既需要自用（多上游管理）也需要商业化（计费/支付），但自用形态不应暴露商业域入口。  
**决策:** 以 `self_mode.enable` 作为强制形态开关，并与 `feature_disable_*` 合并产出最终状态；在路由注册层与 FeatureGateEffective 双保险隔离功能域。  
**理由:** 最小变更、可回滚、避免在业务代码散落 if/else；“关模块=无入口”最符合自用安全预期。  
**替代方案:** 用编译 tag 拆分两套二进制 → 拒绝原因: 维护成本高、容易分叉；且现有系统已具备运行时 feature gate。  
**影响:** 需要补齐路由覆盖与测试，避免“入口残留”。

## API 设计

### 管理面导出/导入（计划新增）
- **[GET]** `/admin/export`（或更细分 `/admin/export/upstreams` 等）
  - 返回 JSON（不含敏感字段）
- **[POST]** `/admin/import`
  - 接受 JSON 并创建/更新配置（敏感字段需要二次补齐）

> 注：具体路径与细分颗粒度以实现时为准，避免一次引入过大面向未来的接口。

## 数据模型
- 本方案不新增业务表；仅引入导出/导入所需的“序列化结构”（代码内 DTO），避免 DB 结构被自用需求绑架。

## 安全与性能
- **安全:**
  - self_mode 下减少暴露面：不注册计费/支付/工单路由；入口统一走 FeatureGateEffective（404）
  - 导出默认不包含敏感字段；日志中避免打印 key/token/密钥（必要时仅展示 hint）
- **性能:**
  - feature gate 判定应尽量复用现有 `FeatureStateEffective`（避免每次请求多次 DB roundtrip）

## 测试与部署
- **测试:**
  - 增加路由级测试：self_mode=true 时关键入口返回 404；self_mode=false 时保持原有行为
  - 增加导出/导入测试：导出不含敏感字段；导入后配置可用且不破坏已有数据
- **部署:**
  - 自用：推荐配置 `self_mode.enable=true`，并在 `app_settings_defaults` 里默认关闭 billing/tickets/admin 对应菜单（数据库可覆盖）


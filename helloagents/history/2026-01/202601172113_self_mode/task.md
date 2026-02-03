# 任务清单: 自用模式（禁用计费与工单）

目录: `helloagents/plan/202601172113_self_mode/`

---

## 1. 配置层（self_mode 开关）
- [√] 1.1 在 `internal/config/config.go` 增加 `self_mode.enable` 配置结构并接入 Load 逻辑，验证 why.md#需求-self_mode-开关-场景-enable_self_mode
- [√] 1.2 更新 `config.example.yaml` 补充自用模式示例配置，验证 why.md#需求-self_mode-开关-场景-enable_self_mode，依赖任务1.1

## 2. 路由裁剪（禁用计费/支付/工单）
- [√] 2.1 在 `internal/server/app.go` 按 `self_mode` 裁剪注册：订阅/订单/充值/支付页/支付 webhook/后台计费入口，验证 why.md#需求-禁用计费与支付域-场景-disable_billing_routes
- [√] 2.2 在 `internal/server/app.go` 按 `self_mode` 裁剪注册：用户工单与后台工单路由（含附件上传/下载），验证 why.md#需求-禁用工单域-场景-disable_tickets_routes_and_jobs，依赖任务2.1

## 3. quota 策略（自用模式不要求订阅）
- [√] 3.1 在 `internal/quota/` 新增 SelfModeProvider（记录 usage_events 但不检查订阅/余额），并在 `internal/server/app.go` 中按 `self_mode` 切换 Provider，验证 why.md#需求-自用模式配额策略-场景-allow_api_without_subscription

## 4. UI 入口隐藏（SSR）
- [√] 4.1 在 `internal/web/templates/base.html` 隐藏订阅/充值/支付/工单入口（自用模式下），验证 why.md#需求-禁用计费与支付域-场景-disable_billing_routes
- [√] 4.2 在 `internal/admin/templates/base.html` 隐藏订阅/订单/支付渠道/工单入口（自用模式下），验证 why.md#需求-禁用计费与支付域-场景-disable_billing_routes

## 5. 文档更新（SSOT）
- [√] 5.1 更新 `helloagents/wiki/api.md`：补充 self_mode 下的行为差异（禁用路径 + 配额策略），验证 why.md#需求-self_mode-开关-场景-enable_self_mode
- [√] 5.2 更新 `helloagents/wiki/modules/realms.md`：新增“自用模式”章节与禁用清单，验证 why.md#需求-self_mode-开关-场景-enable_self_mode
- [√] 5.3 更新 `helloagents/CHANGELOG.md` 记录新增 self_mode（不破坏默认模式），验证 why.md#需求-self_mode-开关-场景-default_mode_unchanged

## 6. 测试
- [√] 6.1 新增路由级测试覆盖 self_mode 与默认模式的关键路径（404/非404），验证点：计费/支付/工单路由不可达 + 数据面可用
- [√] 6.2 执行 `go test ./...`，确保全量通过

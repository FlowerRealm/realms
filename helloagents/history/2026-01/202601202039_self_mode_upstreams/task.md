# 任务清单: 自用模式（self_mode）+ 多上游管理硬化

目录: `helloagents/plan/202601202039_self_mode_upstreams/`

---

## 1. 功能域隔离（self_mode + feature gates）
- [√] 1.1 核对并补齐 `internal/server/app.go` 的路由注册：self_mode=true 时计费/支付/工单相关路由不注册，验证 why.md#核心场景-需求-self_mode-强制关闭-billingtickets-场景-self_modetrue-时入口与回调不可达
- [√] 1.2 核对 `internal/web/*` 与 `internal/admin/*`：页面入口与菜单基于 `FeatureStateEffective` 隐藏，且所有 handler 仍走 `FeatureGateEffective` 兜底，验证 why.md#核心场景-需求-self_mode-强制关闭-billingtickets-场景-self_modetrue-时入口与回调不可达

## 2. 多上游管理体验补齐（OpenAI 兼容 + Codex OAuth）
- [√] 2.1 补齐管理后台对 OpenAI 兼容与 Codex OAuth 的“可解释性信息”（限流/冷却/失败原因展示），验证 why.md#核心场景-需求-上游限流粘性健康具备可解释性-场景-选择结果可解释且不破坏-sse-约束
- [√] 2.2 增加最小“导出/导入”能力（默认不含敏感字段），落在 `internal/admin/*` + `internal/store/*`，验证 why.md#变更内容

## 3. 安全检查
- [√] 3.1 执行安全检查（按 G9）：入口隔离、权限校验、导出脱敏、日志脱敏、open redirect 与 CSRF 检查

## 4. 文档更新
- [√] 4.1 在 `helloagents/wiki/arch.md` 增加“双形态”描述与 ADR-006 索引
- [√] 4.2 在 `helloagents/wiki/api.md` 增加导出/导入接口文档（如本轮实现）

## 5. 测试
- [√] 5.1 增加路由测试：self_mode=true 时计费/工单相关入口 404；self_mode=false 时入口存在（至少不 404）
- [√] 5.2 运行 `go test`（可分包运行以避开与本任务无关的编译阻塞）

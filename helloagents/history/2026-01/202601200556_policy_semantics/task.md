# 任务清单: 语义化功能禁用（Policy Semantics）

目录: `helloagents/history/2026-01/202601200556_policy_semantics/`

---

## 1. Store（策略状态与开关读取）
- [√] 1.1 在 `internal/store/app_settings.go` 增加 `policy_*` keys 常量与读取/写入支持
- [√] 1.2 新增 `internal/store/policies.go` 实现 `PolicyStateEffective(ctx, selfMode)`，并补齐单测

## 2. DB 与余额状态机（区分 payg vs free）
- [-] 2.1 增加 `usage_events.balance_reserved_usd`
  > 备注: 本轮跳过：free mode 下 `reserved_usd=0`，避免误触退款/过期返还逻辑。
- [-] 2.2 调整 refund/expire 判定逻辑
  > 备注: 本轮跳过：同上。

## 3. Quota（free mode + unknown model 宽容）
- [√] 3.1 增加基于 PolicyState 的 provider 选择，free mode 下不校验订阅/余额且不做余额扣减/返还
- [√] 3.2 free mode 下对未知模型的成本估算宽容（缺少定价时记为 0）

## 4. OpenAI Handler（模型穿透与约束调整）
- [√] 4.1 增加 model passthrough 分支：跳过模型白名单/绑定、保持 `model` 透传、调整 constraints
- [-] 4.2 chat passthrough
  > 备注: 本轮不做：保持 `X-Realms-Chat: 1` 的对话分组约束不变。

## 5. Middleware（FeatureGate 一致性）
- [√] 5.1 FeatureGate 改用 effective gate（含 defaults + self_mode），并更新调用方

## 6. Admin（可选：暴露策略开关）
- [√] 6.1 系统设置页增加 policy_* 开关展示与保存（保持向后兼容）

## 7. 安全检查
- [√] 7.1 安全检查：policy 不绕过 SSRF/鉴权护栏，仅改变数据面语义

## 8. 文档更新
- [√] 8.1 更新 `helloagents/wiki/modules/realms.md` 增加 policy_* 说明与“feature vs policy”边界
- [√] 8.2 更新 `helloagents/wiki/api.md` 补充 free mode / model passthrough 对数据面行为的影响

## 9. 测试
- [√] 9.1 新增/更新单测并执行 `go test ./...`

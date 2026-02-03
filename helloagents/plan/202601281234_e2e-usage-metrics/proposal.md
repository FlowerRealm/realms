# 变更提案: e2e_usage_metrics

## 元信息
```yaml
类型: 流程/测试
方案类型: implementation
优先级: P2
状态: ✅完成
创建: 2026-01-28
```

---

## 1. 需求

### 背景
现有 Codex CLI E2E 仅验证 `codex exec` 的输出包含固定标记字符串，缺少对 Realms 数据面“用量事件（usage_events）”的端到端校验。

### 目标
- 在 E2E 中补齐以下断言：
  - 请求数：usage_events 记录数量与预期一致
  - Token 数：`input_tokens/output_tokens` 正确落库且为正数
  - 缓存：`cached_*_tokens`（若上游返回）落库且口径合理（非负、且不超过总 Token）
- 将 prompt 从“回复固定字符串”调整为“生成最小 Go 程序”，让用例更贴近日常代码生成场景，同时保留可稳定断言的标记 `REALMS_CI_OK`。

### 约束条件
```yaml
依赖: Codex CLI（npm 全局安装 @openai/codex）
上游: 保留真实上游（CI 通过 secrets 注入），并新增测试内置 fake upstream（httptest）用于稳定校验缓存命中
安全: 测试输出必须对 upstream key / rlm token 做脱敏（redact）
```

### 验收标准
- [ ] `tests/e2e/codex_cli_test.go`：
  - 输出包含 `package main` 与 `REALMS_CI_OK`
  - 真实上游用例：SQLite 中存在 2 条 committed `usage_event`（两次请求），且第二次必须命中缓存：`cached_input_tokens > 0`
    - 并断言两次请求的 `input_tokens/output_tokens` 均 > 0
  - fake upstream 用例：SQLite 中存在 2 条 committed `usage_event`（第二次用于验证缓存），且第二次必须命中缓存：`cached_input_tokens > 0`
  - 所有 `cached_*_tokens`（若不为 nil）必须 ≥ 0 且 ≤ total tokens
- [ ] `go test ./...` 通过

---

## 2. 方案

### 技术方案概览
1) 保留真实上游 E2E：继续使用 env/secrets 注入的 `base_url/api_key/model`，验证真实链路与用量落库。
2) 增加 fake upstream：可控返回 `usage.input_tokens_details.cached_tokens`，用于稳定验证缓存 Token 落库与“第二次必须命中”。
3) 调整 E2E 的 prompt：让 Codex 输出最小 Go 程序（包含 `REALMS_CI_OK`），并校验关键代码片段存在。
4) 在 E2E 中通过 Store 直接查询 `usage_events`：
   - 加入短轮询等待（避免极端情况下的异步落库竞态）
   - 校验事件数量、状态（committed）、Token 字段与缓存 Token 口径

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| Codex CLI 行为变更导致请求数不符合预期 | 中 | 用例保持最小 prompt，避免触发多轮工具调用；失败时输出明确的 usage_events 计数便于定位 |

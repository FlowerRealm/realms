# 变更提案: fix-ci-real-e2e-gating

## 元信息
```yaml
类型: 修复
方案类型: implementation
优先级: P1
状态: ✅完成
创建: 2026-02-16
```

---

## 1. 问题

当前 CI 在注入 `REALMS_CI_*` Secrets 后，会触发真实上游的 Codex E2E 用例运行，但存在两个问题：

1) **重复执行 / 口径混乱**  
`scripts/ci-real.sh` 里先执行 `go test ./...`，随后又单独执行 `go test ./tests/e2e -run TestCodexCLI_E2E`。  
由于 `TestCodexCLI_E2E` 只要检测到 `REALMS_CI_*` 就会运行，导致它在 `go test ./...` 阶段也会跑一次（且**没有重试**），从而出现：
- 真实上游调用成本增加（重复跑）
- flake 发生时绕过脚本重试逻辑，直接导致 CI 失败

2) **主工作流默认测 real 的稳定性不足**  
当希望“提交即测真实上游”时，需要确保真实上游用例都运行在可控入口内（带 retry），避免被 `go test ./...` 的隐式执行打断。

---

## 2. 目标

- 将 **真实上游 Codex E2E** 的执行入口收敛为 `scripts/ci-real.sh` 的显式步骤（带 retry）。
- 避免 `go test ./...` 在存在 `REALMS_CI_*` 时**隐式触发真实上游**用例。
- 保持本地/CI 同口径：开发者通过 `make ci` / `scripts/ci.sh` 能 1:1 复现 CI 的行为（在有/无 `REALMS_CI_*` 的两种模式下均清晰）。

---

## 3. 方案

### 3.1 让 `TestCodexCLI_E2E` 受控于 `REALMS_CI_ENFORCE_E2E`

- 在 `tests/e2e/codex_cli_test.go` 的 `TestCodexCLI_E2E` 开头增加 gating：
  - 未设置 `REALMS_CI_ENFORCE_E2E` → `t.Skip(...)`
  - 设置后才允许读取 `REALMS_CI_*` 并执行真实上游链路

这样可确保：
- `go test ./...`（不主动开启 E2E）不会触发真实上游
- `scripts/ci-real.sh` 在显式 export `REALMS_CI_ENFORCE_E2E=1` 后再执行该测试，并由脚本负责 retry

### 3.2 文档同步

- 更新 `helloagents/modules/testing.md`：明确真实上游 Codex E2E 的 gating 约定与 CI 运行口径。

---

## 4. 验收标准

- [ ] `go test ./...` 在仅设置 `REALMS_CI_*`（未设置 `REALMS_CI_ENFORCE_E2E`）时，不会运行 `TestCodexCLI_E2E`
- [ ] `scripts/ci-real.sh` 仍能运行 `TestCodexCLI_E2E`，且通过 retry 包裹保证稳定性
- [ ] `helloagents/modules/testing.md` 与当前脚本行为一致（SSOT）

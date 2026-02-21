# 任务清单: testing-unify-codex-playwright

> **@status:** completed | 2026-02-16 13:11

目录: `helloagents/archive/2026-02/202602161127_testing-unify-codex-playwright/`

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
总任务: 13
已完成: 13
完成率: 100%
```

---

## 任务列表

### 1. 统一入口（本地/CI 同口径）

- [√] 1.1 新增统一入口脚本 `scripts/ci.sh`（默认 seed/fake upstream）
  - 内容: go tests + Codex 可用性（fake upstream）+ Playwright（seed）
  - 验证: `bash "scripts/ci.sh"`（本地需已安装 `codex`、已安装 Playwright chromium）

- [√] 1.2 Makefile 增加统一目标 `make ci`（调用 `scripts/ci.sh`）
  - 依赖: 1.1
  - 验证: `make ci`

- [√] 1.3 明确“检查集边界”：主入口仅覆盖 E2E/冒烟（Codex/Playwright），不混入需要真实上游的用例
  - 输出: `scripts/ci.sh` 头部说明 + README 章节同步
  - 依赖: 1.1

### 2. GitHub Actions（主工作流统一入口）

- [√] 2.1 重构 `.github/workflows/ci.yml` 为单一入口调用（不在 YAML 内拼装多段测试命令）
  - 内容: 统一安装 Go/Node/Codex/Playwright chromium 后，执行 `make ci`
  - 验证: CI 在无真实上游 Secrets 的情况下也能通过

- [√] 2.2 新增 `.github/workflows/ci-real.yml`（可选，workflow_dispatch + 可选 schedule）
  - 内容: 复用现有 real upstream 测试逻辑（Codex real + Playwright real 或 seed+real upstream）
  - 验证: 有 secrets 的仓库手动触发通过；无 secrets 时明确失败/提示（不影响主 CI）

- [√] 2.3 清理/合并 CI 文档说明：README 中区分 “ci（默认）/ ci-real（可选）”
  - 依赖: 2.1, 2.2

### 3. Codex 可用性（E2E/冒烟）

- [√] 3.1 调整/补齐 Go E2E 入口，使主入口只跑 fake upstream 的 Codex 用例
  - 目标: `go test ./tests/e2e -run TestCodexCLI_E2E_FakeUpstream_Cache -count=1` 成为主线口径
  - 验证: 本地与 CI 均可运行且不依赖上游 Secrets

- [√] 3.2 （可选）新增 `scripts/smoke-codex.sh`，替代 `scripts/smoke-curl*.sh` 作为“可用性冒烟”推荐入口
  - 验证: `bash "scripts/smoke-codex.sh"`（输出包含预期关键字）

### 4. Playwright（组件级/交互级）

- [√] 4.1 新增至少 1 个 Playwright 用例覆盖关键组件交互（组件级）与页面流程（交互级）
  - 位置: `web/e2e/*.spec.ts`
  - 验证: `npm --prefix web run test:e2e:ci`

### 5. 文档与知识库同步

- [√] 5.1 更新 `README.md`：统一入口、检查集口径、ci-real 触发方式
  - 依赖: 1.1, 2.1

- [√] 5.2 更新 `web/README.md`：Playwright（seed/real）与组件级/交互级覆盖说明
  - 依赖: 4.1

- [√] 5.3 新增/更新知识库模块文档 `helloagents/modules/testing.md`，并更新 `helloagents/modules/_index.md`
  - 内容: 测试分层、入口、CI 约定、环境变量规范

- [√] 5.4 开发完成后归档方案包并更新 `helloagents/CHANGELOG.md`
  - 验证: `helloagents/archive/YYYY-MM/202602161127_testing-unify-codex-playwright/` 存在；CHANGELOG 有对应条目

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 3.1 | completed | 现有 `TestCodexCLI_E2E_FakeUpstream_Cache` 已满足主线口径，无需额外修改 |
| 5.4 | completed | 已迁移至 `helloagents/archive/2026-02/202602161127_testing-unify-codex-playwright/`，并写入 `helloagents/CHANGELOG.md` |

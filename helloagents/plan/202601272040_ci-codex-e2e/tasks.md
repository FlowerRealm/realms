# 任务清单: ci_codex_e2e

目录: `helloagents/plan/202601272040_ci-codex-e2e/`

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
总任务: 5
已完成: 5
完成率: 100%
```

---

## 任务列表

### 1. E2E 测试实现（Go）

- [√] 1.1 新增 Codex CLI E2E 测试（SQLite 自举 + 启动 Realms + codex exec）
  - 文件: `tests/e2e/codex_cli_test.go`

### 2. GitHub Actions

- [√] 2.1 新增 CI workflow（push 触发：单测 + E2E）
  - 文件: `.github/workflows/ci.yml`

### 3. 文档

- [√] 3.1 README 增加 CI secrets 与本地复现说明
  - 文件: `README.md`

### 4. 知识库同步

- [√] 4.1 更新 Changelog（记录 CI/E2E 变更）
  - 文件: `helloagents/CHANGELOG.md`

- [√] 4.2 新增/更新模块文档：CI & E2E
  - 文件: `helloagents/modules/_index.md`、`helloagents/modules/ci_github_actions.md`

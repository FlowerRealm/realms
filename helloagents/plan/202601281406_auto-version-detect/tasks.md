# 任务清单: auto-version-detect

目录: `helloagents/plan/202601281406_auto-version-detect/`

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
总任务: 15
已完成: 12
完成率: 80%
```

---

## 任务列表

### 1. 版本自动探测（internal/version）
- [√] 1.1 设计并实现优先级：ldflags 注入优先 → BuildInfo 回退 →（可选）git describe
- [√] 1.2 通过 `runtime/debug.ReadBuildInfo()` 自动填充 `date`；不生成/不拼接 sha
- [√] 1.3 当未注入版本时：
  - 若可获取 git tag，则将 `version` 设为最近 tag（不含 sha）
  - 否则 `version=dev`（dirty 时可为 `dev.dirty`）
- [√] 1.4 单测：注入优先、dev 回退、空值容错（至少 1 个边界用例）

### 2. 对外输出去 sha（API + 前端）
- [√] 2.1 `GET /api/version`：移除/隐藏 `commit` 字段（确保不输出 sha）
- [√] 2.2 `GET /healthz`：移除/隐藏 `commit` 字段（确保不输出 sha）
- [√] 2.3 Web 控制台/管理后台页脚：移除 commit 显示逻辑（不再展示 `(...commit...)`）

### 3. 构建链路自动注入（可选但推荐）
- [-] 3.1 新增 `scripts/version.sh`：从 git 自动生成 version/date（无 git 时回退；不生成 sha）
- [-] 3.2 Makefile 增加 `make build`：自动注入 `-ldflags -X ...`（本地构建一致性）
- [-] 3.3 Dockerfile 增强：build args 为空时自动计算（仅在 `.git` 可用时生效）
- [√] 3.4 Docker/CI：不再注入/不再展示 commit sha（如需保留内部注入也必须确保不对外输出）

### 4. 文档与对外说明
- [√] 4.1 更新 `README.md` 版本章节：解释“实例自身版本”获取规则与回退策略（不含 sha）
- [√] 4.2 更新 `docs/versioning.md`：区分 runtime 版本 vs GitHub Pages latest（不含 sha）

### 5. 验证
- [√] 5.1 运行测试：`go test ./...`
- [√] 5.2 本地快速验证：`go run ./cmd/realms` 时 `/api/version` 的 `version/date` 正常，且不输出 sha/commit

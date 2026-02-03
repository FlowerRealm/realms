# 任务清单: make_dev_dual_local_docker_self

> **@status:** completed | 2026-01-28 14:50

目录: `helloagents/archive/2026-01/202601281445_make-dev-dual-local-docker-self/`

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
总任务: 9
已完成: 9
完成率: 100%
```

---

## 任务列表

### 1. 开发脚本

- [√] 1.1 更新 `scripts/dev.sh`：启动 docker self（7080，自用模式）+ 本地 air（8080，正常模式）
  - 验证: `bash -n scripts/dev.sh`

### 2. Compose 兼容

- [√] 2.1 更新 `docker-compose.yml`：确保可向容器传入 `REALMS_SELF_MODE_ENABLE`（默认 false）
  - 验证: `docker compose config`

### 3. Makefile & 文档

- [√] 3.1 更新 `Makefile`：说明 `make dev` 会同时启动本地(8080)与 docker self(7080)
- [√] 3.2 更新 `README.md`：补充双环境开发用法与端口/数据库隔离说明

### 4. 知识库同步

- [√] 4.1 更新 `helloagents/wiki/modules/realms.md`：补充 `make dev` 双环境说明
- [√] 4.2 更新 `helloagents/CHANGELOG.md`：记录本次变更，并链接归档后的方案包

### 5. 验证与归档

- [√] 5.1 验证：`bash -n scripts/dev.sh`、`docker compose config`
- [√] 5.2 测试：`go test ./...`
- [√] 5.3 归档：迁移方案包到 `helloagents/archive/2026-01/` 并更新索引

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 5.1 | [√] | `bash -n scripts/dev.sh` 通过；`docker compose config` 通过 |
| 5.2 | [√] | `go test ./...` 通过 |
| 5.3 | [√] | 已迁移至 `helloagents/archive/2026-01/202601281445_make-dev-dual-local-docker-self/`，并更新 `helloagents/archive/_index.md` |

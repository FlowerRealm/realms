# 任务清单: mysql_port_expose_configurable

> **@status:** completed | 2026-01-28 14:29

目录: `helloagents/archive/2026-01/202601281424_mysql-port-expose-configurable/`

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
总任务: 10
已完成: 10
完成率: 100%
```

---

## 任务列表

### 1. Docker Compose

- [√] 1.1 更新 `docker-compose.yml`：为 `mysql` 增加端口映射，默认对外监听；宿主端口可配置
  - 验证: `docker compose config`

### 2. 文档与示例

- [√] 2.1 更新 `.env.example`：补充 `MYSQL_HOST_PORT`（以及安全提示/可选仅本机监听方式）
- [√] 2.2 更新 `docs/USAGE.md`：说明 MySQL 默认暴露端口、如何改端口与安全注意事项
- [√] 2.3 更新 `README.md`：将“修改 docker-compose.yml 端口映射”的说明改为“配置 `MYSQL_HOST_PORT`”

### 3. 开发辅助脚本

- [√] 3.1 更新 `scripts/dev-mysql.sh`：端口检测默认读取 `MYSQL_HOST_PORT`（保留 `REALMS_DEV_MYSQL_PORT` 可覆盖）
  - 验证: `bash -n scripts/dev-mysql.sh`

### 4. 知识库同步

- [√] 4.1 更新 `helloagents/wiki/modules/realms.md`：同步 MySQL 端口暴露与配置项说明
- [√] 4.2 更新 `helloagents/CHANGELOG.md`：记录本次变更，并链接归档后的方案包

### 5. 验证与归档

- [√] 5.1 验证：`docker compose config`
- [√] 5.2 测试：`go test ./...`（确保本次文档/脚本/compose 改动不影响工程）
- [√] 5.3 归档：迁移方案包到 `helloagents/archive/2026-01/` 并更新索引

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 5.1 | [√] | `docker compose config` 通过 |
| 5.2 | [√] | `go test ./...` 通过 |
| 5.3 | [√] | 已迁移至 `helloagents/archive/2026-01/202601281424_mysql-port-expose-configurable/`，并更新 `helloagents/archive/_index.md` |

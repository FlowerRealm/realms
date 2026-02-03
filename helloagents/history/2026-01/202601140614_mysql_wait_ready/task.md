# 任务清单: MySQL 启动等待与重试（dev）

目录: `helloagents/plan/202601140614_mysql_wait_ready/`

---

## 1. 启动健壮性
- [√] 1.1 在 `internal/store/db.go` 中为 `env=dev` 增加 `Ping` 重试与等待窗口，避免 MySQL 容器启动阶段的 `unexpected EOF / driver: bad connection` 直接导致启动失败
- [√] 1.2 保持错误可诊断：超过等待窗口仍失败时返回原始错误（不吞错）

## 2. 文档与记录
- [√] 2.1 更新 `README.md`：说明 dev 环境会等待 MySQL 就绪（有限时）
- [√] 2.2 更新 `helloagents/wiki/modules/codex.md`：补充本次变更记录
- [√] 2.3 更新 `helloagents/CHANGELOG.md`（Unreleased）

## 3. 验证
- [√] 3.1 运行 `go test ./...`

## 4. 迁移
- [√] 4.1 迁移方案包至 `helloagents/history/2026-01/202601140614_mysql_wait_ready/` 并更新 `helloagents/history/index.md`

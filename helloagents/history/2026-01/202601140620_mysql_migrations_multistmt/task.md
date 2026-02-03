# 任务清单: MySQL 迁移多语句执行修复

目录: `helloagents/plan/202601140620_mysql_migrations_multistmt/`

---

## 1. 迁移执行修复
- [√] 1.1 在 `internal/store/migrate.go` 中将迁移文件按语句拆分后逐条执行，避免 `multiStatements` 依赖导致的 SQL 语法报错
- [√] 1.2 保持事务语义：同一迁移文件仍在单个事务内执行，失败回滚

## 2. 文档与记录
- [√] 2.1 更新 `helloagents/wiki/modules/codex.md`：补充本次变更记录
- [√] 2.2 更新 `helloagents/CHANGELOG.md`（Unreleased）

## 3. 验证
- [√] 3.1 运行 `go test ./...`
- [√] 3.2 使用本地 MySQL 启动一次 `go run ./cmd/codex -config config.yaml`，验证迁移可正常执行并进入启动阶段
  > 备注: 当前会进入后续初始化阶段，但仍可能因 Web 模板解析失败而退出（另行修复）

## 4. 迁移
- [√] 4.1 迁移方案包至 `helloagents/history/2026-01/202601140620_mysql_migrations_multistmt/` 并更新 `helloagents/history/index.md`

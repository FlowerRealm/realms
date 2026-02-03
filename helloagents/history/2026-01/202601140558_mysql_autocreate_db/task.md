# 任务清单: MySQL 自动创建数据库（dev）

目录: `helloagents/plan/202601140558_mysql_autocreate_db/`

---

## 1. 启动链路修复
- [√] 1.1 在 `internal/store/db.go` 中识别 MySQL 1049（Unknown database），仅在 `env=dev` 时自动创建 DB 后重试连接
- [√] 1.2 在 `cmd/codex/main.go` 中传递 env 参数，保证只在 dev 生效

## 2. 文档与知识库
- [√] 2.1 更新 `README.md`：补充数据库不存在/端口冲突的处理说明
- [√] 2.2 更新 `helloagents/wiki/modules/codex.md`：记录本次变更与方案包链接

## 3. 验证
- [√] 3.1 运行 `go test ./...` 验证编译与测试通过

## 4. 变更记录
- [√] 4.1 更新 `helloagents/CHANGELOG.md`（Unreleased）
- [√] 4.2 迁移方案包至 `helloagents/history/2026-01/202601140558_mysql_autocreate_db/` 并更新 `helloagents/history/index.md`

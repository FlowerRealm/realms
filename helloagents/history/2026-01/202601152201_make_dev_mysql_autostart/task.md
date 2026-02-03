# 任务清单: make dev 自动启动 MySQL（docker）

目录: `helloagents/plan/202601152201_make_dev_mysql_autostart/`

---

## 1. 开发体验
- [√] 1.1 新增 `scripts/dev-mysql.sh`：当本机 `127.0.0.1:3306` 未监听时，自动 `docker compose up -d mysql`
- [√] 1.2 `scripts/dev.sh` 在启动 `air` 前调用 MySQL 自启动脚本
- [√] 1.3 `Makefile` 更新 `make dev` 说明文案

## 2. 文档与知识库
- [√] 2.1 更新 `README.md`：说明 `make dev` 会自动拉起 MySQL 容器与跳过方式
- [√] 2.2 更新 `helloagents/project.md`：补充本地开发热重载与 MySQL 自启动说明

## 3. 变更记录
- [√] 3.1 更新 `helloagents/CHANGELOG.md`（Unreleased）
- [√] 3.2 迁移方案包至 `helloagents/history/2026-01/202601152201_make_dev_mysql_autostart/` 并更新 `helloagents/history/index.md`

## 4. 验证
- [√] 4.1 运行 `go test ./...` 验证编译与测试通过


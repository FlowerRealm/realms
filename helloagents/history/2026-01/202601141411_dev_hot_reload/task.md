# 任务清单: 开发热重载（自动重启）

目录: `helloagents/history/2026-01/202601141411_dev_hot_reload/`

---

## 1. 开发热重载
- [√] 1.1 新增 `.air.toml`：监听 Go/模板/迁移/配置变更，自动 build + restart
- [√] 1.2 新增 `scripts/dev.sh`：一键启动 air（确保 config.yaml / CODEX_MASTER_KEY_BASE64 可用且稳定）
- [√] 1.3 更新 `README.md`：补充热重载启动说明与注意事项

## 2. 文档更新
- [√] 2.1 更新 `helloagents/wiki/modules/codex.md`：记录 dev 热重载用法
- [√] 2.2 更新 `helloagents/CHANGELOG.md`：记录本次新增

## 3. 测试
- [√] 3.1 执行 `go test ./...`

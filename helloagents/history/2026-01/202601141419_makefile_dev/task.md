# 任务清单: Makefile 开发管理

目录: `helloagents/history/2026-01/202601141419_makefile_dev/`

---

## 1. Makefile
- [√] 1.1 新增 `Makefile`：提供 `make dev/test/fmt/tools` 等常用目标
- [√] 1.2 对齐 `scripts/dev.sh`：提示优先使用 `make tools/dev`，并兼容本地 `.tmp/bin` 安装路径

## 2. 文档更新
- [√] 2.1 更新 `README.md`：推荐 `make dev` 作为开发热重载入口
- [√] 2.2 更新 `helloagents/wiki/modules/codex.md`：补充 Makefile 用法
- [√] 2.3 更新 `helloagents/CHANGELOG.md` 与 `helloagents/history/index.md`

## 3. 测试
- [√] 3.1 执行 `go test ./...`

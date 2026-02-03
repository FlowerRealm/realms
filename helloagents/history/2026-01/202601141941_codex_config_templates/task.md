# 任务清单: 移除命令行直连示例，改为 Codex 配置模板

目录: `helloagents/history/2026-01/202601141941_codex_config_templates/`

---

## 1. 文档与控制台示例

- [√] 1.1 更新 `README.md`：移除命令行调用示例，提供 Codex CLI `~/.codex/config.toml` 模板
- [√] 1.2 更新 Web 控制台（`/dashboard`、`/tokens`）：示例改为 Codex 配置模板（环境变量 + base_url）

## 2. 知识库与记录

- [√] 2.1 更新 `helloagents/wiki/api.md`：同步首页/Token 页“示例/模板”描述
- [√] 2.2 更新 `helloagents/CHANGELOG.md`：记录示例形态变更

## 3. 测试

- [√] 3.0 修复构建：清理 `internal/server/app.go` 未使用的 import
- [√] 3.1 执行 `go test ./...`

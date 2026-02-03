# 任务清单: 参考 88code 文档更新 Codex 配置模板

目录: `helloagents/history/2026-01/202601141956_align_88code_docs/`

---

## 1. README（跨平台）

- [√] 1.1 参考 Linux/Mac/Windows 文档，改用 `OPENAI_BASE_URL` / `OPENAI_API_KEY` 的配置说明
- [√] 1.2 补齐 Windows（PowerShell）写法与配置文件路径提示

## 2. Web 控制台示例

- [√] 2.1 更新 `/dashboard` 示例：改为与文档一致的配置模板（避免命令行直连示例）
- [√] 2.2 更新 `/tokens` 示例：同上

## 3. 知识库与记录

- [√] 3.1 更新 `helloagents/CHANGELOG.md` 与 `helloagents/wiki/api.md`（如需）

## 4. 测试

- [√] 4.0 修复构建：移除 managed_models legacy 上游字段依赖（统一以 `channel_models` 绑定调度）
- [√] 4.1 执行 `go test ./...`

# 任务清单: README 用户向配置教程整理

目录: `helloagents/plan/202601211620_readme_user_guide/`

---

## 1. README 用户向整理
- [√] 1.1 重写 `README.md`：以“配置教程 + 最小可用路径”为主，移除对未纳入仓库的内部文档链接，保留必要的安全提示。
- [√] 1.2 增加快速验证：提供 `curl /v1/responses` 示例与客户端环境变量示例（OpenAI SDK / CLI）。

## 2. 质量验证
- [√] 2.1 校验 Markdown 链接目标存在（README 内相对链接不应断链）。
- [√] 2.2 执行 `go test ./...`。

## 3. 版本管理
- [√] 3.1 提交 Git：
  - `0ed9b56` - `docs: user-facing README and repo docs`
  - `754e006` - `docs: clarify model setup in README`

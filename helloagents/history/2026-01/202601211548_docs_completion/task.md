# 任务清单: 文档补全（根目录协作文档 + 知识库索引完善）

目录: `helloagents/plan/202601211548_docs_completion/`

---

## 1. 根目录协作文档
- [√] 1.1 新增 `SECURITY.md`：漏洞上报渠道、披露流程、期望信息清单；覆盖 why.md「安全漏洞披露路径清晰 / 发现潜在安全问题」。
- [√] 1.2 新增 `CODE_OF_CONDUCT.md`：贡献者行为准则与执行方式；覆盖 why.md「新开发者快速定位文档入口」。
- [√] 1.3 新增 `CONTRIBUTING.md`：开发环境、测试命令、提交/PR 规范；覆盖 why.md「第一次克隆仓库」。
- [√] 1.4 新增 `LICENSE`：根据确认的许可证类型落地最终文本；覆盖 why.md「许可证边界明确」。

## 2. README 与知识库索引
- [√] 2.1 更新 `README.md`：添加“文档索引 / 贡献 / 安全”链接区，保持现有快速开始内容不变；覆盖 why.md「新开发者快速定位文档入口」。
- [√] 2.2 更新 `helloagents/wiki/overview.md`：模块表增加 `research`；快速链接增加根目录协作文档入口；覆盖 why.md「新开发者快速定位文档入口」。
- [√] 2.3 更新 `helloagents/project.md` 与 `helloagents/wiki/arch.md`：统一“敏感字段存储”描述，与当前实现一致（上游凭据明文入库、旧加密格式禁用）；覆盖 why.md「新开发者快速定位文档入口」。

## 3. 安全检查
- [√] 3.1 执行安全检查（按G9）：扫描仓库中是否出现真实 Key/Token（如 `sk-`、`rlm_`、`Authorization:` 等），确保文档仅含占位符。

## 4. 验证
- [√] 4.1 执行 `go test ./...`
- [√] 4.2 手动校验 Markdown 相对链接（README ↔ 知识库 ↔ 根目录协作文档）可跳转

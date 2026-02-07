# 社区协作模板（Issue / PR）

## 背景

为降低沟通成本并提高 triage 效率，项目引入 GitHub 原生模板体系：

- Issue Forms（结构化字段）
- Pull Request Template（统一提交流程）
- Issue Config（入口分流与安全提示）

## 文件位置

- `.github/ISSUE_TEMPLATE/bug_report.yml`
- `.github/ISSUE_TEMPLATE/feature_request.yml`
- `.github/ISSUE_TEMPLATE/config.yml`
- `.github/pull_request_template.md`

## 设计要点

### Issue 模板

- `Bug 报告`：要求最小可复现信息、期望/实际行为、环境信息，减少往返追问。
- `Feature 请求`：要求问题背景、方案、验收标准与兼容性说明，便于评估优先级。
- 统一前置检查：避免重复 Issue，避免泄露敏感信息。

### 安全分流

- 关闭空白 Issue（`blank_issues_enabled: false`）。
- 在 config 中提供安全漏洞私密披露入口：`/security/policy`。

### PR 模板

- 强制填写变更概述、关联 Issue、验证方式。
- 显式要求兼容性风险与回滚方案，降低上线风险。
- 检查清单要求文档同步（`README` / `docs` / `helloagents`）。

## 维护建议

- 新增重要功能域时，同步扩展 Feature 模板的“影响范围”选项。
- 若 CI 验证流程变化，更新 PR 模板中的默认验证项。
- 若安全披露渠道调整，优先更新 `config.yml` 的 contact links。

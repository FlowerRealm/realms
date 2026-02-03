# 任务清单: 用户可见界面与 README 全中文化

目录: `helloagents/plan/202601151953_ui_cn_readme/`

---

## 1. Web 用户控制台（用户可见）
- [√] 1.1 在 `internal/web/templates/base.html` 中将残留英文导航与页脚文案中文化（保留 `Realms` 等专有名词）
- [√] 1.2 在 `internal/web/templates/models.html` 中将表格列名与状态标签中文化（保留 `Model ID` 中的 `ID`，并统一计费相关术语）
- [√] 1.3 在 `internal/web/templates/tokens.html` 中统一 “Token/令牌” 口径，避免中英混用
- [√] 1.4 在 `internal/web/server.go` 中统一页面 Title（如：`API Tokens - Realms` → `API 令牌 - Realms`）

## 2. 管理后台（用户可见）
- [√] 2.1 在 `internal/admin/templates/base.html` 中将 `Admin`、`Signed in as`、页脚等固定文案中文化
- [√] 2.2 在 `internal/admin/templates/home.html` 中将 `UTC Time` 等固定文案中文化，并保持原有信息表达不变
- [√] 2.3 在 `internal/admin/templates/*.html` 中统一残留英文按钮/列名/提示（重点覆盖：channels/models/users/settings/usage 等页面）
- [√] 2.4 在 `internal/admin/server.go` 中统一页面 Title（保留 `Realms`）

## 3. 文档更新
- [√] 3.1 更新 `README.md`：将非专有名词英文替换为中文，统一“渠道/端点/凭证/令牌/管理后台”等术语，并保留示例代码块中的技术标识符

## 4. 安全检查
- [√] 4.1 执行安全检查（按G9：确认未误改任何配置 key / 环境变量 / API 路径 / 参数名；确认未引入敏感信息输出）

## 5. 测试与校验
- [√] 5.1 对目标文件执行英文残留扫描（仅允许保留清单内的专有名词/技术标识符）
- [√] 5.2 执行 `go test ./...`，确保无回归

## 6. 知识库与归档
- [√] 6.1 更新 `helloagents/wiki/modules/realms.md`：补充“用户可见文案中文化/术语口径”说明，并更新最后更新日期
- [√] 6.2 更新 `helloagents/CHANGELOG.md`（Unreleased 记录本次变更）
- [√] 6.3 迁移方案包至 `helloagents/history/2026-01/` 并更新 `helloagents/history/index.md`

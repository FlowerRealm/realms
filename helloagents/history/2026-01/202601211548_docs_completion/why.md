# 变更提案: 文档补全（根目录协作文档 + 知识库索引完善）

## 需求背景

- 现状：根目录仅有 `README.md`，缺少常见协作/合规文档（如 `LICENSE`、`SECURITY.md`、`CONTRIBUTING.md`、`CODE_OF_CONDUCT.md`）。
- 现状：知识库 `helloagents/wiki/overview.md` 的模块索引未覆盖已存在的 `research` 模块文档，且缺少对根目录协作文档的入口引用。
- 问题：新贡献者/新部署者难以快速找到“贡献方式 / 安全披露 / 许可边界”，文档入口不够集中。

## 变更内容

1. 在仓库根目录新增协作与合规文档（`LICENSE` / `SECURITY.md` / `CONTRIBUTING.md` / `CODE_OF_CONDUCT.md`）。
2. 更新 `README.md` 增加“文档入口 / 贡献 / 安全”索引（不改变现有快速开始内容）。
3. 补齐知识库索引（`overview` 模块表补充 `research`；快速链接补充根目录协作文档入口）。

## 影响范围

- **模块:** docs / knowledge-base（无代码行为变更）
- **文件:** `README.md`、`LICENSE`、`SECURITY.md`、`CONTRIBUTING.md`、`CODE_OF_CONDUCT.md`、`helloagents/wiki/overview.md`
- **API:** 无
- **数据:** 无

## 核心场景

### 需求: 新开发者快速定位文档入口
**模块:** docs

根目录 `README.md` 与知识库概览应提供一致的入口，避免“只看 README 找不到深层设计”或“只看知识库不知道怎么贡献”。

#### 场景: 第一次克隆仓库
- 能在 `README.md` 中找到：运行方式、文档索引、贡献入口与安全说明。
- 能从 `helloagents/wiki/overview.md` 进入：架构 / API / 数据 / 模块文档。

### 需求: 安全漏洞披露路径清晰
**模块:** docs

明确安全漏洞报告渠道和披露流程，避免在 issue/公开渠道泄漏细节。

#### 场景: 发现潜在安全问题
- 能在 `SECURITY.md` 找到上报方式与期望信息（复现步骤、影响范围等）。
- 文档中不出现真实密钥/令牌，仅使用占位符。

### 需求: 许可证边界明确
**模块:** docs

明确对外使用/分发的许可边界，降低法律/合规风险。

#### 场景: 想在其他项目复用代码
- 能在 `LICENSE` 中明确许可类型，或明确“未授权”。
- 若为私有仓库，文档避免误导为开源许可。

## 风险评估

- **风险:** `LICENSE` 类型未明确，写错会造成法律/合规风险。  
  **缓解:** 开发实施前先确认许可证类型（MIT/Apache-2.0/Proprietary/其他）。
- **风险:** 文档误写真实密钥/配置。  
  **缓解:** 全部使用占位符，完成后执行敏感信息扫描检查。


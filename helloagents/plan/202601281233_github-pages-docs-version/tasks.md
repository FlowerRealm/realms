# 任务清单: github-pages-docs-version

目录: `helloagents/plan/202601281233_github-pages-docs-version/`

---

## 任务状态符号说明

| 符号 | 状态 | 说明 |
|------|------|------|
| `[ ]` | pending | 待执行 |
| `[√]` | completed | 已完成 |
| `[X]` | failed | 执行失败 |
| `[-]` | skipped | 已跳过 |
| `[?]` | uncertain | 待确认 |

---

## 执行状态
```yaml
总任务: 23
已完成: 20
完成率: 87%
```

---

## 任务列表

### 0. 调研与规划
- [√] 0.1 阅读现状：`internal/version/*`、`README.md`、`.github/workflows/*`、`docs/*`
- [√] 0.2 明确目标：运行时版本 vs latest 发布版本拆分
- [√] 0.3 输出方案提案：`proposal.md`
- [√] 0.4 输出任务清单：`tasks.md`

### 1. GitHub Pages：文档站点（MkDocs）
- [√] 1.1 新增 `mkdocs.yml`（site_name/nav/repo_url/theme）
- [√] 1.2 新增 `docs/index.md`（从 `README.md` 拆分：简介 + 快速开始）
- [√] 1.3 整理部署文档：将 `docs/USAGE.md` 纳入站点导航（可重命名为 `deploy.md`）
- [√] 1.4 迁移/整理技术文档：从 `helloagents/wiki/{api,arch,data}.md` 抽取可公开内容到 `docs/`
- [√] 1.5 增加“版本与更新”页面：说明 `/api/version` 与 `version.json` 的差异、升级方式与发布策略

### 2. GitHub Pages：构建与部署（GitHub Actions）
- [√] 2.1 新增 `.github/workflows/pages.yml`：push 到 `master` 构建并部署到 Pages
- [√] 2.2 Pages 构建流程：安装 Python + mkdocs-material，`mkdocs build --strict`
- [√] 2.3 Pages 配置说明：在仓库 Settings -> Pages 选择 Source=GitHub Actions（补充到文档）

### 3. GitHub Pages：版本号存储（version.json）
- [√] 3.1 在 Pages workflow 中生成 `version.json`（站点根目录），字段包含 `latest/released_at/repo/docs/docker_image`（不包含 sha/commit）
- [√] 3.2 版本源：tag push 用 `${{ github.ref_name }}`；非 tag 用 `git describe --tags --abbrev=0`（需 `git fetch --tags`）
- [√] 3.3 可选：同时生成 `version.txt`（仅包含 latest），便于 shell/脚本快速读取

### 4. 版本号机制迁移（去除仓库内写文件版本）
- [√] 4.1 移除 `internal/version/version.txt` 与 `version.go` 的 embed 兜底逻辑
- [√] 4.2 调整 `internal/version/version_test.go`：去除对 embeddedVersion 的依赖，覆盖 dev/override 行为
- [√] 4.3 更新 `README.md` “版本号”章节：说明发布版本由 tag/构建注入，latest 见 GitHub Pages `version.json`
- [-] 4.4 可选：新增 `make build`（或脚本）用 `git describe --tags --always --dirty` 注入本地构建版本

### 5. 可选增强：UI 提示可升级
- [-] 5.1 方案 A（前端）：在页脚额外拉取 `version.json` 并提示“有新版本”（评估 CORS/超时/离线）
- [-] 5.2 方案 B（后端）：服务端定时拉取并缓存 latest（可配置开关），`/api/version` 增加 `latest_version`

### 6. 验证
- [√] 6.1 运行测试：`go test ./...`
- [√] 6.2 本地构建文档站点：`mkdocs build`（或通过 CI 确认 Pages workflow 正常）

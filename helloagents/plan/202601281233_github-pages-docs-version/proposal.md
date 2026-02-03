# 变更提案: github-pages-docs-version

## 元信息
```yaml
类型: 变更
方案类型: implementation
优先级: P1
状态: 已完成
创建: 2026-01-28
```

---

## 1. 需求

当前 Realms 以“仓库内写文件”的方式维护默认版本号：

- `internal/version/version.txt` 作为默认版本号来源（供 `GET /api/version` / `GET /healthz` 返回）。

希望达成：

1) **版本号 SSOT 外置**：使用 GitHub Pages 承载“最新版本号（latest）”与发布元信息，便于外部读取与自动化更新提示。
2) **项目文档站点**：使用 GitHub Pages 承载面向用户的项目文档（部署、配置、API、架构等）。
3) **减少手动版本维护成本**：发布新版本时不再需要在仓库里手动改动 `internal/version/version.txt`。

---

## 2. 现状（代码阅读要点）

### 2.1 当前版本号实现

- `internal/version/version.go`：
  - 构建注入：`realms/internal/version.Version/Commit/Date` 支持通过 `-ldflags -X` 注入。
  - 默认兜底：当 `Version` 为空或 `dev` 时，使用 `//go:embed version.txt` 的内容作为默认版本号。
- Docker 发布链路：
  - `Dockerfile` 通过 `-ldflags` 注入 `REALMS_VERSION/REALMS_COMMIT/REALMS_BUILD_DATE`。
  - `.github/workflows/docker.yml` 在 tag push 时传入 `REALMS_VERSION=${{ github.ref_name }}`，因此 **发布镜像已具备“用 tag 作为版本号”** 的能力。
- 对外版本接口：
  - `GET /api/version` 与 `GET /healthz` 返回 `a.version`（来自 `version.Info()`）。
  - Web 控制台与管理后台页脚会从 `/api/version` 获取并展示版本信息（见模板 `internal/web/templates/base.html`、`internal/admin/templates/base.html`）。

### 2.2 当前文档形态

- 面向用户的文档主要集中在：
  - `README.md`（入门、配置、API 入口、CI 等）
  - `docs/USAGE.md`（部署指南）
- 另有较完整的内部/项目说明文档：
  - `helloagents/wiki/*.md`（API/架构/数据等）

---

## 3. 方案

本方案把“运行时自身版本信息”和“最新发布版本号（latest）”拆开：

- **运行时自身版本**：继续由构建注入（release）或 `dev`（本地）提供，供 `/api/version`、`/healthz` 与日志输出使用。
- **最新发布版本（latest）**：由 GitHub Pages 公开提供，供外部查询、升级提示、文档站点显示使用。

### 3.1 GitHub Pages：文档站点（推荐：MkDocs + GitHub Actions）

- 采用 MkDocs（推荐主题：Material），以仓库 `docs/` 作为文档源目录。
- 文档信息架构建议：
  - 首页：项目简介 + 快速开始（从 `README.md` 精简/拆分）
  - 部署：沿用 `docs/USAGE.md`（可重命名为 `deploy.md`，或保持文件名并在 nav 中引用）
  - 架构 / 数据：从 `helloagents/wiki/arch.md`、`helloagents/wiki/data.md` 摘要化迁移到 docs
  - API：从 `helloagents/wiki/api.md` 提炼对外接口清单与示例
  - 版本与更新：解释 `/api/version`（运行时版本）与 `version.json`（latest）差异
- GitHub Actions 构建并部署到 GitHub Pages（Pages Source 选择 “GitHub Actions”）。

### 3.2 GitHub Pages：版本号存储（version.json）

在 GitHub Pages 站点根目录生成一个机器可读的版本文件，例如：

- `https://<owner>.github.io/<repo>/version.json`

建议字段：

```json
{
  "latest": "0.1.9",
  "released_at": "2026-01-28T12:33:00Z",
  "docker_image": "flowerrealm/realms:0.1.9",
  "repo": "FlowerRealm/realms",
  "docs": "https://<owner>.github.io/<repo>/"
}
```

生成策略（建议使用 Git tags 作为版本源）：

- tag push：`latest = github.ref_name`
- push 到 `master`：`latest = git describe --tags --abbrev=0`（确保 `git fetch --tags`）

这样版本号由 tag 驱动，不再依赖仓库内的 `version.txt` 手工维护。

### 3.3 版本号机制迁移（去除 version.txt 作为默认版本源）

将 `internal/version` 的默认行为调整为：

- 默认 `Version = "dev"`（本地 `go run` / 非注入构建）
- release 镜像 / release 构建：仍通过 `-ldflags` 注入 `Version/Commit/Date`
- 移除 `internal/version/version.txt` 的 embed 兜底逻辑与对应单测

迁移后：

- `/api/version` / `/healthz` 仍可用于识别当前实例的“构建指纹”
- “最新发布版本号”改由 GitHub Pages `version.json` 提供（外置 SSOT）

### 3.4 可选增强：在 UI 提示“可升级”

两种实现选项（按侵入性从低到高）：

1) 前端直接 fetch GitHub Pages 的 `version.json` 并与 `/api/version.version` 比较，提示“有新版本可用”（注意 CORS 与离线场景）。
2) 服务端增加一个轻量缓存（例如 10 分钟）去拉取 `version.json`，并在 `/api/version` 额外返回 `latest_version` 字段（避免浏览器跨域问题，但增加服务端外部依赖）。

---

## 4. 验收标准

- [√] GitHub Pages 文档站点：已提供 MkDocs 配置与 GitHub Actions 部署工作流（启用 Pages 后可访问）
- [√] GitHub Pages 版本号：`version.json` / `version.txt` 在 Pages workflow 中生成并随部署发布
- [√] 发布新版本时不再需要修改 `internal/version/version.txt`（已移除该兜底机制）
- [√] 版本信息仍可通过 `GET /api/version` 与 `GET /healthz` 获取（运行时构建指纹）
- [√] `go test ./...` 通过

# 变更提案: auto-version-detect

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

将项目的“版本号逻辑”改为**自动获取版本号**，减少人工维护与发布时的手动注入成本，确保：

- `/api/version` 与 `/healthz` 的 `version/date` 在常见构建方式下可自动得到合理值（**不输出 sha/commit**）
- 本地开发（`go run` / `make dev`）无需手工改文件或显式传参也能看到可追溯版本
- CI/发布（Docker/二进制）能够稳定产出 semver 版本（优先使用 tag）
- 前端（Web 控制台/管理后台）页脚不显示 sha/commit

---

## 2. 现状（已确认）

- `internal/version` 支持构建注入（`-ldflags -X realms/internal/version.Version/Date=...`），未注入时 `version=dev`。
- tag 发布 Docker 镜像时，GitHub Actions 已传入 `REALMS_VERSION=${{ github.ref_name }}`，可以得到 semver 版本（但本地构建/其他链路仍可能是 `dev`）。
- 项目已有 GitHub Pages 的 `version.json/version.txt`（latest 发布版本 SSOT），但它解决的是 “latest”，不是“当前实例自身版本”。

---

## 3. 目标定义（推荐的“自动获取”语义）

区分两类版本信息：

1) **实例自身版本（runtime build fingerprint）**：用于排障定位（`/api/version`、`/healthz`、日志）
2) **最新发布版本（latest）**：用于升级提示/外部查询（GitHub Pages `version.json`）

本提案只改 **1) 实例自身版本** 的获取方式：让它在常见构建场景下尽可能自动推导。

> 约束：对外 API 与前端展示不包含任何 sha/commit。

---

## 4. 方案（推荐：多级回退自动探测）

### 4.1 优先级（从强到弱）

1. **构建注入（保持兼容）**  
   若 `version.Version/Date` 已通过 `-ldflags -X` 注入，则直接使用（这是最可靠的发布链路）。

2. **Go BuildInfo（自动，零依赖）**  
   使用 `runtime/debug.ReadBuildInfo()` 读取 `vcs.time/vcs.modified`：
   - 自动填充 `date`（若可得）
   - `version` 保持为 `dev`（不拼接 sha；若为 dirty 可追加 `.dirty`）

3. **Git Describe（可选，仅开发场景）**  
   当存在 `.git` 且系统可执行 `git` 时，尝试用 tag 推导版本：
   - `git describe --tags --abbrev=0` 获取最近 tag（不包含 sha）
   - dirty 时可追加 `.dirty`

> 说明：生产镜像通常不包含 `.git`，因此发布版仍推荐走“构建注入”。该方案的价值在于：开发与非标准构建链路也能自动得到可追溯版本。

### 4.2 输出格式建议

- `version`：
  - 有 tag：`v0.1.9` 或 `0.1.9`（保持现有习惯，不强制带 `v`，以当前 tag 风格为准）
- 无 tag：`dev`（若 modified：`dev.dirty`）
- `date`：UTC RFC3339（例如 `2026-01-28T05:00:00Z`）

---

## 5. 工程落地范围

### 5.1 代码（internal/version）

- 新增自动探测逻辑（BuildInfo + 可选 git describe）
- 保持现有 `Info()` 对外接口不变，避免影响 server/web/admin 等调用方

### 5.2 构建链路（可选增强）

为 `go build`/`make`/本地 Docker build 提供“自动注入”能力，确保发布与本地构建一致：

- `scripts/version.sh`：输出 `REALMS_VERSION/REALMS_BUILD_DATE`（不生成 sha/commit）
- `make build`：默认自动注入（无需手填）
- Dockerfile：当 build args 为空时自动计算（前提：构建环境包含 `.git` 与 `git`）

---

## 6. 风险与注意事项

- `git describe` 依赖 git 与 `.git`，不能作为唯一来源；必须有 buildinfo/默认回退。
- Go BuildInfo 不能直接拿到 tag（只有 revision/time/modified），因此 “semver 版本”仍推荐在发布链路注入。
- 版本字符串格式一旦变更，会影响 UI 页脚展示与用户习惯；建议保持兼容并在文档中明确语义。
- 移除 `/api/version` 与 `/healthz` 的 `commit` 字段属于接口变更；但该字段当前仅用于 UI 页脚展示，且本需求明确要求不展示 sha/commit。

---

## 7. 验收标准

- [ ] 未传 `-ldflags` 的 `go run`：
  - 若当前工作区存在 git tag：`/api/version.version` 为最近 tag（不含 sha）
  - 否则：`/api/version.version` 为 `dev`（不含 sha）
- [ ] 通过 `-ldflags` 注入时：仍优先使用注入值（不被自动探测覆盖）
- [ ] `/api/version` 与 `/healthz` 不返回 `commit`（或恒为空/none），前端页脚不展示 commit
- [ ] `go test ./...` 通过，且 `internal/version` 覆盖“注入优先/自动回退”的核心用例
- [ ] 文档中说明 “实例自身版本 vs latest（Pages）” 的差异与用途

# 任务清单: playwright-web-e2e

目录: `helloagents/plan/202602011131_playwright-web-e2e/`

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
已完成: 22
完成率: 96%
```

---

## 任务列表

### 1. 口径与验收

- [√] 1.1 固化“100%覆盖率”口径：以 `helloagents/modules/web_spa.md` 路由清单为准，补充动态路由覆盖策略
  - 验证: proposal.md 的“验收标准/约束条件”明确且可执行

- [√] 1.2 明确 E2E 的本地运行入口（npm 脚本 + 可选 Makefile target）
  - 依赖: 1.1
  - 验证: 文档记录清晰，本地可一键运行

### 2. Go：E2E 初始化与启动器

- [√] 2.1 新增 `cmd/realms-e2e/main.go`：dev 配置启动 Realms（SQLite），监听 `127.0.0.1:${REALMS_E2E_PORT}`
  - 验证: `go run ./cmd/realms-e2e` 可启动并响应 `/healthz`

- [√] 2.2 启动时自动创建 SQLite 临时库并初始化 schema（`store.EnsureSQLiteSchema`）
  - 依赖: 2.1
  - 验证: 重复启动不会失败；schema 完整

- [√] 2.3 Seed 最小数据集：root 用户（固定账号/密码）+ 公告 + 工单（用于详情页路由）
  - 依赖: 2.2
  - 验证: UI 可用 root 登录；公告/工单详情页可正常访问

- [√] 2.4 提供可控的输出/参数：端口、db 路径、frontend dist dir（env 或 flag 二选一）
  - 依赖: 2.1
  - 验证: CI 与本地均可配置运行

### 3. Node：Playwright 测试框架

- [√] 3.1 在 `web/package.json` 增加 `@playwright/test` 依赖与脚本（`test:e2e` 等）
  - 验证: `npm --prefix web run test:e2e -- --help` 可用

- [√] 3.2 新增 `web/playwright.config.ts`：仅 chromium；接入 `webServer` 启动 Go E2E 服务并等待 `/healthz`
  - 依赖: 3.1, 2.1
  - 验证: `npx playwright test --list` 正常；`webServer` 启动/关闭无残留进程

- [√] 3.3 增加认证复用：一次登录生成 `storageState`，后续用例复用（root 账号）
  - 依赖: 3.2, 2.3
  - 验证: 多个用例运行时不重复登录；失败时可产出 trace

- [√] 3.4 覆盖 public 路由（至少）：`/login`、`/register`
  - 依赖: 3.3
  - 验证: 页面标题/关键表单元素可见；注册页在禁用/启用状态下表现符合预期

- [√] 3.5 覆盖用户侧路由（以 `web_spa.md` 为准）
  - 依赖: 3.3
  - 验证: 每个路由至少 1 条 smoke case

- [√] 3.6 覆盖管理侧路由（以 `web_spa.md` 为准）
  - 依赖: 3.3
  - 验证: root 用户可进入 `/admin` 并覆盖各页面路由

- [√] 3.7 补齐动态路由代表用例（例如：`/announcements/:id`、`/tickets/:id`、`/admin/*/:id`）
  - 依赖: 2.3, 3.5, 3.6
  - 验证: 至少 1 个真实存在的实体可被详情页访问（非纯 404）

- [√] 3.8 视情况补充稳定 selector（仅必要时加 `data-testid`，优先用语义/label 选择器）
  - 依赖: 3.4-3.7
  - 验证: E2E 运行稳定（降低 flake）

### 4. CI：GitHub Actions 集成

- [√] 4.1 更新 `.github/workflows/ci.yml`：新增 `e2e-web` job（Go + Node + Playwright chromium）
  - 验证: CI 在无任何 secrets 的情况下可运行

- [√] 4.2 在 CI 中构建前端（`npm --prefix web run build`）并运行 E2E
  - 依赖: 4.1
  - 验证: 后端可从 `web/dist` 提供 SPA；E2E 可访问页面

- [√] 4.3 失败时上传 artifacts（Playwright report/trace/screenshot）
  - 依赖: 4.1
  - 验证: 故意制造失败时，Actions 可下载报告定位问题

### 5. 文档与知识库同步

- [√] 5.1 更新 `helloagents/modules/ci_github_actions.md`：记录 `e2e-web` job 与本地复现命令
  - 验证: 文档无真实密钥；命令可用

- [√] 5.2 更新 `helloagents/modules/web_spa.md`：增加 Playwright E2E 约定（目录结构/selector 约定/新增路由需加用例）
  - 验证: 新人可按文档补用例

- [√] 5.3 补齐知识库入口文件（如缺失则创建）：`helloagents/INDEX.md`、`helloagents/context.md`
  - 验证: 知识库结构完整，可作为导航入口

### 6. 验收与稳定性

- [√] 6.1 本地验证：`npm --prefix web run test:e2e`
  - 验证: 可重复通过（至少连续 3 次）

- [?] 6.2 CI 验证：push 后 `ci` workflow 全绿（含 `e2e-web`）
  - 验证: GitHub Actions 页面可见成功记录

- [√] 6.3 稳定性与耗时控制：必要时将 E2E worker 固定为 1 并设置合理超时
  - 验证: CI 总耗时可接受，flake 显著降低

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|
| 6.2 | [?] | 本地无法直接验证 GitHub Actions；需 push 后观察 `ci` workflow |

# 变更提案: playwright-web-e2e

## 元信息
```yaml
类型: 新功能
方案类型: implementation
优先级: P1
状态: 草稿
创建: 2026-02-01
```

---

## 1. 需求

### 背景
- 当前仓库已有 Go 层 E2E：`tests/e2e/codex_cli_test.go`（Codex CLI -> Realms -> upstream），用于覆盖代理链路与 usage 落库口径。
- 仍缺少浏览器层 E2E 来覆盖 `web/` SPA 的关键页面/路由，UI/路由/鉴权/回归风险主要依赖人工发现。

### 目标
- 引入 Playwright（Chromium 桌面）E2E，用于覆盖 `web/` SPA 路由（以 `helloagents/modules/web_spa.md` 的路由清单为准）。
- 在 CI（GitHub Actions）中必跑，且不依赖外部 secrets（使用本地 SQLite + seed）。
- 提供稳定初始化：E2E 启动时生成临时 SQLite 并 seed root 用户及必要样例数据，使测试可重复、可调试（初期 worker=1，优先稳定）。
- 在失败时产出 Playwright trace/screenshot/report，便于排查。

### 约束条件
```yaml
时间约束: 无
性能约束: CI 单次 E2E 目标 < 10 min（仅 chromium + 单 worker）
兼容性约束: 仅覆盖 Chromium Desktop；不引入 Firefox/WebKit
业务约束:
  - 不引入真实支付/真实上游依赖（测试环境完全自洽）
  - 不提交任何真实密钥/凭据
```

### 验收标准
- [ ] `npm --prefix web run test:e2e` 在本地可稳定通过（默认 headless chromium）。
- [ ] GitHub Actions 新增 `e2e-web` job 通过（每次 push 触发），失败时上传 Playwright artifacts。
- [ ] 覆盖率=100%：`helloagents/modules/web_spa.md` 中列出的 SPA 路由均有对应 E2E 用例（动态路由至少 1 个代表用例）。
- [ ] 初始化稳定：E2E 启动脚本自动创建 SQLite + seed root 用户，测试不依赖任何预置环境。

---

## 2. 方案

### 技术方案
采用“方案 3：Node + @playwright/test + Go seed/启动脚本”。

Go 侧新增 E2E 专用启动器（建议路径：`cmd/realms-e2e/main.go`）：
- 创建临时 SQLite（或可指定路径），调用 `store.EnsureSQLiteSchema` 初始化 schema。
- Seed：root 用户（固定账号/密码），以及公告/工单等用于动态路由的最小数据集（避免详情页全部 404）。
- 以 dev 配置启动 Realms（使用 `server.NewApp`，监听 `127.0.0.1:${PORT}`）。
- 健康检查：复用 `/healthz`，供 Playwright `webServer` 等待。

Node 侧在 `web/` 引入 `@playwright/test`：
- 新增 `web/playwright.config.ts`：
  - `projects` 仅 chromium
  - `webServer`：启动 `go run ./cmd/realms-e2e` 并等待 `/healthz`
  - `reporter`：html + list；`trace: on-first-retry`；`screenshot: only-on-failure`
- 使用 `storageState` 复用登录态（减少每个用例重复登录）
- 用例按路由分组（public/app/admin），每个路由至少一个 smoke case（页面可加载 + 关键标题/元素存在）

### 影响范围
```yaml
涉及模块:
  - web/: 增加 Playwright 配置与 E2E 用例
  - cmd/: 增加 E2E seed/启动器（仅测试用途）
  - .github/workflows/ci.yml: 增加 e2e-web job
  - helloagents/modules/: 更新 CI 与 SPA 测试约定文档
预计变更文件: 10-20
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| E2E 用例易 flake（异步加载/动画/网络请求） | 中 | 统一 wait 策略（networkidle/关键元素）、尽量用稳定 selector；必要时补 data-testid |
| CI 运行 Playwright 依赖较重（浏览器安装） | 中 | 仅 chromium；使用 `npx playwright install --with-deps chromium`；缓存 npm |
| 初始化数据不足导致部分页面报错 | 中 | seed 最小数据集（announcement/ticket 等）；用例允许 empty state |
| 端口/进程清理不当导致 CI hang | 中 | 使用 Playwright `webServer` 管理；设置超时；确保进程退出 |

---

## 3. 技术设计（可选）

> 涉及架构变更、API设计、数据模型变更时填写

### 架构设计
```mermaid
flowchart LR
  PW[Playwright @playwright/test] -->|HTTP| R[Realms E2E Server (Go)]
  R --> DB[(SQLite temp DB)]
  R -->|serve| SPA[web/dist SPA]
```

### API设计
本方案不新增对外 API；仅新增一个测试启动入口（Go command）。

### 数据模型
无新增数据模型。

---

## 4. 核心场景

> 执行完成后同步到对应模块文档

### 场景: Root 登录后覆盖所有 SPA 路由
**模块**: web/ + router/ + cmd/realms-e2e  
**条件**: E2E seed 已创建 root 用户并启动服务  
**行为**: Playwright 登录 -> 依次访问 `web_spa.md` 路由清单中的页面（含 admin）  
**结果**: 页面渲染成功、关键元素可见、无致命错误提示

---

## 5. 技术决策

> 本方案涉及的技术决策，归档后成为决策的唯一完整记录

### playwright-web-e2e#D001: 采用 Node + @playwright/test + Go seed/启动脚本
**日期**: 2026-02-01
**状态**: ✅采纳
**背景**: 需要在 CI 稳定运行浏览器 E2E，且测试环境可自洽（无外部依赖/可初始化）
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: Go + playwright-go | 与 Go 测试一致、无需 Node | 生态/trace/report 体验较弱；团队更难复用 Playwright 标准实践 |
| B: Node + @playwright/test | 官方最佳体验，report/trace 完整 | 需要处理服务启动/初始化 |
| C: Node + @playwright/test + Go seed/启动脚本 | 兼得官方体验 + 初始化可控 | 实现更重（新增启动器/seed） |
**决策**: 选择方案 C
**理由**: 覆盖率目标高且需 CI 稳定；初始化必须可控；同时希望保留 Playwright 原生调试与报告能力
**影响**: `web/` 增加 Playwright；`cmd/` 增加测试启动器；CI 增加 job

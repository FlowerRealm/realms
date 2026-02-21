# Realms Web（Vite + React）

该目录是 Realms 的前端工程（SPA），对齐 new-api 的“页面路由由前端负责，后端仅做 API + SPA fallback”的组织方式。

## 本地开发

1) 启动后端（默认 8080）：

```bash
go run ./cmd/realms
```

2) 启动前端（默认 5173，已配置 proxy 到 8080）：

```bash
npm install
npm run dev
```

访问：`http://localhost:5173/login`

## 构建

```bash
npm run lint
npm run build
```

构建产物输出到 `web/dist`：
- 同源部署：后端会从 `FRONTEND_DIST_DIR`（默认 `./web/dist`）提供静态资源，并对 `/login` 等路径回落到 `index.html`
- Docker：镜像构建期会先 `npm run build`，再用 `-tags embed_web` 将 `web/dist` embed 到二进制

## 环境变量（可选）

- `VITE_API_BASE_URL`：API baseURL（默认空字符串，同源）

## E2E（Playwright，可选）

Playwright 用于前端的**组件级交互**与**跨页面流程**回归（见 `web/e2e/`）。  
说明：CI 默认不跑 Playwright（避免浏览器依赖带来的不稳定），CI 的 Web 验证改为 `curl` 冒烟（见下方）。

默认（seed 模式，自动启动 `cmd/realms-e2e`，使用内置种子数据）：

```bash
npm run test:e2e
```

真实数据模式（连接你本地已运行的 Realms 服务/数据库，不启动内置 seed 服务）：

```bash
REALMS_E2E_PROFILE=real \
REALMS_E2E_EXTERNAL_SERVER=1 \
REALMS_E2E_BASE_URL=http://127.0.0.1:8080 \
REALMS_E2E_USERNAME=<root用户名> \
REALMS_E2E_PASSWORD=<root密码> \
npm run test:e2e:ci
```

若你仍想使用 `cmd/realms-e2e` 启动器，但复用已有 SQLite 数据与上游配置（跳过自动 seed）：

```bash
REALMS_E2E_SKIP_SEED=1 \
REALMS_E2E_DB_PATH=/path/to/your.sqlite \
npm run test:e2e:ci
```

若你想继续使用 `cmd/realms-e2e` 的 seed 数据，但把请求转发到真实上游（CI 推荐）：

```bash
REALMS_E2E_UPSTREAM_BASE_URL=https://api.openai.com/v1 \
REALMS_E2E_UPSTREAM_API_KEY=sk-*** \
REALMS_E2E_BILLING_MODEL=gpt-5.2 \
REALMS_E2E_ENFORCE_REAL_UPSTREAM=1 \
npm run test:e2e:ci
```

若你希望在前端“模型列表”相关用例中覆盖多个模型，可在 seed 模式下用逗号分隔一次性 seed 多个模型：

```bash
REALMS_E2E_BILLING_MODELS=gpt-5.2,gpt-5.2-mini,gpt-5.2-nano \
npm run test:e2e:ci
```

从仓库根目录执行统一检查集（推荐，本地/CI 同口径）：

```bash
make ci
```

若你已配置 `REALMS_CI_UPSTREAM_BASE_URL/REALMS_CI_UPSTREAM_API_KEY/REALMS_CI_MODEL`，则 `make ci` 会默认跑真实上游集成回归（等价于 `make ci-real`）。

## CI Web 冒烟（curl）

CI 的 Web 部分不跑 Playwright，改为：
- 构建 `web/dist`
- 启动 `cmd/realms-e2e`
- `curl` 校验：`/healthz`、`/`、`/assets/realms_icon.svg`

入口：
- `scripts/ci.sh`（seed/fake upstream）
- `scripts/ci-real.sh`（seed + real upstream 配置）

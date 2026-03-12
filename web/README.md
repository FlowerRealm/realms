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
- 同源部署：后端会提供嵌入后的静态资源，并对 `/login` 等路径回落到 `index.html`
- Docker：镜像构建期会先 `npm run build`，再用 `-tags embed_web` 将 `web/dist` embed 到二进制
- 运行时不再支持通过环境变量切换磁盘目录或外置前端 URL

## 环境变量（可选）

- `VITE_API_BASE_URL`：API baseURL（默认空字符串，同源）

## Web 冒烟（curl）

本仓库的 CI Web 校验默认使用 `curl` 做最小冒烟（构建 `web/dist` + 启动 `cmd/realms-e2e` + 校验关键路由）。

从仓库根目录执行统一检查集（推荐，本地/CI 同口径）：

```bash
make ci
```

若你已配置 `REALMS_CI_UPSTREAM_BASE_URL/REALMS_CI_UPSTREAM_API_KEY/REALMS_CI_MODEL`，则 `make ci` 会默认跑真实上游集成回归（等价于 `make ci-real`）。

你也可以只跑 Web 冒烟（本地）：

```bash
go run ./cmd/realms-e2e
curl -fsS "http://127.0.0.1:18181/healthz"
```

`cmd/realms-e2e` 常用环境变量：

```bash
REALMS_E2E_BASE_URL="http://127.0.0.1:18181"         # 对外 baseURL（用于脚本/测试）
REALMS_E2E_ADDR="127.0.0.1:18181"                    # 监听地址（默认同上）
REALMS_E2E_SKIP_SEED="1"                             # 跳过自动 seed
REALMS_E2E_DB_PATH="/path/to/your.sqlite"            # 复用 SQLite
REALMS_E2E_UPSTREAM_BASE_URL="https://api.openai.com/v1"
REALMS_E2E_UPSTREAM_API_KEY="sk-***"
REALMS_E2E_BILLING_MODEL="gpt-5.2"
REALMS_E2E_ENFORCE_REAL_UPSTREAM="1"
```

## CI Web 冒烟（curl）

CI 的 Web 部分使用 `curl` 冒烟：
- 构建 `web/dist`
- 启动 `cmd/realms-e2e`
- `curl` 校验：`/healthz`、`/`、`/assets/realms_icon.svg`

入口：
- `scripts/ci.sh`（seed/fake upstream）
- `scripts/ci-real.sh`（seed + real upstream 配置）

## UI E2E（已移除）

历史上仓库曾使用浏览器 UI E2E 测试（`web/e2e` + 测试框架），现已移除以减少 CI 依赖与不稳定因素。

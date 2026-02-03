# CI（GitHub Actions）

本模块记录 Realms 的 CI 设计与 Codex CLI E2E 用例（含真实上游与 fake upstream 两条链路）。

## 工作流

- Workflow 文件：`.github/workflows/ci.yml`
- 触发：`push`（每次提交）
- Job：
  - `test`：运行 `go test ./...`
  - `e2e-codex`：安装 Codex CLI，运行 `go test ./tests/e2e -run TestCodexCLI_E2E -count=1`
    - 该 job 会设置 `REALMS_CI_ENFORCE_E2E=1`，确保 E2E 不会在本地开发的 `go test ./...` 中被默认执行
  - `e2e-web`：Playwright Web E2E（Chromium Desktop）
    - 构建 `web/dist` 后运行：`npm --prefix web run test:e2e:ci`
    - 通过 `cmd/realms-e2e` 自动创建临时 SQLite + seed 数据，不依赖外部 secrets
    - 失败时上传：`web/playwright-report`、`web/test-results`

## GitHub Pages（文档站点）

- Workflow 文件：`.github/workflows/pages.yml`
- 触发：`push` 到 `master`、push tag、或手动触发
- 构建：
  - 安装 `mkdocs` + `mkdocs-material`
  - 通过 `actions/configure-pages` 取得 `base_url`，生成临时 `mkdocs.pages.yml` 注入 `site_url`（MkDocs CLI 没有 `mkdocs build --site-url` 选项）
  - 执行 `mkdocs build --strict --config-file mkdocs.pages.yml` 输出到 `site/`
  - 额外写入 `site/version.json` 与 `site/version.txt`（用于文档站点展示“latest 版本/发布时间”等信息）
- 部署：`actions/deploy-pages@v4`

## Secrets（仓库级）

> 仅以占位符形式记录，禁止提交真实密钥。

- `REALMS_CI_UPSTREAM_BASE_URL`：上游 OpenAI 兼容 `base_url`（例如 `https://api.openai.com` 或 `https://api.openai.com/v1`）
- `REALMS_CI_UPSTREAM_API_KEY`：上游 API Key（例如 `sk-***`）
- `REALMS_CI_MODEL`：E2E 使用的模型名（例如 `gpt-4.1-mini`）

## E2E 用例（Codex CLI → Realms）

- 测试文件：`tests/e2e/codex_cli_test.go`
- 用例 1（真实上游链路）：`TestCodexCLI_E2E`
  - 依赖：`REALMS_CI_UPSTREAM_BASE_URL/REALMS_CI_UPSTREAM_API_KEY/REALMS_CI_MODEL`
  - 断言：
    - 执行两次 `codex exec`（同一代码上下文），输出包含 `package main` 与 `REALMS_CI_OK`
    - SQLite `usage_events` 应为 2 条 committed 事件
    - `input_tokens/output_tokens` 必须存在且 > 0
    - 第二次请求必须命中缓存：`cached_input_tokens > 0`
    - 所有 `cached_*_tokens`（若上游返回）必须 ≥ 0 且不超过总 Token
- 用例 2（fake upstream 稳定缓存用例）：`TestCodexCLI_E2E_FakeUpstream_Cache`
  - 目的：稳定验证 Realms 对 `usage.input_tokens_details.cached_tokens` 的解析与落库口径（在不依赖真实上游具体行为的情况下覆盖该路径）
  - 断言：
    - 执行两次 `codex exec`（同 prompt），第二次必须命中缓存：`cached_input_tokens > 0`
    - SQLite `usage_events` 应为 2 条 committed 事件，且 Token 口径一致

## 本地复现

Web E2E（Playwright，Chromium Desktop）：

```bash
npm --prefix web ci
cd web && npx playwright install chromium
npm --prefix web run test:e2e
```

Codex CLI E2E（需要真实上游 secrets）：

```bash
npm install -g @openai/codex
export REALMS_CI_UPSTREAM_BASE_URL="https://api.openai.com"
export REALMS_CI_UPSTREAM_API_KEY="sk-***"
export REALMS_CI_MODEL="gpt-4.1-mini"
go test ./tests/e2e -run TestCodexCLI_E2E -count=1
```

仅运行 fake upstream 缓存用例（无需真实上游 secrets）：

```bash
npm install -g @openai/codex
export REALMS_CI_ENFORCE_E2E=1
export REALMS_CI_MODEL="gpt-4.1-mini"
go test ./tests/e2e -run TestCodexCLI_E2E_FakeUpstream_Cache -count=1
```

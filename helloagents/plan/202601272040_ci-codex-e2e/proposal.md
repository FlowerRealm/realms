# 变更提案: ci_codex_e2e

## 元信息
```yaml
类型: 流程/测试
方案类型: implementation
优先级: P1
状态: ✅完成
创建: 2026-01-27
```

---

## 1. 需求

### 背景
需要为 Realms 增加“真实链路”的 CI：使用 GitHub Secrets 提供上游 `base_url/api_key` 与 `model`，在 CI 中自动完成：
1) 初始化上游渠道（OpenAI 兼容）
2) 创建测试用户、增加余额、创建数据面 Token（`rlm_...`）
3) 安装 Codex CLI、写入 `~/.codex/config.toml`
4) 通过 `codex exec` 发起一次最小请求，验证链路：Codex CLI → Realms → 上游

### 目标
- 每次 push 自动跑：`go test ./...` + “Codex CLI E2E”
- E2E 全流程自动化，无需人工在 Web 控制台点击
- 不在日志/代码中泄露任何真实密钥（仅使用 GitHub Secrets 注入）

### 约束条件
```yaml
触发: push（每次 commit）
依赖: GitHub Secrets (base_url/api_key/model)
测试形态: SQLite + 内存启动 HTTP server（避免 MySQL/容器依赖）
客户端: Codex CLI（npm 全局安装）
```

### 验收标准
- [ ] GitHub Actions 在每次 push 触发
- [ ] 单测 `go test ./...` 通过
- [ ] E2E job 在配置 secrets 后可稳定通过：
  - 自动建好 channel/endpoint/credential/model binding/user/balance/token
  - `codex exec` 能通过 Realms 成功返回预期标记
- [ ] 本地开发环境未配置 secrets/未安装 codex 时，E2E 测试自动 skip（不影响 `go test ./...`）

---

## 2. 方案

### 技术方案概览
1) 新增 `tests/e2e/codex_cli_test.go`：
   - 读取 env（兼容多组变量名）
   - 使用 SQLite 初始化 schema 并直接写入必要数据（绕开 Web 登录/CSRF）
   - `httptest.NewServer` 启动 Realms
   - 生成 `~/.codex/config.toml` 指向 Realms 的 `/v1`，并用 `OPENAI_API_KEY=rlm_...` 鉴权
   - 执行 `codex exec` 并校验输出包含 `REALMS_CI_OK`
2) 新增 `.github/workflows/ci.yml`：
   - job1: `go test ./...`
   - job2: 安装 Node + `npm i -g @openai/codex`，注入 secrets，运行 `go test ./tests/e2e`

### Secrets 约定（仓库级）
```yaml
REALMS_CI_UPSTREAM_BASE_URL: 上游 OpenAI 兼容 base_url（可含或不含 /v1）
REALMS_CI_UPSTREAM_API_KEY: 上游 API Key
REALMS_CI_MODEL:            用于 E2E 的模型名
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| 上游不稳定/限流导致 E2E 波动 | 中 | 选择成本低/稳定的模型；请求尽量小；必要时增加重试/超时控制 |
| secrets 泄露到日志 | 高 | 测试输出做 redact；workflow 不回显 secrets；文档仅用占位符 |
| Codex CLI 行为变更 | 中 | 通过固定配置文件 + 最小 prompt 约束；必要时锁定 npm 包版本 |

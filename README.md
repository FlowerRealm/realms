# Realms（OpenAI 兼容 API 中转 + Web 控制台）

Realms 是一个 Go 单体服务（Gin），对外提供 **OpenAI 兼容** 的 `/v1/*`（数据面），并提供一个 Web 控制台（管理面）用于配置上游渠道、路由策略与用量查询等。

你可以用它做什么：
- 作为 OpenAI SDK / Codex CLI 的 `base_url` 中转层（支持 `POST /v1/responses` SSE 透传）
- 在 Web 控制台里管理 Token（正常模式）或管理 Key（自用模式），并查看用量与请求明细
- 在管理后台里管理上游渠道（OpenAI 兼容 base_url / Codex OAuth）与路由策略

---

## 0) 先选一种“运行形态”

- **Server（正常模式，推荐）**：部署/多人使用，默认开启账号/Token/模型目录等完整功能域。
- **Desktop（自用模式）**：Electron 桌面版，适合个人自用（固定 `127.0.0.1:8080`，启动内置后端并打开页面）。

> 说明：自用模式也可以以 Server 形态运行（`REALMS_SELF_MODE_ENABLE=true`）。Desktop 只是把“自用模式 Server”封装成桌面应用。

---

## 1) 最小开始：Server（正常模式，推荐：Docker Compose）

### 1.1 一键启动（可复制运行）

```bash
git clone "https://github.com/FlowerRealm/realms.git"
cd "realms"

cp ".env.example" ".env"
docker compose pull realms
docker compose up -d

curl -fsS "http://127.0.0.1:18080/healthz"
```

默认端口是 `127.0.0.1:18080`（可用 `.env` 的 `REALMS_HTTP_PORT` 覆盖；见 `docker-compose.yml`）。

---

## 2) 最小开始：Desktop（自用模式）

Desktop 用于把 Realms 以“自用模式”封装成桌面应用（启动本地后端并打开页面）。

关键约束（Desktop）：
- 固定监听：`127.0.0.1:8080`（端口占用则启动失败，端口不会自动改动）
- 固定 base_url：`http://127.0.0.1:8080/v1`
- 后端强制启用自用模式：`REALMS_SELF_MODE_ENABLE=true`
- 自用模式鉴权：首次打开 `/login` 设置 **管理 Key**；之后外部客户端与管理面都使用该 Key（`Authorization: Bearer <key>` 或 `x-api-key`）

### 2.1 直接使用安装包（推荐）

从 GitHub Releases 下载并安装对应平台的安装包，打开应用即可（更新源也默认使用 GitHub Releases）。

### 2.2 从源码开发运行（本机）

前置：Go + Node.js + npm

```bash
make desktop-dev
```

### 2.3 打包当前平台安装包

```bash
make desktop-dist
```

产物默认输出到：`desktop/dist/`。

更多细节见：`docs/USAGE.md`（“桌面版（Electron，自用模式）”章节）与 `desktop/README.md`。

---

## 3) 客户端接入（OpenAI SDK / Codex CLI）

### 3.1 环境变量（最常用）

- Server（正常模式）：`OPENAI_API_KEY` 填你在 `/tokens` 创建的 `sk_...`
- Desktop / 自用模式：`OPENAI_API_KEY` 填你在 `/login` 设置的管理 Key

Linux/macOS（bash/zsh）：

```bash
export OPENAI_BASE_URL="http://127.0.0.1:18080/v1" # Desktop 改为 http://127.0.0.1:8080/v1
export OPENAI_API_KEY="sk_..."
```

Windows（PowerShell）：

```powershell
$env:OPENAI_BASE_URL = "http://127.0.0.1:18080/v1" # Desktop 改为 http://127.0.0.1:8080/v1
$env:OPENAI_API_KEY = "sk_..."
```

### 3.2 （可选）Codex 配置文件示例

Linux/macOS：`~/.codex/config.toml`；Windows：`%USERPROFILE%\\.codex\\config.toml`

```toml
disable_response_storage = true
model_provider = "realms"
model = "gpt-5.2"

[model_providers.realms]
name = "Realms"
base_url = "http://127.0.0.1:18080/v1" # Desktop 改为 http://127.0.0.1:8080/v1
wire_api = "responses"
requires_openai_auth = true
```

---

## 4) 本地开发（贡献者）

开发热重载（正常模式，固定 `127.0.0.1:8080`）：

```bash
make dev
```

运行测试：

```bash
go test ./...
```

更多内容见：`CONTRIBUTING.md`。

---

## 5) 文档与配置索引

- 环境变量示例：`.env.example`
- 可直接复制运行的部署命令：`docs/USAGE.md`
- 前后端分离说明：`docs/frontend.md`
- 桌面版说明：`desktop/README.md`
- 贡献指南：`CONTRIBUTING.md`
- 安全政策：`SECURITY.md`
- 行为准则：`CODE_OF_CONDUCT.md`
- 许可证：`LICENSE`

---

<details>
<summary>深入：运行模式（正常模式 vs 自用模式）</summary>

- 正常模式：`REALMS_SELF_MODE_ENABLE=false`（默认）  
  面向完整功能（订阅/余额/支付/工单等），需要账号系统与更多配置项。

- 自用模式：`REALMS_SELF_MODE_ENABLE=true`  
  适合个人/小团队自用：不提供账号/Token/系统设置/OAuth 等功能域；管理后台通过 `/login` 的管理 Key 解锁；数据面与管理面 API 均要求携带该 Key。

</details>

<details>
<summary>深入：安全说明（重要）</summary>

- 上游 API 密钥 / OAuth 令牌 **明文入库**（BLOB）。
- 用户数据面令牌 / Web 会话 **仅存 hash**（SHA256）。
- `base_url` 会做最小校验（协议/Host/DNS）。

</details>

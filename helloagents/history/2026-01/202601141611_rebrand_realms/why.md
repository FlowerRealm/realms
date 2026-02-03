# 变更提案: Realms 品牌改名（移除 Codex 作为产品名）

## 需求背景
当前仓库与站点对外品牌期望为 **Realms**，但控制台/文档/构建产物仍存在将服务称为 **Codex** 的遗留，造成：
- 用户认知混乱（Realms vs Codex）
- 部署与运维层面的命名不一致（环境变量/二进制名/镜像入口）

本次变更目标：将本服务的**产品/站点品牌**统一为 **Realms**。

> 约束：`Codex OAuth` 等属于上游/协议的技术术语 **保留**（不是产品名）。

## 变更内容
1. 服务入口与构建产物改名：将 `cmd/codex`/二进制 `codex` 迁移为 `cmd/realms`/二进制 `realms`。
2. 配置与环境变量改名：将 `CODEX_*` 环境变量前缀改为 `REALMS_*`（不保留兼容别名）。
3. Web 控制台与管理后台 UI 改名：所有面向用户的 “Codex” 品牌字样改为 “Realms”。
4. 文档与知识库同步：更新 README 与 `helloagents/` 知识库中对服务名称/入口的描述。
5. 默认开发环境资源改名：将示例 MySQL 数据库名从 `codex` 调整为 `realms`（不做迁移兼容）。

## 影响范围
- **模块:**
  - `cmd/*`（服务入口）
  - `internal/config`（环境变量覆盖）
  - `internal/web`（Web 控制台 UI/会话 cookie）
  - `internal/admin`（管理后台 UI）
  - `scripts/*`、`.air.toml`、`Dockerfile`、`docker-compose.yml`（开发与构建）
  - 文档：`README.md`、`helloagents/*`
- **API:**
  - 数据面 OpenAI 兼容接口（`/v1/*`）不变
  - 管理后台与控制台页面的文案/标题变更
- **数据:**
  - 不修改线上数据结构；仅调整开发示例默认库名

## 核心场景

### 需求: 品牌统一为 Realms
**模块:** Web 控制台 / 管理后台 / 文档

#### 场景: 用户访问登录页与后台
- 登录页标题显示 “Realms”
- 管理后台显示 “Realms Admin”
- 页脚/导航不再出现 “Codex” 品牌字样

#### 场景: 开发者启动服务
- `go run ./cmd/realms -config config.yaml` 可启动
- 开发热重载与 Docker 构建产物名称为 `realms`
- 使用 `REALMS_MASTER_KEY_BASE64` 完成敏感字段加密（旧 `CODEX_*` 不再生效）

## 风险评估
- **风险:** 破坏性改名（命令/环境变量/默认数据库名变更），旧部署脚本会失效。
- **缓解:** 文档明确“无兼容”，并在 README/知识库给出新的启动方式与变量名；通过 `go test ./...` 与构建验证保障改名不引入回归。


# 技术设计: Realms 品牌改名（移除 Codex 作为产品名）

## 技术方案

### 核心技术
- Go 单体服务（`net/http`）入口目录调整
- 服务端渲染模板（`html/template`）文案替换
- 构建/开发工具链（`air` / Docker）产物命名统一

### 实现要点
- **入口改名：** 将 `cmd/codex` 迁移为 `cmd/realms`，并同步修改：
  - `.air.toml` 的 build target 与 entrypoint
  - `Dockerfile` 的 build 输出与 ENTRYPOINT
- **环境变量前缀：** 将 `internal/config` 中的 `CODEX_*` 覆盖项改为 `REALMS_*`；同时更新错误提示文案。
- **UI 文案：**
  - `internal/web/templates/*` 与 `internal/web/server.go` 中的站点标题/页脚/登录文案替换为 Realms
  - `internal/admin/templates/*` 与 `internal/admin/server.go` 中的后台标题/状态文案替换为 Realms
  - 保留 `Codex OAuth` 等协议术语（包含相关页面标题/按钮文案），避免语义漂移
- **默认开发库名：** 更新 `docker-compose.yml` 与 `config.example.yaml` 中示例库名为 `realms`

## 架构决策 ADR

### ADR-0001: 不提供旧命名兼容
**上下文:** 用户明确要求“旧的全删”，且目标是统一品牌与运维命名。
**决策:** 不保留 `cmd/codex`、`CODEX_*` 环境变量、示例库名 `codex` 的兼容入口。
**理由:** 降低长期维护成本与认知负担，避免双命名并存导致的不一致与文档债。
**替代方案:** 保留旧别名并做 deprecate → 拒绝原因: 与“旧的全删”约束冲突。
**影响:** 这是破坏性变更；升级需要同步更新启动命令、环境变量、示例配置与数据库名。

## 安全与性能
- **安全:** 不触碰凭据加密逻辑与权限模型，仅替换环境变量名与提示信息；避免在日志/文档中写入真实密钥。
- **性能:** 仅文案与启动入口变更，对性能无影响。

## 测试与部署
- **测试:** `go test ./...`
- **构建验证:** `go build ./...`；Docker 构建能产出 `realms` 入口镜像
- **部署变更提示:** 升级时需要将运行命令改为 `./realms` 并更新 `REALMS_*` 环境变量（旧变量不再读取）


# 任务清单: 配置统一改为 `.env`（移除 YAML 配置接口）

目录: `helloagents/plan/202601212159_env_only_config/`

---

## 1. 配置加载（env-only）
- [√] 1.1 在启动入口自动加载 `.env`（不覆盖已存在 env），验证 why.md#需求-本地开发只维护一份配置-场景-宿主机运行非容器
- [√] 1.2 重构 `internal/config/config.go`：移除 YAML 读取入口，提供 `Load()`，并补齐对原 `config.example.yaml` 全量配置项的 env 映射，验证 why.md#需求-本地开发只维护一份配置-场景-docker-compose-长期运行

## 2. 启动入口与依赖清理
- [√] 2.1 在 `cmd/realms/main.go` 移除 `-config` 参数与 `configPath` 透传，改为调用 `config.Load()`
- [√] 2.2 在 `internal/server/app.go` 与 `internal/admin/server.go` 移除 `ConfigPath` 相关字段与参数
- [√] 2.3 清理依赖：移除 `gopkg.in/yaml.v3`（如不再需要），并 `go mod tidy`

## 3. 示例与脚本/文档同步
- [√] 3.1 更新 `.env.example`：覆盖原 `config.example.yaml` 的全部配置项，并按场景给出默认值/注释
- [√] 3.2 更新 `scripts/dev.sh`：改为生成 `.env`（不再生成 `config.yaml`）
- [√] 3.3 更新文档：`README.md`、`CONTRIBUTING.md`、`helloagents/project.md`，统一以 `.env` 为准；删除 `config.example.yaml` 并移除引用
- [√] 3.4 更新管理后台提示：`internal/admin/templates/settings.html` 中“只能在 config.yaml 修改”的文案改为 “只能通过环境变量/.env 修改”

## 4. 安全检查
- [√] 4.1 执行安全检查（按G9: 禁止日志打印敏感 DSN/密钥；`.env` 加载不覆盖显式注入 env；避免生产误用）

## 5. 测试
- [√] 5.1 为 `.env` 加载与 `config.Load()` 添加单元测试（覆盖关键解析行为与校验）
- [√] 5.2 运行 `go test ./...`，确保无回归

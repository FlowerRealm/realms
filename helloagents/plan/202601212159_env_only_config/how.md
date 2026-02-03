# 技术设计: 配置统一改为 `.env`（移除 YAML 配置接口）

## 技术方案

### 核心技术
- Go（`net/http`）
- 环境变量解析（`os.Getenv` + 类型解析）
- 轻量 `.env` 解析器（进程启动时 best-effort 加载）

### 实现要点

1. **新增 `.env` 自动加载（best-effort）**
   - 启动时读取当前工作目录的 `.env`
   - 解析 `KEY=VALUE` 形式；忽略空行与注释行
   - 默认 **不覆盖** 已存在的环境变量（生产环境一般通过 env 注入，不应被 `.env` 覆盖）
   - 解析失败时返回明确错误，但不打印敏感值

2. **配置加载器改为 env-only**
   - `internal/config` 保留 `Config` 结构与默认值（作为“缺省值”）
   - 移除 `Load(path)` / YAML 读取逻辑，提供 `Load()`：
     - 先加载 `.env`（若存在）
     - 再应用环境变量覆盖（env overrides）
     - 进行与现有一致的校验（addr、dsn、URL、时区、目录/上限等）

3. **补齐 env 覆盖映射（完整覆盖原 YAML 配置项）**
   - 当前缺口主要在：
     - `server.read_*` 等超时
     - `limits.*`
     - `debug.proxy_log.*`
     - `app_settings_defaults.*`
   - 为上述字段补齐 `REALMS_...` 环境变量，并在 `.env.example` 给出示例值

4. **移除 `-config` 启动参数与相关传递**
   - `cmd/realms/main.go` 去掉 `flag` 与 `configPath`
   - `internal/server` / `internal/admin` 移除 `ConfigPath` 透传（目前仅存字段未实际使用）

5. **仓库层面只保留 `.env.example`**
   - 删除 `config.example.yaml`
   - `scripts/dev.sh` 改为：
     - 若无 `.env` 则从 `.env.example` 生成
     - `source .env` 仍保留（便于 `air`/子进程继承），但即使不 source，主进程也会自动加载 `.env`

## 架构决策 ADR

### ADR-0001: 配置来源统一为环境变量（支持 `.env` 自动加载）
**上下文:** YAML 配置 + env 覆盖导致配置分裂，实际运行时容易“看起来改了但没生效”，并引发 DSN 主机名不一致等错误。  
**决策:** 移除 YAML 配置文件接口；启动时 best-effort 加载 `.env`；最终配置来源为环境变量。  
**理由:** 只有一个配置入口，最少认知负担；与 Docker/CI/生产环境注入方式一致；避免双写同步。  
**替代方案:** 保留 YAML 并要求与 `.env` 双向同步 → 拒绝原因: 维护成本高且仍会出现覆盖顺序误用。  
**影响:** 启动参数 `-config` 与 `config.example.yaml` 被移除（破坏性变更）；需要提供清晰迁移说明。

## 测试与部署

- **测试:**
  - 为 `.env` 解析器添加单元测试（覆盖注释/空行/引号/不覆盖现有 env 等关键行为）
  - 为 `config.Load()` 添加单元测试（确保 env-only 下能通过现有校验）
  - 运行 `go test ./...`
- **部署/迁移:**
  1. `cp .env.example .env`
  2. 按运行方式修改 `REALMS_DB_DSN`：
     - 宿主机运行：`tcp(127.0.0.1:3306)`
     - compose 容器内：`tcp(mysql:3306)`
  3. 启动方式统一：`go run ./cmd/realms` 或 `./realms`


# 变更提案: 配置统一改为 `.env`（移除 `config.yaml`/YAML 配置接口）

## 需求背景

当前服务配置存在两套入口：
- `config.yaml`（YAML 文件）
- 环境变量（含 `.env` 文件被脚本 `source` 后的覆盖）

这会带来典型的“配置源不一致”问题：例如在宿主机运行时，`.env` 里使用了容器网络的 DSN（`tcp(mysql:3306)`），覆盖了 `config.yaml` 的本机 DSN，最终导致 `lookup mysql: no such host` 一类的启动失败。配置分裂也增加了文档与排障成本。

本变更目标是把配置来源收敛为 **唯一入口：环境变量（并在进程启动时自动加载当前目录的 `.env` 文件）**，彻底移除 YAML 配置文件接口，避免“同步两份配置”这类无意义的维护成本。

## 变更内容

1. **移除 YAML 配置入口**
   - 删除 `-config` 启动参数与 `config.yaml`/`config.example.yaml` 配置模式
2. **进程启动时自动加载 `.env`**
   - 若当前目录存在 `.env`，启动时读取并写入环境变量（默认不覆盖已存在的环境变量）
3. **完整迁移 YAML 配置项到 `.env`**
   - 将原 `config.example.yaml` 中的所有配置项提供对应的环境变量映射
   - 更新 `.env.example`，使其成为唯一的配置示例与“可拷贝模板”
4. **同步更新文档/脚本/页面提示**
   - 更新 README/贡献指南/管理后台提示等，统一以 `.env` 为准

## 影响范围

- **模块:**
  - 配置加载：`internal/config`
  - 启动入口：`cmd/realms/main.go`
  - 管理后台提示：`internal/admin/templates`
  - 开发脚本：`scripts/dev.sh`
  - 文档：`README.md`、`CONTRIBUTING.md`、`helloagents/project.md`
- **兼容性:**
  - 这是一次 **破坏性变更**：移除 `-config` 与 YAML 配置文件接口

## 核心场景

### 需求: 本地开发只维护一份配置
**模块:** 配置加载 / 开发脚本

#### 场景: 宿主机运行（非容器）
开发者只需维护 `.env`：
- 期望结果：`REALMS_DB_DSN` 默认指向 `127.0.0.1:3306`，不再出现 `mysql` 解析失败
- 期望结果：无需生成/维护 `config.yaml`

#### 场景: Docker Compose 长期运行
`.env` 作为 compose 的变量来源：
- 期望结果：容器内通过 `REALMS_DB_DSN=tcp(mysql:3306)` 访问 compose 网络内的 MySQL
- 期望结果：服务启动不依赖容器内存在 `.env` 文件

## 风险评估

- **风险:** 现有用户依赖 `-config config.yaml` 启动参数与 YAML 配置文件  
  **缓解:** 文档给出迁移步骤；错误信息明确提示“改用 `.env`/环境变量”
- **风险:** `.env` 可能包含敏感信息  
  **缓解:** 继续保持 `.env` 在 `.gitignore` 中忽略；禁止日志打印敏感值
- **风险:** `.env` 覆盖真实环境变量可能导致生产事故  
  **缓解:** `.env` 仅在存在时加载，且默认不覆盖已存在的环境变量（以实际注入的 env 为准）


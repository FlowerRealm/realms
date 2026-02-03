# 变更提案: compose_pull_flowerrealm_realms

## 元信息
```yaml
类型: 优化
方案类型: implementation
优先级: P1
状态: ✅已完成
创建: 2026-01-28
```

---

## 1. 需求

### 背景
当前 `docker-compose.yml` 默认通过 `Dockerfile` 本地构建 `realms` 镜像，部署/升级时需要本地构建环境且耗时；同时不利于以“发布物（镜像）”为准固定版本与回滚。

### 目标
- `docker compose up` 默认从 Docker Hub 拉取并运行 `flowerrealm/realms`
- 支持通过环境变量覆盖镜像（私有仓库/固定 tag/本地构建镜像）
- 同步更新部署文档与更新脚本，确保与运行时行为一致

### 约束条件
```yaml
兼容性约束: 保持 mysql + realms 编排与 env 配置方式不变
```

### 验收标准
- [x] `docker compose config` 校验通过（`docker-compose.yml` + 默认 override）
- [x] 文档不再要求 `--build`，并说明升级/回滚（镜像 tag）与 `REALMS_IMAGE`
- [x] `scripts/update-realms.sh` 改为 pull + up（不再 build）

---

## 2. 方案

### 技术方案
1) 将 `docker-compose.yml` 中 `realms` 服务从 `build:` 改为 `image:`，默认 `flowerrealm/realms`，并引入 `REALMS_IMAGE` 覆盖能力  
2) 更新部署文档（`docs/USAGE.md`、`.env.example`、KB 模块文档）以匹配“拉取镜像”流程  
3) 更新 `scripts/update-realms.sh`：更新代码后执行 `docker compose pull realms` + `docker compose up -d`

### 影响范围
```yaml
预计变更文件:
  - docker-compose.yml
  - docs/USAGE.md
  - .env.example
  - scripts/update-realms.sh
  - helloagents/wiki/modules/realms.md
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| Docker Hub 镜像不可用或 tag 与预期不一致 | 中 | 提供 `REALMS_IMAGE` 覆盖并在文档中明确固定 tag 的回滚方式 |

---

## 3. 核心场景

### 场景: Compose 默认拉取并运行
**条件**: 具备 docker compose 运行环境  
**行为**: `docker compose pull realms && docker compose up -d`  
**结果**: `realms` 以 `flowerrealm/realms`（或 `REALMS_IMAGE` 指定的镜像）运行

---

## 4. 技术决策

### compose_pull_flowerrealm_realms#D001: docker-compose 使用 REALMS_IMAGE 覆盖镜像
**日期**: 2026-01-28  
**状态**: ✅采纳  
**背景**: 需要在默认使用 `flowerrealm/realms` 的同时，允许固定 tag 或切换私有/本地镜像用于回滚与调试。  
**选项分析**:
| 选项 | 优点 | 缺点 |
|------|------|------|
| A: `image: flowerrealm/realms`（硬编码） | 简单直观 | 固定版本/私有镜像需要改文件 |
| B: `image: ${REALMS_IMAGE:-flowerrealm/realms}` | 默认简单，且支持 `.env` 覆盖 | 增加一个可选变量 |
**决策**: 选择方案 B  
**理由**: 不改变默认行为的同时，为升级/回滚与自建镜像提供最小成本的切换手段。  
**影响**: 影响 Docker Compose 部署与文档/更新脚本说明。

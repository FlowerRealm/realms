# 项目上下文

## 1. 基本信息

```yaml
名称: Realms
描述: 单体 OpenAI 风格 API 中转服务（数据面代理 + 管理面控制台）
类型: 服务
状态: 开发中
```

## 2. 技术上下文

```yaml
语言: Go
框架: Gin（HTTP 路由）
包管理器: go mod
构建工具: go / make
```

### 主要依赖
| 依赖 | 版本 | 用途 |
|------|------|------|
| github.com/gin-gonic/gin | - | HTTP 服务与路由 |
| github.com/gin-contrib/sessions | - | Web 会话 |
| github.com/gin-contrib/gzip | - | 管理面 API 压缩 |
| github.com/shopspring/decimal | - | 计费金额/倍率精度 |
| modernc.org/sqlite | - | SQLite 驱动 |
| github.com/go-sql-driver/mysql | - | MySQL 驱动 |

## 3. 项目概述

### 核心功能
- 提供 OpenAI 兼容北向接口（`/v1/responses`、`/v1/chat/completions`、`/v1/messages`、`/v1/models` 等）
- 调度上游（Channel → Endpoint → Credential），支持 failover、冷却、亲和与会话绑定
- 透传/改写上游请求与响应（含 SSE 流式）
- 提供管理面控制台（用户/Token/渠道/渠道组/模型/用量/计费等）

### 项目边界
```yaml
范围内:
  - 北向 OpenAI 兼容代理与调度
  - 上游鉴权注入、错误映射、SSE 透传
  - 基础计费/配额与用量事件记录
范围外:
  - 自研大模型推理/训练
  - 作为通用反向代理（非 OpenAI 兼容 API 不保证）
```

## 4. 开发约定

### 代码规范
```yaml
命名风格: Go 标准风格（导出/非导出）
文件命名: snake_case.go（项目现状以仓库为准）
目录组织: cmd/（入口）, internal/（业务）, router/（HTTP 路由）, web/（前端）
```

### 错误处理
```yaml
错误码格式: HTTP 状态码 + 文本错误消息（部分路径返回 JSON error）
日志级别: slog.Info 为主（见 internal/middleware/logging.go）
```

### 测试要求
```yaml
测试框架: go test
覆盖率要求: 无硬性指标（以关键路径回归为主）
测试文件位置: 贴近模块（internal/**, router/**, tests/**）
```

### Git规范
```yaml
分支策略: 以仓库实际策略为准
提交格式: 中英双语（仓库历史示例：feat(x): 中文 / English）
```

## 5. 当前约束（源自历史决策）

> 这些是当前生效的技术约束，详细决策过程见对应方案包

| 约束 | 原因 | 决策来源 |
|------|------|---------|
| SSE/长连接写回不使用 server.WriteTimeout | 避免误伤流式响应 | 待补充（后续方案包归档后补链） |
| base_url 需通过 `security.ValidateBaseURL` | 降低明显 SSRF/配置错误风险 | 待补充（后续方案包归档后补链） |

## 6. 已知技术债务（可选）

| 债务描述 | 优先级 | 来源 | 建议处理时机 |
|---------|--------|------|-------------|
| `.env` 历史字段与 `internal/config` 的 `_SECONDS` 字段命名不一致（可能导致误以为配置生效） | P2 | 2026-02-16 识别 | 结合一次配置清理/文档更新处理 |


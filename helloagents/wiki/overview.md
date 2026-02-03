# Realms（OpenAI 风格 API 中转）知识库概览

> 本文件包含项目级别的核心信息。详细的模块文档见 `modules/` 目录。

---

## 1. 项目概述

### 目标与背景

本项目目标是实现一个 **OpenAI 风格 API 中转（proxy/relay，含 Codex/OpenAI 上游接入）** 服务，并在此过程中沉淀可复用的鉴权、配额/用量口径与安全实践文档。

### 范围

- **范围内:**
  - Codex/相关 OpenAI 接口的鉴权方案调研与实现笔记
  - 中转服务的请求/响应兼容（OpenAI 格式等）
  - 用量/花费/配额口径梳理（以官方文档为准）
- **范围外:**
  - 绕过平台条款或安全机制的实现
  - 非授权的抓取/爬取/注入等高风险行为

### 干系人

- **负责人:** flowerrealm

---

## 2. 模块索引

| 模块名称 | 职责 | 状态 | 文档 |
|---------|------|------|------|
| realms | OpenAI 风格中转服务（数据面代理 + 管理面配置 + SSE 中转） | 🚧可用（MVP） | [modules/realms.md](modules/realms.md) |
| research | 外部实现调研与实现笔记沉淀（参考资料、对比与结论） | ✅稳定 | [modules/research.md](modules/research.md) |

---

## 3. 快速链接

- GitHub：<https://github.com/FlowerRealm/realms>
- [技术约定](../project.md)
- [架构设计](arch.md)
- [API 手册](api.md)
- [数据模型](data.md)
- [贡献指南](../../CONTRIBUTING.md)
- [安全政策](../../SECURITY.md)
- [行为准则](../../CODE_OF_CONDUCT.md)
- [许可证](../../LICENSE)
- [实现入口：cmd/realms/main.go](../../cmd/realms/main.go)
- [配置示例：config.example.yaml](../../config.example.yaml)
- [已执行方案包：MVP（统一中转服务）](../history/2026-01/202601131914_codex/)
- [调研：new-api 端口通信](research/new-api_api_port_communication.md)
- [调研：Codex CLI 协议形态（wire API）](research/codex_cli_wire_protocol.md)
- [调研：claude-proxy 路由与 failover 机制](research/claude-proxy-routing.md)
- [变更历史](../history/index.md)

---

## 4. 开发环境测试账号

仅用于开发环境快速验证登录流程：

- Email：`test@test.com`
- 密码：`testtest`

# 变更提案: Web 对话功能构建期开关（可选编译）

## 需求背景

当前 Realms Web 控制台包含用户侧对话页面（`/chat`）以及配套接口（`/api/chat/*`）。这部分功能在某些部署场景并非必须（例如仅作为数据面中转、或希望尽量缩小二进制体积/减少前端依赖暴露面）。

本变更希望将“Web 对话界面”与主服务实现 **半分离**：在 **编译期** 即可选择是否将其编译进最终产物；默认行为保持不变（仍包含 Web 对话），仅在显式指定构建选项时完全剔除。

## 变更内容

1. 引入 Go build tag：`no_webchat`
   - 默认（未指定 tag）: Web 对话保持可用，与当前行为一致。
   - 指定 `-tags no_webchat`: 不编译 Web 对话界面与 `/api/chat/*` 辅助接口。
2. Web 控制台侧边栏入口随构建产物自动隐藏（避免出现入口但路由不存在的情况）。
3. `/v1/*` 数据面接口保持不变；对话相关的请求语义（如 `X-Realms-Chat`）不在本变更中调整。

## 影响范围

- **模块:**
  - `internal/web/*`（模板嵌入与对话页 handler 的构建裁剪）
  - `internal/server/*`（路由注册的构建裁剪）
  - `internal/store/*`（FeatureState/FeatureGate 对构建期强制禁用的统一表达）
  - `Dockerfile` / `README.md`（构建方式说明与可选构建参数）
- **路由:**
  - 受影响：`GET /chat`、`POST /api/chat/token`、`GET /api/chat/models`、`POST /api/chat/search`
  - 不受影响：`/v1/responses`、`/v1/chat/completions`、`/v1/models` 以及其他 Web/管理后台路由

## 核心场景

### 需求: 可在编译期剔除 Web 对话界面
**模块:** build/web/server

#### 场景: 默认构建（保持不变）
未指定 build tag 构建并运行服务。
- `/chat` 路由存在（未登录时重定向 `/login`）
- Web 控制台侧边栏显示“对话”入口

#### 场景: 禁用构建（完全不编译）
使用 `-tags no_webchat` 构建并运行服务。
- `/chat` 与 `/api/chat/*` 路由不注册（直接 404）
- Web 控制台侧边栏不显示“对话”入口

## 风险评估

- **风险:** 通过 build tag 拆分文件后，易出现“某处仍引用 chat handler/模板”导致编译失败  
  **缓解:** 将路由注册与模板嵌入分别做 build-tag 隔离，并增加带 tag 的路由测试覆盖。
- **风险:** 运行期功能禁用（Feature Bans）与构建期禁用叠加后语义不清晰  
  **缓解:** 构建期禁用作为“强制禁用”叠加到最终 FeatureState/FeatureGate 计算中，确保 UI 与路由 gate 一致。


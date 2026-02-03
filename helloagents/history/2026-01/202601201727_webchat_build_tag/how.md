# 技术设计: Web 对话功能构建期开关（no_webchat）

## 技术方案

### 核心技术
- Go build tags：`//go:build no_webchat` / `//go:build !no_webchat`
- 以“最小改动”为原则，默认构建行为保持不变

### 实现要点

1. **路由注册隔离（server）**
   - 在 `internal/server` 增加一个 `registerWebChatRoutes(...)` 的钩子函数：
     - `!no_webchat` 版本：注册 `/chat` 与 `/api/chat/*` 路由
     - `no_webchat` 版本：空实现（不注册任何路由）
   - `internal/server/app.go` 中统一调用该钩子，避免主路由文件写大量 `//go:build` 条件分支。

2. **handler 文件隔离（web）**
   - 给 `internal/web/chat.go` 添加 `//go:build !no_webchat`，使其在禁用构建时完全不参与编译。
   - 禁用构建时不需要提供 stub handler（因为路由不会注册）。

3. **模板嵌入隔离（web/templates embed）**
   - 将 `templatesFS` 的 `//go:embed` 从 `internal/web/server.go` 拆到 build-tag 文件中：
     - `!no_webchat`: 继续 `//go:embed templates/*.html`
     - `no_webchat`: 显式列出除 `templates/chat.html` 外的所有模板文件，确保禁用构建时不把 chat 模板打进二进制。
   - `template.ParseFS(templatesFS, "templates/*.html")` 保持不变。

4. **FeatureState / FeatureGate 一致性（store）**
   - 在 `internal/store` 增加 build-tag 常量（例如 `webChatBuilt`）：
     - `!no_webchat`: `true`
     - `no_webchat`: `false`
   - 在 `FeatureStateEffective` 中：当 `!webChatBuilt` 时强制 `WebChatDisabled=true`，用于隐藏侧边栏入口。
   - 在 `FeatureDisabledEffective` 中：当 `!webChatBuilt` 且 key 为 `feature_disable_web_chat` 时直接返回 `true`，确保运行期 gate 与 UI 一致。

5. **构建与交付**
   - `README.md` 增加构建说明：`go build -tags no_webchat ...`
   - `Dockerfile` 增加可选 `ARG REALMS_BUILD_TAGS`，允许在镜像构建时传入 `no_webchat`（默认空=保持现状）。

## 架构决策 ADR

### ADR-001: 采用 build tag `no_webchat` 作为“默认包含、可选剔除”的构建开关
**上下文:** 需要在编译期选择是否包含 Web 对话界面，同时保持现有默认构建不变。  
**决策:** 使用 `no_webchat`（默认不传即包含）。  
**理由:** 最小破坏、最符合“默认不变”的兼容性要求；实现简单可维护。  
**替代方案:** 采用 `webchat`（默认不包含，显式开启） → 拒绝原因: 会改变现有默认构建产物与行为，风险更高。  
**影响:** 需要在 server/web/store 三处做 build-tag 分层，但改动面可控。

## API 设计
无新增 API，仅对现有路由做“构建期是否注册”的裁剪。

## 数据模型
无变更。

## 安全与性能
- **安全:** 禁用构建时不嵌入 chat 前端资源与相关入口，减少暴露面；数据面 `/v1/*` 行为不变。
- **性能:** 禁用构建时减小二进制体积（不包含 chat.html 与其内联脚本/样式）。

## 测试与部署
- **测试:**
  - 默认构建：新增路由测试，验证 `GET /chat` 存在（未登录返回 302 到 `/login`）
  - `no_webchat` 构建：新增带 tag 的路由测试，验证 `GET /chat` 为 404
- **部署:**
  - 需要禁用 Web 对话时：`go build -tags no_webchat ...` 或 Docker 构建时传入 `REALMS_BUILD_TAGS=no_webchat`


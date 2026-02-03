# 技术设计: 合并 Endpoint 页面与 Key 管理页面

## 技术方案

### 核心技术
- Go 1.22 `net/http`
- `html/template`（`internal/admin/templates`）
- Bootstrap（现有样式/Modal/表格）
- 现有 Store 能力：
  - `ListOpenAICompatibleCredentialsByEndpoint`
  - `CreateOpenAICompatibleCredential`
  - `DeleteOpenAICompatibleCredential`

### 实现要点
- 在 `internal/admin/server.go` 的 `Endpoints` handler 中：
  - 加载 `channel` + 主 `endpoint`
  - 当 `Channel.Type == openai_compatible` 时，加载 credentials 并填充 `templateData.Credentials`
- 更新 `internal/admin/templates/endpoints.html`：
  - `openai_compatible`: 增加 Keys 区块（列表 + 添加 modal + 删除按钮）
  - `codex_oauth`: 保持“管理授权账号”入口不变
- 兼容策略：
  - 删除 `GET /admin/endpoints/{endpoint_id}/openai-credentials` 页面（不做兼容）
  - `POST /admin/endpoints/{endpoint_id}/openai-credentials` 与 `POST /admin/openai-credentials/{credential_id}/delete` 保持不变；其成功跳转目标统一指向合并页
- 导航修正：
  - `internal/admin/templates/channels.html` 中 “OpenAI Credentials/Keys” 的入口指向合并页（减少一次跳转）

## 架构决策 ADR

### ADR-001: 直接删除旧 openai-credentials 页面
**上下文:** 单 Endpoint 模式下，Endpoint 与 Keys 强绑定；分离页面导致操作跳转频繁且维护两份模板。  
**决策:** 删除 GET 页面与入口；Keys 仅在合并页内维护一份渲染。  
**理由:** 最小维护成本（删除一套模板与 handler），并与“单页完成配置”的目标一致。  
**替代方案:** 保留旧页面做重定向 → 拒绝原因: 仍需保留额外路由与心智负担，且当前需求明确不考虑兼容。  
**影响:** 旧 URL 书签将失效（404）；需要用户改用合并页入口。

## API设计
- 保持现有 POST 行为不变（新增/删除 Key）
- GET `openai-credentials` 仅用于兼容跳转

## 数据模型
- 无变更：继续使用 `openai_compatible_credentials` 表与加密字段 `api_key_enc`

## 安全与性能
- **安全:**
  - 不引入“查看明文 Key”能力
  - 继续要求 CSRF Token
- **性能:**
  - 仅多一次 credentials 查询（按 `endpoint_id`）；Key 数量通常很小

## 测试与部署
- **测试:**
  - `go test ./...`
  - 手动验证：在 `/admin/channels/{id}/endpoints` 添加/删除 Key，确认旧 URL 重定向生效
- **部署:**
  - 无迁移；按常规发布

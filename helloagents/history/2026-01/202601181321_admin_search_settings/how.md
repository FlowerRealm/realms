# 技术设计: 管理后台可配置 Chat 联网搜索

## 技术方案

### 核心技术
- 复用 `app_settings` 作为运行期配置持久化存储
- 管理后台 `/admin/settings` 表单保存配置
- Web Chat 在请求时按“默认配置 + app_settings 覆盖”计算生效配置

### 实现要点

1. **新增 app_settings keys（常量）**
   - `search_searxng_enable`
   - `search_searxng_base_url`
   - `search_searxng_timeout`
   - `search_searxng_max_results`
   - `search_searxng_user_agent`

2. **管理后台系统设置 UI**
   - 在 `internal/admin/templates/settings.html` 增加“对话/搜索”Tab 与 SearXNG 配置卡片
   - 显示“界面覆盖 / 配置文件默认”状态与基础的合法性提示

3. **保存逻辑（/admin/settings）**
   - `enable=true` 时要求 `base_url` 非空且合法
   - `timeout` 使用 `time.ParseDuration` 校验（>0）
   - `max_results` 限制范围 `1..20`
   - 值等于配置文件默认时删除对应 app_setting key（保持“仅覆盖差异”的语义）
   - `reset` 动作时删除上述 keys

4. **Web Chat 生效逻辑**
   - `/chat` 页面的 `ChatSearchEnabled/ChatSearchMaxResults` 使用生效配置渲染（即：界面配置后立刻生效）
   - `POST /api/chat/search` 每次请求根据生效配置创建/使用 SearXNG client（避免启动时固定配置）

## 安全与性能
- **SSRF:** 搜索仅允许访问管理员配置的固定 `base_url`（服务端校验并规范化）；不允许客户端指定上游 URL。
- **XSS:** 不改变现有 Chat 的 Markdown/XSS 防护策略。
- **性能:** 搜索请求增加超时与结果数上限；每次请求创建 client 仅是轻量对象，不做复杂缓存（YAGNI）。

## 测试与部署
- **测试:** `go test ./...`
- **部署:** 可继续通过 `config.yaml` 配置默认值；管理后台保存的值优先，且无需重启即可生效。


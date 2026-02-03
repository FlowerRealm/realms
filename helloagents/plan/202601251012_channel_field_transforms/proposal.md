# 变更提案: channel-field-transforms

## 元信息
```yaml
类型: 功能增强
方案类型: implementation
优先级: P1
状态: 已完成
创建: 2026-01-25
完成: 2026-01-25
```

---

## 1. 背景/问题

Realms 已实现并对齐 new-api 的渠道级能力：
- 请求字段策略：`allow_service_tier/disable_store/allow_safety_identifier`
- 参数改写：`param_override`（new-api `operations` 兼容）
- 请求头覆盖：`header_override`（支持 `{api_key}` 替换）
- 状态码映射：`status_code_mapping`（仅改写对外 HTTP status code）

仍存在缺口：
- 部分上游（尤其 openai_compatible 的自定义 base_url / 各类代理）对请求体字段支持不一致，需要像 new-api 一样做**字段转换**与更通用的**字段黑白名单**处理，减少“渠道可用但被误判不可用/频繁 400”的情况。

---

## 2. 目标与范围

### 2.1 生效范围（北向）
- 覆盖现有数据面：
  - `POST /v1/responses`
  - `POST /v1/messages`
- 暂不恢复/新增：`POST /v1/chat/completions`

### 2.2 配置位置
- 以 **渠道维度**（`/admin/channels`）配置与生效

### 2.3 网络限制
- 允许配置为内网 `base_url`（不引入默认禁止内网的 SSRF 行为）

---

## 3. 参考 new-api 的对齐点（A+B）

### 3.1 字段转换（A）
- **A1/A2（Responses 侧）**：对齐 new-api OpenAI adaptor 的 `ConvertOpenAIResponsesRequest`：
  - 支持从 `model` 后缀解析推理力度（`-low/-medium/-high/-minimal/-none/-xhigh`）并写入 `reasoning.effort`，同时去掉后缀
- **Tokens 字段名兼容（补齐常见代理差异）**：
  - `/v1/responses`：将 `max_tokens/max_completion_tokens` 规范化为 `max_output_tokens`
  - `/v1/messages`：将 `max_output_tokens/max_completion_tokens` 规范化为 `max_tokens`

### 3.2 黑白名单（B）
- **模型后缀保护名单（new-api thinking_model_blacklist 同语义）**：
  - 某些模型名的 `-thinking`（以及可能的 `-low/-high` 等）是模型名的一部分，不应被当成“推理后缀”解析；需要可配置“保护名单”来跳过自动后缀解析。
- **通用请求体字段黑/白名单（按渠道）**：
  - 支持按 JSON path 删除（blacklist）或仅保留（whitelist）请求字段，作为 `service_tier/store/safety_identifier` 之外的通用机制。
  - 执行顺序保持与前序对齐：模型 alias rewrite →（字段转换/过滤）→ 请求字段策略 → 通用黑白名单 → param_override

---

## 4. 实施概要

### 4.1 数据结构
- 为 `upstream_channels` 增加以下配置字段（TEXT/JSON）：
  - `model_suffix_preserve`（或等价命名）：JSON 数组，匹配到的模型不做后缀解析
  - `request_body_blacklist`：JSON 数组（JSON path 列表）
  - `request_body_whitelist`：JSON 数组（JSON path 列表）

### 4.2 数据面实现
- `/v1/responses` 与 `/v1/messages` 在每次 selection 转发前：
  - 先应用字段转换（model 后缀 → reasoning.effort；tokens 字段规范化）
  - 再应用请求字段策略与黑白名单
  - 最后应用 `param_override`（允许管理员改写覆盖前序过滤）

### 4.3 管理后台
- `/admin/channels` 增加弹窗编辑：
  - 模型后缀保护名单
  - 请求体字段黑名单
  - 请求体字段白名单

### 4.4 导出/导入
- Admin Config 导出/导入版本递增并兼容旧版本（仅增加字段，不破坏旧文件导入）

---

## 5. 验收标准

- [ ] `/v1/responses`：`model=gpt-5-mini-high` 等能正确写入 `reasoning.effort=high` 并去掉后缀（未命中保护名单时）
- [ ] `/v1/messages`：当客户端误传 `max_output_tokens` 时能自动规范化为 `max_tokens`
- [ ] 黑白名单按渠道生效，且 failover 时重新按“当前渠道”应用（无跨渠道串扰）
- [ ] `go test ./...` 通过

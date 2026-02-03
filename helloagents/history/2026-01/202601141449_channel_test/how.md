# 技术设计: Channel 测试延迟与可用性指标

## 技术方案

### 核心技术
- Go `net/http` + 现有 `internal/upstream.Executor`（复用超时、鉴权注入、SSRF 防护与禁重定向）
- MySQL 迁移：`internal/store/migrations`
- SSR 管理后台：`internal/admin`（HTML 模板 + 表单 POST）

### 实现要点
1. **数据结构扩展（持久化最近一次测试）**
   - 在 `upstream_channels` 增加字段：
     - `last_test_at DATETIME NULL`
     - `last_test_latency_ms INT NOT NULL DEFAULT 0`
     - `last_test_ok TINYINT NOT NULL DEFAULT 0`
   - `last_test_at IS NULL` 表示“从未测试”；否则以 `last_test_ok/last_test_latency_ms` 展示结果。

2. **管理后台测试入口（root-only）**
   - 新增路由：`POST /admin/channels/{channel_id}/test`
   - 行为：对指定 channel 发起一次上游探测请求，完成后回写测试字段，并重定向回 Channels 页面。

3. **选择测试目标（Endpoint + Credential/Account）**
   - 测试粒度是 **channel**，但实际发请求需要落到一个可用的 endpoint 与 credential/account。
   - 选择策略（与现有调度尽量一致，但不引入绑定/亲和副作用）：
     - endpoints：按 `priority DESC, id DESC` 过滤 `status=1`
     - openai_compatible：选择第一个 `status=1` 的 credential
     - codex_oauth：选择第一个 `status=1` 且不在 `cooldown_until` 的 account
   - 若找不到可用 credential/account，则测试直接失败（返回可理解提示，不更新调度状态）。

4. **测试请求（固定走 responses）**
   - 请求：`POST /v1/responses`
   - Body（最小化，避免大成本）：
     - `model`：按 channel 类型给默认值（例如 openai_compatible 用 `gpt-4o-mini`；codex_oauth 用 `codex-mini-latest`）
     - `input`：固定 `"hi"`
     - `max_output_tokens`：固定较小值（例如 `16`）
   - 结果判定：
     - `2xx` 记为 `last_test_ok=1`
     - 非 `2xx` 或网络错误 记为 `last_test_ok=0`
   - 延迟：以“从发起请求到读取完响应（或到错误返回）”的耗时毫秒数为准（更贴近用户体感）。

5. **展示**
   - `admin/templates/channels.html` 增加列：最近测试时间 / 延迟(ms) / OK
   - 对 `last_test_at IS NULL` 显示 “未测试”

## 架构决策 ADR
### ADR-001: 测试结果是否进入调度
**上下文:** 需求明确选择“只记录/展示测试结果”，不希望引入自动禁用或调度跳过逻辑。  
**决策:** 本期仅落库并展示 `last_test_*`，不改变 `scheduler.Select` 行为。  
**替代方案:** 将 `last_test_ok` 作为调度过滤条件 → 拒绝原因: 误伤风险高、需要更多策略（阈值/回退/灰度）。  
**影响:** 实现更简单，可先满足运维可观测性；后续若要“基于健康度调度”再单独立项。

## API 设计
### [POST] /admin/channels/{channel_id}/test
- **描述:** Root 在管理后台触发对指定 channel 的一次测试
- **请求:** 表单 POST（携带 CSRF）
- **响应:** 302 Redirect 回 `/admin/channels`（附带可选提示参数）

## 数据模型
```sql
ALTER TABLE upstream_channels
  ADD COLUMN last_test_at DATETIME NULL,
  ADD COLUMN last_test_latency_ms INT NOT NULL DEFAULT 0,
  ADD COLUMN last_test_ok TINYINT NOT NULL DEFAULT 0;
```

## 安全与性能
- **安全:**
  - 路由仅挂在 admin chain（`RequireRoles(root)` + CSRF），避免普通用户触发外部请求。
  - 复用 `security.ValidateBaseURL` 防止 SSRF（即使 endpoint 已校验，执行层再校验一次更稳）。
- **性能:**
  - 测试为手动触发，不做定时扫描与并发风暴。
  - 复用现有上游超时配置；响应体读取做上限限制避免内存风险。

## 测试与部署
- **测试:**
  - 单元测试覆盖：选择策略（endpoint/credential 选择）、落库字段更新、模板渲染（可选）
  - 回归测试：`go test ./...`
- **部署:**
  - 应用启动时迁移自动执行（已有机制），上线后不会影响现有数据面接口。


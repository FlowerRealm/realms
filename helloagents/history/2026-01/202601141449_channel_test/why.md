# 变更提案: Channel 测试延迟与可用性指标

## 需求背景
当前 Realms 的上游路由以 `upstream_channels` 为粒度进行优先级/推广/失败计分的选择，但**管理后台缺少“按 channel 一键验证上游是否可用、延迟大概多少”**的能力。

实际运维中，常见问题不是“完全不可用”，而是：
1. 某个 channel 的上游凭据/账号已失效（401/403）
2. 某个 endpoint 能连通但延迟显著升高（用户体感差）
3. 新增/变更上游后，需要快速验证配置是否正确

参考 `new-api` 的通道测试思路，本次只做“**测试并记录/展示**”，不把测试结果直接引入调度或自动禁用逻辑（避免误伤与过度自动化）。

## 变更内容
1. 为 `upstream_channels` 增加最近一次测试指标：
   - `last_test_at`
   - `last_test_latency_ms`
   - `last_test_ok`
2. 管理后台 Channels 页面：
   - 展示最近一次测试结果（可用性/延迟/时间）
   - 提供“测试”按钮，触发一次对该 channel 的上游探测（固定走 `responses`）

## 影响范围
- **模块:**
  - `internal/store`（迁移、模型与读写接口）
  - `internal/admin`（SSR 管理后台：按钮/处理器/展示）
  - `internal/server`（新增路由与依赖注入）
  - `internal/upstream`（复用现有 Executor 发起测试请求）
- **文件:** 预计涉及 6-10 个 Go/HTML/SQL 文件
- **API:**
  - 不新增对外数据面 API
  - 新增 1 个管理后台内部动作路由（root-only）
- **数据:**
  - `upstream_channels` 增加 3 个字段（仅记录最近一次测试结果，不做历史留存）

## 核心场景

### 需求: Channel 测试与结果展示
**模块:** admin / store / upstream
对单个 channel 进行一次“可用性+延迟”测试，并把结果回写后展示在 Channels 列表。

#### 场景: Root 在 Channels 页面点击“测试”
前置条件：channel 处于启用状态，且其下至少存在 1 个启用的 endpoint 与可用的 credential/account。
- 预期结果: 服务向该 channel 选出的上游发起一次 `POST /v1/responses` 探测请求
- 预期结果: 更新 `last_test_at/last_test_latency_ms/last_test_ok`
- 预期结果: Channels 页面可见测试结果

#### 场景: channel 缺少可用 credential/account
- 预期结果: 不崩溃，返回可理解的错误提示
- 预期结果: 不影响其他 channel 的正常工作

## 风险评估
- **风险:** 测试请求会产生少量上游消耗（可能计费）
  - **缓解:** 使用最小请求体与极小输出（例如 `max_output_tokens=16`），且仅提供给 Root 手动触发
- **风险:** 测试过程耗时导致管理后台请求阻塞
  - **缓解:** 复用现有上游超时配置；必要时可将测试做成异步（本期不做，保持简单）


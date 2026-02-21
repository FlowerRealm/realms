# scheduler

## 职责

- 实现三层选择：Channel → Endpoint → Credential
- 维护运行态：用户亲和（Affinity）、会话绑定（Binding）、RPM 统计、Token 统计
- 处理失败：凭据冷却（Cooling）、渠道封禁（Ban）与恢复探测（Probe）

## 接口定义（可选）

### 公共API
| 函数/方法 | 参数 | 返回值 | 说明 |
|----------|------|--------|------|
| `New` | `UpstreamStore` | `*Scheduler` | 创建调度器 |
| `(*Scheduler).SelectWithConstraints` | `ctx, userID, routeKeyHash, cons` | `Selection, error` | 根据约束选择上游 |
| `NewGroupRouter` | `store, sched, userID, routeKeyHash, cons` | `*GroupRouter` | 按渠道组顺序路由并返回 Selection |
| `(*Scheduler).Report` | `sel, res` | - | 上报一次请求结果（用于冷却/封禁/权重） |

## 行为规范

### 场景: 会话绑定命中
**条件**: 存在 routeKeyHash 且命中绑定，且绑定未冷却/未封禁  
**行为**: 优先返回绑定的 Selection，并 touch 续期  
**结果**: 提升同会话连续请求的稳定性

### 场景: probe_due 下的并发选择（probe claim 单飞）
**条件**: channel 处于 `probe_due`（通常由封禁到期触发）  
**行为**:
- 调度器仍会优先选择 `probe_due` channel 作为恢复探测路径
- 使用 probe claim（带 TTL）的单飞机制：同一时刻仅允许一个并发请求抢占 probe 并使用该 channel
- probe claim 被占用时，其它并发请求会跳过该 `probe_due` channel（绑定命中路径会清理绑定，避免粘性卡死）
**结果**: 避免并发探测风暴；但在仅剩单一可用 channel 的场景下可能降低并发可用性（更容易返回“无可用上游”）

### 场景: 失败冷却与封禁
**条件**: 上游失败且 `Retriable=true`  
**行为**: 对 credential 进入冷却；必要时对 channel 进入封禁并设置 probe due  
**结果**: 提升后续选择的成功率与切换速度

## 依赖关系

```yaml
依赖:
  - store（上游配置读取、绑定存储、指针存储）
被依赖:
  - openai-api
```

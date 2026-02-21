# 变更提案: rollback-perf-security-since-093

## 元信息
```yaml
类型: 回滚
方案类型: implementation
优先级: P0
状态: ✅完成
创建: 2026-02-16
```

---

## 1. 需求

### 背景
自 `0.9.3` tag 之后，引入了一批以“性能/安全/规模化”为目标的改动；在当前环境下这些改动明显降低了可用性（请求被拒绝、并发退化、链路复杂化等）。目标是在不重写历史的前提下，撤回这些提交，尽快恢复可用性。

### 目标
- 撤回 `0.9.3..HEAD` 范围内与性能/安全相关的核心变更（按提交信息与变更范围确认）
- 保留其它与 UI/CI/测试稳定性相关的提交
- 保持 Git 历史可追溯（使用 `git revert`；不做 `reset --hard` / 强推）
- `go test ./...` 通过

### 回滚范围（自新到旧）
- `59106f2` refactor(upstream): 移除 MaxConnsPerHost 配置 / remove MaxConnsPerHost config
- `d9ff862` fix(scheduler): 移除 probe claim 单飞限制 / remove probe claim single-flight
- `30d65b2` fix(debug): 调试路由代理感知鉴权 / proxy-aware debug guard
- `b46db9b` feat(net): HTTP 连接硬化+请求体上限 / HTTP hardening + body limits
- `75273a1` feat(usage): 小时 rollup 分表+回填 / sharded hourly rollups + backfill
- `7fb13cf` feat(scale): 多实例缓存失效+鉴权缓存 / multi-instance invalidation + auth cache

### 验收标准
- [√] 以上提交已被 revert 且无冲突残留
- [√] 分支 `rollback/0.9.3-usability` 可线性应用（便于 merge/cherry-pick）
- [√] `go test ./...` 通过
- [√] 知识库已同步（CHANGELOG + 模块文档 + 归档索引）

---

## 2. 方案

### 技术方案
- 在新分支 `rollback/0.9.3-usability` 上，按“从新到旧”顺序执行 `git revert --no-edit`，生成一组显式回滚提交，避免历史重写。
- 回滚完成后运行 `go test ./...` 进行快速验证。
- 同步知识库：新增归档方案包、更新 `helloagents/CHANGELOG.md`，并修正受影响的模块文档（scheduler）。

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| 回滚安全硬化/限流可能重新暴露滥用面 | 中 | 后续以“可用性优先”的方式重新引入（更细粒度开关/默认值/更友好的错误） |
| 回滚规模化/缓存/迁移相关代码后，已升级的数据库 schema 可能处于“超集”状态 | 低 | 旧代码通常可忽略额外表；如需完全收敛可再补充清理/迁移策略 |
| probe claim 单飞回归导致 probe_due 场景并发退化 | 中 | 已按需求回滚；如仍需并发可用性，可在后续以更安全的方式优化 |

---

## 3. 核心场景

### 场景: probe_due 下的并发请求
**模块**: scheduler  
**行为**: `probe_due` channel 使用 probe claim TTL 单飞；并发请求可能跳过该 channel  
**结果**: 避免探测风暴，但在仅单一可用 channel 时可能影响并发可用性

---

## 4. 技术决策

### rollback-perf-security-since-093#D001: 使用 `git revert` 撤回变更而不重写历史
**日期**: 2026-02-16  
**状态**: ✅采纳  
**背景**: 需求是“撤回若干提交以恢复可用性”，同时希望保持历史可追溯、便于审阅与安全合并。  
**决策**: 在新分支上逐个 revert 目标提交（从新到旧），生成一组显式的回滚提交。  
**影响**: 可直接 merge 分支或按需 cherry-pick 回滚提交；无需 `reset --hard` 或强推。

# 技术设计: 移除 policy_*，以功能禁用表达数据面语义

## 技术方案

### 核心技术
- Go（`net/http`）
- MySQL（内置 SQL migrations）
- SSR 模板（管理后台/控制台）

### 实现要点

1) 新增全局模型禁用 key
- 新增 `store.SettingFeatureDisableModels = "feature_disable_models"`
- 删除 legacy keys：
  - `feature_disable_web_models`
  - `feature_disable_admin_models`

2) 用 feature 推导数据面语义（替代 policy）
- free mode：由 `feature_disable_billing`（以及 `self_mode` 强制）决定
- model passthrough：由 `feature_disable_models` 决定

3) 删除 policy_* 读取/写入链路
- 移除 `policy_free_mode` / `policy_model_passthrough` 的：
  - 管理后台展示/保存
  - store 计算逻辑
  - 文档与示例配置

4) 路由层统一 gate
- `/models`、`/admin/models*`、`/v1/models` 均绑定 `feature_disable_models`

## 架构决策 ADR

### ADR-001: 以 feature 禁用替代 policy
**上下文:** `feature_disable_*` 与 `policy_*` 并存，用户需要同时理解“UI/路由禁用”和“数据面语义切换”，并且模型域开关被拆分为 web/admin 两份。

**决策:**
- 删除 policy 这一独立维度；用 feature 禁用表达语义切换：
  - `feature_disable_billing=true` ⇒ free mode
  - `feature_disable_models=true` ⇒ model passthrough
- 新增全局 `feature_disable_models`，作为模型域唯一开关。

**理由:**
- 一个地方配置，一个语义来源，减少歧义与重复入口。
- “禁用就是全禁”可直接表达（web/admin/API 一致）。

**替代方案:** 保留 policy_* 并继续解耦 → 拒绝原因: 语义重复、入口分散、维护成本高。

**影响:**
- 需要一次性迁移 `app_settings` 旧 key
- 文档与示例配置需同步更新

## 数据模型

不改表结构，仅迁移 `app_settings` 的 key：

- 写入（如条件满足）：
  - `feature_disable_billing=true`
  - `feature_disable_models=true`
- 删除（迁移完成后清理）：
  - `policy_free_mode`
  - `policy_model_passthrough`
  - `feature_disable_web_models`
  - `feature_disable_admin_models`

迁移 SQL 需满足幂等（重复执行不改变最终结果）。

## 安全与性能

- **安全:** 开关语义更强（禁用模型=放宽模型校验），需确保只由 root 管理员可修改（现有 `/admin/settings` 权限机制不变）。
- **性能:** 读取 `app_settings` 属于每请求一次的轻量查询；保持现有行为即可（不新增额外 DB 热点）。

## 测试与部署

- **测试:**
  - `go test ./...`
  - 覆盖：
    - `feature_disable_billing` 触发 free provider
    - `feature_disable_models` 触发模型穿透（跳过启用/绑定校验）
    - 迁移 SQL 对旧 key 的映射与清理（可用集成测试或最小 DB 断言）
- **部署:**
  - 正常发布即可；启动时自动应用迁移
  - 如需回滚：因 key 已迁移/删除，回滚版本可能无法读取旧 policy 语义（不建议回滚；或在回滚前手工恢复旧 key）

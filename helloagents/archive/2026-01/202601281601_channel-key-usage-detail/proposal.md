# 变更提案: channel-key-usage-detail

## 元信息
```yaml
类型: 优化
方案类型: implementation
优先级: P1
状态: 已完成
创建: 2026-01-28
```

---

## 1. 需求

### 背景
当前管理后台的“上游渠道”页面（`/admin/channels`）支持配置渠道与 Key/账号，但缺少“按 Key/账号维度”的近端用量视图，不便于排查某个 Key 是否异常、近期是否有使用，以及在 Codex OAuth 场景下查看账号余额。

### 目标
- 为每个渠道提供可点击的“详情”入口
- 详情页支持时间窗口切换：`5m / 1h / 24h`
- 详情页按 Key/账号维度展示：
  - 请求数
  - 成功/失败
  - 输入/输出 Token
  - 最近使用时间
  - 余额（Codex OAuth 账号显示余额；OpenAI Compatible/Anthropic 暂定占位符）
- Key 脱敏展示（仅展示末 4 位）

### 验收标准
- [ ] `/admin/channels` 每行渠道提供「详情」入口
- [ ] `/admin/channels/{channel_id}/detail` 可正常渲染
- [ ] `window=5m|1h|24h` 可切换并影响统计区间
- [ ] 对 OpenAI Compatible/Anthropic：按 Key 聚合展示用量
- [ ] 对 Codex OAuth：按账号聚合展示用量与余额
- [ ] Key 脱敏，不泄露完整密钥

---

## 2. 方案

### 技术方案
1) 新增管理后台页面：
- 路由：`GET /admin/channels/{channel_id}/detail`
- 模板：`internal/admin/templates/channel_detail.html`

2) 新增用量聚合查询（按凭证）：
- 数据源：`usage_events`
- 维度：`upstream_channel_id` + `upstream_credential_id` + `[since, until)`
- 指标：请求数、成功/失败、输入/输出 Token、最近使用时间
- 兼容：SQLite/MySQL 对聚合时间字段的扫描差异，通过 epoch seconds 表达式规避

3) UI 入口：
- 在 `internal/admin/templates/channels.html` 的渠道操作区增加「详情」按钮

### 影响范围
```yaml
涉及模块:
  - Admin/UI
  - Store/Usage
  - Server/Routes
新增文件:
  - internal/admin/channel_detail.go
  - internal/admin/templates/channel_detail.html
  - helloagents/modules/admin_channel_detail.md
测试:
  - tests/admin/channel_detail_test.go
  - tests/store/usage_credential_stats_test.go
```

### 风险评估
| 风险 | 等级 | 应对 |
|------|------|------|
| 统计口径误解（成功/失败定义） | 低 | 明确按 `status_code` 是否为 2xx 统计 |
| SQLite 聚合时间扫描失败 | 中 | 使用 epoch seconds 表达式聚合并在 Go 侧转回 `time.Time` |
| Key 泄露风险 | 低 | 仅展示 `api_key_hint` 的末 4 位 |

---

## 3. 决策

无新增全局决策。


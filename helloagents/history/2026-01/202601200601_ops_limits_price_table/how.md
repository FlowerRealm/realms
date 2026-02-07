# 技术设计: Realms 补齐 3/4/5（上游限额、价格表、运维能力）

## 方案选择（复杂任务：2 方案）

### 方案 1（推荐）：最小可用 + 可演进（先落地 cc/rpm、生效清晰）

目标：
- 先把“会误导人的部分”做成可控且可验证：**cc（并发）+ rpm（每分钟请求数）** 先真正在运行时生效。
- `tpm/rpd` 先作为配置展示或第二阶段再做强制生效（需要更复杂的统计/预测）。
- 价格表导入落在 `managed_models`（不引入第二套计费 SSOT）。
- 运维能力用现有 `usage_events` 做聚合，避免新增复杂表。

### 方案 2：一次性全量生效（tpm/rpd/cc/rpm 全做硬限）

问题：
- TPM 需要 token 统计（流式场景无法提前得知），做到“硬限”会引入大量近似与边界争议。
- RPD 需要日级窗口计数；若用 DB 统计会增加热点查询，若用内存统计会有重启不一致问题。

结论：先用方案 1，避免用复杂方案把项目拖死。

## 3) Provider 限额（映射到 Realms 的 upstream_channel）

### 数据模型

在 `upstream_channels` 上新增可选字段：
- `limit_cc`：并发上限（NULL=无限制）
- `limit_rpm`：每分钟请求上限（NULL=无限制）
- `limit_rpd`：每日请求上限（先存储，第二阶段再 enforce）
- `limit_tpm`：每分钟 token 上限（先存储，第二阶段再 enforce）

原因：
- Realms 的“provider”最接近的是 `upstream_channel`（它是调度第一层、也是管理面主要对象）。

### 生效策略（第一阶段）

- CC：在 proxy 执行前对选中的 `ChannelID` 做并发 Acquire/Release。
  - 可实现为 `internal/limits` 新增 `ChannelLimits`（内存信号量）。
- RPM：在 scheduler state 内复用“窗口计数”做粗粒度限流。
  - 在 `Scheduler.SelectWithConstraints` 过滤 candidates 时，若 `limit_rpm` 非空且已达上限，则跳过该 channel。
  - 计数时机：选中 channel 后立即 `Inc`（视为请求开始），避免并发穿透。

### 管理后台

- 在 `admin/channels` 的创建/编辑表单中增加四个输入框（支持留空）。
- 展示时对“已生效/未生效”做明确提示（比如 cc/rpm 标注为“已生效”，tpm/rpd 标注为“仅配置”）。

## 4) 价格表导入（写入 managed_models）

### 导入格式支持（第一阶段：2 种）

1) Realms 简化格式（推荐给内部使用）：
```json
{
  "gpt-5.2": {
    "input_usd_per_1m": 0.15,
    "output_usd_per_1m": 0.60,
    "cache_input_usd_per_1m": 0.00,
    "cache_output_usd_per_1m": 0.00
  }
}
```

2) LiteLLM 常见格式（尽力兼容）：
- 识别字段：`input_cost_per_token` / `output_cost_per_token` / `cache_read_input_cost_per_token` 等（若存在）。
- 转换：`usd_per_1m = cost_per_token * 1_000_000`
- 如果字段缺失：按 0 处理或标记失败（以“可解释”为准）。

### 写入策略

- 对每个 model：
  - 不存在：创建 `managed_models`（默认 `status=0` 更安全；由管理员手动启用/绑定渠道后才可用）。
  - 已存在：仅更新定价字段（保留 PublicID/OwnedBy/Status 不变）。
- 导入结果返回：added / updated / unchanged / failed（含原因）。

### 管理后台入口

- 在 `/admin/models` 增加 “导入价格表” 入口：
  - 支持上传 JSON 文件（multipart）
  - 或 textarea 粘贴 JSON（作为 fallback）

## 5) 运维能力

### 5.1 Proxy Status（活跃/最近请求）

数据来源：`usage_events`
- 活跃请求：`state='reserved' AND reserve_expires_at > NOW()`
- 最近请求：按 user_id 分组取 `updated_at` 最大的一条（排除 reserved 可选）

输出：
- 管理后台页面 `/admin/proxy-status`（root 才可访问）
- 同时提供 JSON API `/admin/api/proxy-status` 供页面轮询（5s）

### 5.2 Version API

- 新增 `GET /api/version`：
  - 返回 `internal/version.Info()`（version/commit/date）
  - 可选：当配置了 `version_check.github_owner/repo` 时，附带 latest release 信息（缓存 1h）

### 5.3 Dev 调试落盘（默认关闭）

- 新增 config：
  - `debug.proxy_log.enable`（默认 false）
  - `debug.proxy_log.dir`（默认 `./out`）
  - `debug.proxy_log.max_bytes`（单条最大大小）
- 行为：
  - 仅在 `env=dev` 且 enable=true 时写盘。
  - 强制脱敏：Authorization / x-api-key / Cookie 等。
  - 仅记录失败或非 2xx（减少敏感面与磁盘压力）。

## 安全与性能

- 限额逻辑必须“宁可不生效也不能误限”：默认 NULL=无限制；解析失败按 NULL 处理并在管理后台提示。
- Proxy status 查询要走索引：复用 `idx_usage_events_user_state_time` 与 `idx_usage_events_state_reserve_expires`。
- Dev 落盘必须限制大小与脱敏，避免把用户提示词写入磁盘（或默认截断）。

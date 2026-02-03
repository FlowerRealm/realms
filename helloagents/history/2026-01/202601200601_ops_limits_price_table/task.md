# 任务清单: Realms 补齐 3/4/5（上游限额、价格表、运维能力）

目录: `helloagents/plan/202601200601_ops_limits_price_table/`

---

## 1. Provider 限额（upstream_channels）
- [√] 1.1 新增 DB 迁移：为 `upstream_channels` 增加 `limit_cc/limit_rpm/limit_rpd/limit_tpm`（NULL=无限制），验证 why.md#需求-上游限额配置可见且可控
- [√] 1.2 更新 `internal/store/upstreams.go`：读写上述字段（List/Get/Create/Update），验证 why.md#需求-上游限额配置可见且可控
- [√] 1.3 更新 `internal/admin/templates/channels*.html` + handlers：管理后台可编辑上述字段，并明确标注“cc/rpm 已生效，tpm/rpd 仅配置”，验证 why.md#需求-上游限额配置可见且可控
- [√] 1.4 实现 cc/rpm 生效（第一阶段）：
  - [√] 1.4.1 `internal/limits` 增加 channel 级并发 limiter（按 channel_id）
  - [√] 1.4.2 `internal/scheduler` 增加 channel rpm 过滤（按 limit_rpm）
  验证 why.md#需求-上游限额配置可见且可控

## 2. 价格表导入（managed_models）
- [√] 2.1 新增管理后台入口：在 `/admin/models` 增加“导入价格表”页面/对话框（上传 JSON），验证 why.md#需求-批量导入模型价格
- [√] 2.2 新增导入逻辑：解析 JSON（支持 Realms 简化格式 + LiteLLM 常见字段），生成 added/updated/unchanged/failed 结果，验证 why.md#需求-批量导入模型价格
- [√] 2.3 写入策略：批量 upsert 到 `managed_models`（新建默认 status=0；已存在仅更新价格字段），验证 why.md#需求-批量导入模型价格

## 3. 运维能力
- [√] 3.1 新增 Proxy Status：
  - [√] 3.1.1 `internal/store` 增加聚合查询（活跃请求 + 最近请求）
  - [√] 3.1.2 新增 `/admin/proxy-status` 页面与 `/admin/api/proxy-status`（root-only）
  验证 why.md#需求-运维快速定位问题
- [√] 3.2 新增 Version API：`GET /api/version` 返回 build info（可选：GitHub latest release），验证 why.md#需求-运维快速定位问题
- [√] 3.3 Dev 调试落盘（默认关闭）：
  - [√] 3.3.1 增加 config `debug.proxy_log.*`
  - [√] 3.3.2 在数据面代理失败时写入脱敏日志（限制大小/数量）
  验证 why.md#需求-运维快速定位问题

## 4. 安全检查
- [√] 4.1 安全检查：限额默认行为、导入数据校验、日志脱敏与磁盘保护、权限（root-only），验证 why.md#风险评估

## 5. 测试
- [√] 5.1 为 store/scheduler/limits 的新增逻辑补充单元测试，并运行 `go test ./...`

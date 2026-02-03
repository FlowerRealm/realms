# 技术设计: 渠道绑定模型（channel_models）

## 技术方案

### 核心技术
- Go `net/http`（现有）
- MySQL migrations（现有）
- SSR 管理后台（现有）

### 实现要点
1. 新增 `channel_models`：`channel_id + public_id` 唯一，支持 `upstream_model` 与 `status`
2. `managed_models` 只负责元信息与全局启用/禁用
3. 调度约束从“单 channel/type”扩展为“允许的 channel 集合”
4. handler 逻辑调整为：
   - 校验 public model
   - 取该模型允许的 channel 集合
   - 在集合内调度得到 selection
   - 用选中 channel 对应的 upstream_model 改写并转发

## 数据模型

```sql
CREATE TABLE IF NOT EXISTS `channel_models` (
  `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
  `channel_id` BIGINT NOT NULL,
  `public_id` VARCHAR(128) NOT NULL,
  `upstream_model` VARCHAR(128) NOT NULL,
  `status` TINYINT NOT NULL DEFAULT 1,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  UNIQUE KEY `uk_channel_models_channel_public` (`channel_id`, `public_id`),
  KEY `idx_channel_models_public_id` (`public_id`),
  KEY `idx_channel_models_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

## 安全与性能
- 管理面仅 root
- 绑定校验：`channel_id` 必须存在；`public_id` 必须存在于 `managed_models`

## 测试与部署
- 补齐 handler/scheduler/store 单测
- `go test ./...` 通过


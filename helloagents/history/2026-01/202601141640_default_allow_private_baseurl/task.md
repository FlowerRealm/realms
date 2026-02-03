# 任务清单（轻量迭代）

目标：允许使用内网地址作为上游 `base_url`（仍保留基础校验）。

- [√] 默认配置调整：移除 base_url 地址范围限制相关开关
- [√] 示例配置同步：`config.example.yaml` 默认值改为 `true`
- [√] 文档同步：更新安全说明（README/Changelog）
- [√] 测试验证：`go test ./...`
- [√] 迁移方案包至 `helloagents/history/` 并更新索引

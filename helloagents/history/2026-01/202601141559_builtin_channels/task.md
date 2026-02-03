# 任务清单（轻量迭代）

目标：将上游 Channel 改为内置（自动初始化），避免用户在管理后台手动创建/输入 Channel。

- [√] 自动初始化内置 Channel：仅 `codex_oauth`（迁移脚本幂等插入；`openai_compatible` 不提供默认值）
- [√] 管理后台 Channels UI：至少保证 `codex_oauth` 不需要手动创建（隐藏创建入口 + 服务端校验 + 禁止删除）
- [√] 文档更新：补充“Channel 为内置”的使用说明；更新 Changelog
- [√] 测试验证：`go test ./...`
- [√] 迁移方案包至 `helloagents/history/` 并更新索引

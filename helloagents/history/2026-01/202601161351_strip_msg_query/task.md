# 任务清单: 清理 URL msg 参数

目录: `helloagents/plan/202601161351_strip_msg_query/`

---

## 1. URL 清理（避免污染浏览器历史）
- [√] 1.1 在 `internal/web/templates/base.html` 中加入 `history.replaceState`，页面加载后移除 `msg` query 参数
- [√] 1.2 在 `internal/admin/templates/base.html` 中加入 `history.replaceState`，页面加载后移除 `msg` query 参数

## 2. 安全检查
- [√] 2.1 确认仅重写当前 URL（删除 `msg` 参数），不引入重定向/XSS 风险

## 3. 文档更新
- [√] 3.1 更新 `helloagents/wiki/modules/realms.md` 记录该行为
- [√] 3.2 更新 `helloagents/CHANGELOG.md` 增加修复记录
- [√] 3.3 更新 `helloagents/history/index.md` 增加变更索引

## 4. 测试
- [√] 4.1 运行 `go test ./...`

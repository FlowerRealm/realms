# 任务清单: 管理后台侧边栏溢出修复

目录: `helloagents/plan/202601190615_admin_sidebar_overflow/`

---

## 1. 管理后台模板/样式
- [√] 1.1 在 `internal/admin/templates/base.html` 调整侧边栏样式：容器固定高度并隐藏溢出，导航区域支持纵向滚动，验证 why.md#需求-侧边栏可滚动-场景-菜单项超过一屏
  > 备注: 补充 `flex-wrap: nowrap`，避免在高度不足时被 flex wrap 挤成“多列”。
- [√] 1.2 在 `internal/admin/templates/base.html` 为导航列表增加滚动容器 class（如 `.sidebar-nav`），确保顶部/底部区域不随滚动，验证 why.md#需求-侧边栏可滚动-场景-菜单项超过一屏
- [?] 1.3 验证移动端（<768px）展开侧边栏时滚动正常，验证 why.md#需求-移动端可用性不退化-场景-移动端展开侧边栏
  > 备注: 未进行真实浏览器/真机验证；建议在移动端或 DevTools 响应式模式下手动确认滚动与展开行为。

## 2. 安全检查
- [√] 2.1 执行安全检查（按G9: 不引入外部脚本、不新增敏感信息处理、不改变权限控制路径）

## 3. 文档更新
- [√] 3.1 更新 `helloagents/CHANGELOG.md` 记录“管理后台侧边栏溢出修复”

## 4. 测试
- [?] 4.1 手动回归：访问主要管理页面确认布局无回归（`/admin`、`/admin/settings`、`/admin/users`、`/admin/tickets`）
  > 备注: 已执行 `go test ./...` 通过；但未进行浏览器层面的页面回归检查。

# 技术方案: 公告功能

## 数据模型

新增两张表：

1) `announcements`
- `title` / `body`
- `status`：0=草稿，1=已发布
- `created_at/updated_at`

2) `announcement_reads`
- `user_id + announcement_id` 作为联合主键
- `read_at` 记录首次阅读时间（写入幂等）

## 路由与权限

### 用户控制台（SessionAuth）
- `GET /announcements`：公告列表（已发布）
- `GET /announcements/{announcement_id}`：公告详情（已发布；进入即标记已读）
- `POST /announcements/{announcement_id}/read`：标记已读（CSRF）
- `GET /dashboard`：若存在未读公告，注入弹窗数据并自动弹出

### 管理后台（SessionAuth + RequireRoles(root) + CSRF）
- `GET /admin/announcements`：公告管理页
- `POST /admin/announcements`：创建公告
- `POST /admin/announcements/{announcement_id}`：发布/撤回（status 切换）
- `POST /admin/announcements/{announcement_id}/delete`：删除公告（同时清理已读记录）

## 交互设计（SSR）

- 用户侧：侧边栏新增“公告”入口；列表展示已读/未读；详情展示正文
- Dashboard：存在未读公告时自动弹出 Bootstrap Modal；用户点击“我知道了”后标记已读并刷新（若仍有未读则继续弹出下一条）
- 管理侧：表格列出公告状态与时间；提供创建弹窗与行内操作（发布/撤回/删除）

## 安全与边界

- 管理入口严格限制为 `root`
- 所有写操作均走 CSRF 链路（用户标记已读 / 管理端创建/发布/删除）
- `redirect` 参数仅允许站内路径，避免 open redirect


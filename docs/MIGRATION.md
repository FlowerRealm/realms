# 迁移说明：从旧桌面壳到 `realms-app`

本项目已将“桌面入口”统一为 Go 的 `realms-app`（`cmd/realms-app`）。它会启动本地后端，并在启动日志中提示访问 `/login` 的地址（不自动打开浏览器）。

本文只保留迁移与历史说明：**不再提供、也不再建议任何 Electron 构建/发布流程**。

---

## 你需要迁移什么？

如果你之前使用旧桌面壳（Electron）跑的是 **SQLite personal 模式**，通常需要迁移：

- 数据库文件：`realms.db`
- 附件目录：`tickets/`（如果你用过工单/附件）

如果你之前使用的是 Docker Compose（MySQL）/Server（business 模式），一般不需要迁移桌面侧数据文件。

---

## 迁移步骤（SQLite personal 模式）

### 1) 安装并启动 `realms-app`

- 直接下载：GitHub Releases 的 `realms-app`（Windows 为 `realms-app.exe`）
- 源码运行（当前平台）：`make app-dev`
- 源码打包（当前平台）：`make app-dist`

### 2) 把旧数据拷贝到新数据目录

`realms-app` 默认把数据放在“用户配置目录”的 `Realms/` 下（由 Go 的 `os.UserConfigDir()` 决定）：

- macOS：`~/Library/Application Support/Realms/`
- Windows：`%APPDATA%\\Realms\\`
- Linux：`~/.config/Realms/`

将旧桌面壳的数据拷贝到上述目录：

- `realms.db` → `Realms/realms.db`
- `tickets/` → `Realms/tickets/`

> 找不到旧数据位置时：在你的磁盘上搜索 `realms.db`，它通常位于 Electron 的 `userData` 目录下（应用名可能是 `Realms` 或 `realms-desktop`；仅供历史参考）。

### 3) 保持“仅本机访问”（可选）

旧桌面壳固定监听 `127.0.0.1:8080`；`realms-app` 默认监听 `:8080`（对局域网可访问）。

如需保持仅本机访问，启动前设置：

```bash
export REALMS_ADDR="127.0.0.1:8080"
```

Windows（PowerShell）：

```powershell
$env:REALMS_ADDR = "127.0.0.1:8080"
```

---

## 常见问题

### 迁移后我的客户端要改什么？

不需要改协议：依旧使用 OpenAI 兼容的 `base_url`：

- `OPENAI_BASE_URL=http://127.0.0.1:8080/v1`

鉴权在 personal 模式下仍是管理 Key（首次打开 `/login` 设置）。

### 我可以继续使用旧桌面壳吗？

不建议。旧桌面壳仅作为历史参考，不再维护，也不会再提供构建/发布指引。

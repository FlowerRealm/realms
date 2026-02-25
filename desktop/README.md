# Realms Desktop（Electron，自用模式）

本目录提供一个 Electron 桌面壳，用于把 Realms 以“自用模式”封装成桌面应用：
- 桌面应用启动时会拉起内置 Realms 后端（本机 `127.0.0.1` 固定端口）
- BrowserWindow 直接加载 `http://127.0.0.1:<port>/login`

## 端口（固定）

- 桌面版固定监听：`127.0.0.1:8080`
- 端口被占用时：桌面版会启动失败并提示用户释放端口（端口不会自动改动）

## 构建

详见：`docs/USAGE.md` 的“桌面版（Electron，自用模式）”章节。

# 使用说明

## 服务端运行

源码运行：

```bash
npm --prefix web ci
npm --prefix web run build
go run ./cmd/realms
```

或使用：

```bash
make dev
```

## 已删除的旧路径

以下旧入口已删除，不再支持：

- `cmd/realms-app`
- `make app-dev`
- `make app-dist`
- `make app-set-key`
- `REALMS_MODE`

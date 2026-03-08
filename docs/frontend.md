# 前端说明

## 构建产物

- 默认静态目录：`./web/dist`
- 可通过 `FRONTEND_DIST_DIR` 覆盖
- 可通过 `FRONTEND_BASE_URL` 使用外置前端

## 本地构建

```bash
npm --prefix web ci
npm --prefix web run build
```

项目已删除 personal 前端入口，不再生成 `web/dist-personal`。

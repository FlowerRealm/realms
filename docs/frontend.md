# 前端说明

## 构建产物

- 默认静态目录：`./web/dist`
- 生产部署仅支持嵌入式同源前端；构建镜像时会将 `web/dist` 嵌入后端二进制

## 本地构建

```bash
npm --prefix web ci
npm --prefix web run build
```

项目已删除 personal 前端入口，不再生成 `web/dist-personal`。

# 任务清单: light-green-theme

> **@status:** completed | 2026-02-19 09:42

目录: `helloagents/archive/2026-02/202602190938_light-green-theme/`

---

## 任务状态符号说明

| 符号 | 状态 | 说明 |
|------|------|------|
| `[ ]` | pending | 待执行 |
| `[√]` | completed | 已完成 |
| `[X]` | failed | 执行失败 |
| `[-]` | skipped | 已跳过 |
| `[?]` | uncertain | 待确认 |

---

## 执行状态
```yaml
总任务: 4
已完成: 4
完成率: 100%
```

---

## 任务列表

### 1. Web 主题

- [√] 1.1 在 `web/src/index.css` 中将主题主色调整为更浅的绿色，并全局覆盖 `nav-pills` 激活态避免亮蓝色
  - 验证: 访问 `/admin/settings`，tabs 激活态不再呈现默认亮蓝色；主按钮/链接命中主题色

- [√] 1.2 在 `web/e2e/theme-colors.spec.ts` 中同步更新主题 RGB 断言，覆盖 `nav-pills` 与 `btn-primary` 的回归校验
  - 依赖: 1.1

### 2. 验证

- [√] 2.1 运行 `npm -C web run check:theme`
  - 依赖: 1.1

- [√] 2.2 运行 `npm -C web run build` 与 `npm -C web run test:e2e:ci -- e2e/theme-colors.spec.ts`
  - 依赖: 1.2

---

## 执行备注

> 执行过程中的重要记录

| 任务 | 状态 | 备注 |
|------|------|------|

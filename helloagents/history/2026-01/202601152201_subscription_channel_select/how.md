# How：实现方案（最小改动、可回滚）

## 1) 数据结构

- 在 `subscription_plans` 增加 `channel_group` 字段（默认 `default`）。
- 仍复用既有 `channel_groups` 字典与后台分组管理页作为下拉数据源。

## 2) 管理后台（SSR）

- `/admin/subscriptions` 的新增/编辑表单增加 `channel_group` 下拉选择。
- 列表展示套餐所属的 `channel_group`，便于审核与排查。
- 服务端保存时校验所选分组必须存在且启用。

## 3) 数据面调度绑定订阅

- 配额预留（Reserve）在选择到具体“可用订阅”后，把该订阅套餐的 `channel_group` 一并返回。
- OpenAI 兼容数据面在调度前，将 `Constraints.RequireChannelGroup` 覆盖为本次请求使用订阅的分组。
  - 这样粘性绑定命中也会做分组校验，避免跨组绕过。

## 4) 用户侧展示（可选增强）

- 订阅购买页展示套餐的渠道分组（便于用户理解购买内容）。


# 上游渠道模型后缀保护名单（Responses）

## 背景

为对齐 new-api 的 Responses 请求处理能力，Realms 支持从 OpenAI Responses 请求体的 `model` 字段解析推理力度后缀，并写入 `reasoning.effort`：

- `-low/-medium/-high/-minimal/-none/-xhigh`

同时会将 `model` 去掉对应后缀后再转发到上游。

但在部分代理/上游中，模型名可能天然包含这些后缀（不应当被当作“推理力度后缀”解析），因此需要一个可配置的“保护名单”用于跳过自动解析。

## 字段（upstream_channels）

- `model_suffix_preserve`：模型后缀保护名单（JSON 数组，元素为模型名字符串）

> 为空或 `[]` 表示禁用。

## 语义

当 `/v1/responses` 转发前检测到：

- `model` 以 `-low/-medium/-high/-minimal/-none/-xhigh` 结尾

且 `model_suffix_preserve` **命中**（匹配任一项）时：

- 不做后缀解析与替换（保持 `model` 原样转发，不注入 `reasoning.effort`）

匹配规则：

- 支持匹配“对下游的 public model”或“alias rewrite 后的 upstream model”（任一匹配即视为命中）

## 生效位置

- 仅对 `/v1/responses` 的“推理力度后缀解析”生效
- failover 到其他渠道时，会按新渠道的 `model_suffix_preserve` 重新判定

## 管理后台

- 在 `/admin/channels` 的渠道行点击“设置”，在“模型后缀保护名单”折叠项中配置
- Admin Config 导出/导入版本为 `5`，导入兼容 `1/2/3/4/5`（`v5` 起包含该字段）

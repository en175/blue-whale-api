# vendors — 厂商

!!! info "模块 / 定位"
    **模块:** 渠道与模型

    **定位:** 模型厂商(火山方舟/OpenAI/智谱…)。给 `models` 展示用。

## 表结构

| 列名 | 含义 |
|---|---|
| `name` | 厂商名(如 火山方舟) |
| `description` | 描述 |
| `icon` | 图标 |
| `status` | 状态 |

## 业务规则

- 被 `models.vendor_id` 引用
- Pricing 接口的 `vendors` 字段来自此表(展示「火山/OpenAI」标签)

## 关联表

- `models`:通过 vendor_id 被引用(一个厂商对多个模型)

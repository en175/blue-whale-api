# quota_data — 用量统计(多维聚合)

!!! info "模块 / 定位"
    **模块:** 计费与财务

    **定位:** 按 用户/模型/分组/token/渠道/节点 维度的用量统计(定时聚合)。**统计表,非实时计费源。**

## 关键列

| 列名 | 含义 |
|---|---|
| `user_id`/`username` | 用户 |
| `model_name` | 模型 |
| `use_group` | 分组 |
| `token_id`/`channel_id`/`node_name` | token/渠道/节点维度 |
| `token_used` | token 用量 |
| `count` | 调用次数 |
| `quota` | quota 消耗 |
| `created_at` | 统计时间 |

!!! warning "易错点"
    - **统计聚合表,不是原始消费**。原始在 `logs`
    - 用于报表/分析,不要当计费/对账依据

## 关联表

- `users`/`tokens`/`channels`(多维度引用)

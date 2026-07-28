# user_subscriptions — 用户订阅(有效订阅)

!!! info "模块 / 定位"
    **模块:** 计费与财务

    **定位:** 用户当前/历史的订阅记录。订阅期内有独立额度池。

## 关键列

| 列名 | 含义 |
|---|---|
| `user_id`/`plan_id` | 用户/档位 |
| `amount_total`/`amount_used` | **订阅额度池:总额度/已用(独立于 users.quota)** |
| `start_time`/`end_time` | 订阅起止 |
| `status` | 有效/过期 |
| `upgrade_group`/`prev_user_group`/`downgrade_group` | 当前分组/原分组/到期回退 |
| `last_reset_time`/`next_reset_time` | 配额重置时间 |

!!! warning "易错点"
    - `amount_total/amount_used` 是**订阅额度池**,独立于 `users.quota`
    - 消费优先扣订阅额度(见 subscription_pre_consume_records),扣完再扣 users.quota

## 关联表

- `subscription_plans`:plan_id
- `subscription_pre_consume_records`:每次预扣
- `users`:分组变更

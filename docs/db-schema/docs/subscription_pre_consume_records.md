# subscription_pre_consume_records — 订阅预扣记录

!!! info "模块 / 定位"
    **模块:** 计费与财务

    **定位:** 订阅额度池的每次预扣。请求来时若有有效订阅,先从订阅额度扣。

## 关键列

| 列名 | 含义 |
|---|---|
| `request_id` | 请求ID(唯一) |
| `user_id` | 用户 |
| `user_subscription_id` | 订阅 → user_subscriptions |
| `pre_consumed` | 预扣量 |
| `status` | 预扣/确认/回退 |

## 业务规则

```text
请求 → 有有效订阅 → 预扣订阅额度(pre_consumed)→ 结算确认/失败回退
```

## 关联表

- `user_subscriptions`:user_subscription_id

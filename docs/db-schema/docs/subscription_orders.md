# subscription_orders — 订阅订单

!!! info "模块 / 定位"
    **模块:** 计费与财务

    **定位:** 订阅支付订单。下单 → 支付 → 生成 user_subscription。

## 关键列

| 列名 | 含义 |
|---|---|
| `user_id` | 用户 |
| `plan_id` | 档位 → subscription_plans |
| `money` | 实付金额 |
| `trade_no` | 订单号(唯一) |
| `payment_method`/`payment_provider` | 支付方式/渠道 |
| `status` | 订单状态 |

## 业务规则

```text
下单 → 支付成功 → 生成 user_subscription + users 变 upgrade_group + 加 total_amount quota
```

## 关联表

- `subscription_plans`:plan_id
- `user_subscriptions`:支付成功生成

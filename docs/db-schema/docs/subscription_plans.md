# subscription_plans — 订阅档位

!!! info "模块 / 定位"
    **模块:** 计费与财务

    **定位:** 订阅会员档位定义(VIP/SVIP 套餐)。买档位 → 变分组 + 得 quota + 时长。

## 关键列

| 列名 | 含义 |
|---|---|
| `title`/`subtitle` | 档位名/副标题 |
| `price_amount`/`currency` | **价格(钱)/币种** |
| `duration_unit`/`duration_value`/`custom_seconds` | 时长(天/月/自定义秒) |
| `total_amount` | **含的 quota 总额(int,不是钱)** |
| `upgrade_group`/`downgrade_group` | 买这个变哪个分组/到期回哪个(对应 GroupRatio) |
| `quota_reset_period` | 配额重置周期(每月/每周) |
| `stripe_price_id`/`creem_product_id`/`waffo_pancake_product_id` | 各支付平台商品ID |

!!! warning "易错点"
    - `price_amount` 是钱,`total_amount` 是 quota —— **别混**
    - `upgrade_group` 对应 `options.GroupRatio` 的分组(决定折扣)

## 关联表

- `user_subscriptions`/`subscription_orders`:购买记录
- `options.GroupRatio`:upgrade_group 对应的分组系数

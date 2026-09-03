# top_ups — 充值订单

!!! info "模块 / 定位"
    **模块:** 计费与财务

    **定位:** 用户充值订单(法币 → quota)。**充值成功 → `users.quota += amount`。**

## 关键列

| 列名 | 含义 |
|---|---|
| `user_id` | 充值用户 |
| `amount` | 钱包流/代收：美元额度 N，回调 `users.quota += N × QuotaPerUnit`。档位流：raw tokens，回调直接加。**不是**实付金额。 |
| `money` | **实付法币(才是钱)** |
| `trade_no` | 订单号(唯一) |
| `payment_method` | 支付方式（渠道族）：钱包新记录 `wxpay`/`alipay`，存量可能是 `bluewhalepay` |
| `payment_provider` | 支付网关（如 `bluewhalepay`），不是微信/支付宝 |
| `status` | 订单状态 |

!!! warning "易错点"
    - `amount` 与 `money` **别混**：`money` 是实付法币；钱包流/代收的 `amount` 是 N，加账时才变成 quota
    - 代收口 `POST /internal/v1/topup/pay` 按下游写入 Money/Amount，不走 `getPayMoney`
    - status 到「完成」才加 quota
    - `payment_method` 不是网关名；钱包流新记录用它区分微信/支付宝，`payment_provider` 才是 bluewhalepay

## 关联表

- `users`:user_id,完成时加 users.quota

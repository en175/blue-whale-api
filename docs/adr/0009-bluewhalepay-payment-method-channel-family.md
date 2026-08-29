# 钱包 BlueWhalePay 的 payment_method 落渠道族，不回传付款码

IKBVQQ：产品账户记录要区分微信 / 支付宝，但钱包建单把 `top_ups.payment_method` 写成网关名 `bluewhalepay`，`GET /api/user/topup/self` 无法区分。付款码（`WX_NATIVE` / `ALI_QR`）只用于向 pay 下单，不落库、不进查询口。档位流这张单不动。

**Status:** accepted

## 决策

- **支付网关** `payment_provider` 仍为 `bluewhalepay`。
- **支付方式** `payment_method`：钱包建单按付款码前缀写入 `wxpay` / `alipay`（空走默认 `ALI_QR` → `alipay`，未知前缀 → `bluewhalepay` 不拒单）；回调 pending→success 时若已验签 `ifCode` 为 `wxpay`/`alipay` 且与现值不同则覆盖。存量 `bluewhalepay` 不迁移。
- **查询口不变**：下游继续读现有 `payment_method`。不加 `wayCode` 字段。
- **范围**：只改 `RequestBlueWhalePayPay` 与 `BlueWhalePayNotify` / `RechargeBlueWhalePay`。档位流保持 `wechat`/`alipay`。

## 被否掉的选项

- **查询口加 wayCode**：账户记录只要微信 / 支付宝，付款码是扫码形态，产品列表用不上。
- **本单同时改档位流**：产品未走该路径，两套值对齐可另开。
- **未知付款码拒单**：pay 新码会变成充值失败；兜底 `bluewhalepay` 与存量同一待遇。

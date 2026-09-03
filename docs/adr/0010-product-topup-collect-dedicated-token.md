# 产品代收口使用独立机机凭证，不复用铸券令牌

IKBWY4：产品充值改为下游定价、Go 代收。网关已有 `BLUE_WHALE_INTERNAL_SERVICE_TOKEN`，只给 `POST /internal/v1/model-capabilities` 铸发能力券（Agent Run 短命熔断凭证）。代收是另一件财务动作：按下游报的实付建收款单、回调后加额度。

**Status:** accepted

决定代收口用单独的机机凭证，不复用铸券令牌，也不用 root / 用户 JWT。同一 Nest 进程被打穿时两把钥匙仍会一起漏；拆开挡的是权限混用——避免「内部服务令牌」慢慢变成 `/internal/v1` 万能钥匙，也避免铸券泄漏面扩大到造充值单。

凭证只证明「这次下单是 BlueWhale 后端发的」，不核对产品折扣。Go 不按 `AmountDiscount` 同意或拒绝折后价；定价留在 Node `topup_tiers`。env `BLUE_WHALE_TOPUP_COLLECT_TOKEN` 配在 Go（验）和 Node（带）；请求头 `X-Blue-Whale-Collect-Token`（不加 Bearer），避免和铸券的 `Authorization` 混用。用户 JWT 不进代收口，用户身份只在 body 的 `user_id`。


路由挂 `POST /internal/v1/topup/pay`（与铸券同前缀、不同凭证），不挂用户钱包 `POST /api/user/bluewhalepay/pay`，也不用看起来像对外 API 的 `/api/blue-whale/...`。

入参：`money` 为下游折后实付（元，写入 `top_ups.money`）；`amount` 为美元额度 N（JSON 整数，与今天钱包口相同，例如档位原价 37，写入 `top_ups.amount`）；`way_code` 必填（`WX_NATIVE` / `ALI_QR`），空值不回落到钱包默认渠道。回调仍走 `/api/bluewhalepay/notify`：`users.quota += N × QuotaPerUnit`。禁止把已乘好的 quota 写入 `amount` 却复用该回调（会再乘一次）。折扣只减实付，不减到账 quota。

上限（超限 / 非有限 / `amount` 非正整数 → 400，不建单）：`money` ≥ 0.01 且 ≤ 100000 元；`amount` ≥ 1，且 `N × QuotaPerUnit` 不得超过 `common.MaxQuota`（int32 账本容量）。

`user_id` 必须是存在且启用的网关用户，否则 400、不建单。停用或不存在的账号不能拉码、不能加额度。

拉码失败（支付网关不可用等）与钱包口一致：pending 充值单保留，接口报错；不删除、不改成失败。用户重试会再开一张新单。

HTTP 与钱包 handler 一致：成功失败都 200；成功 `message=success` 且 `data.pay_data` / `data.order_id`；失败 `message=error`、`data` 为原因。不单独用 401/400 区分鉴权与参数（本口鉴权也走同一套 200 包）。





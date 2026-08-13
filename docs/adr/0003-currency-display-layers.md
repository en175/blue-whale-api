# 货币展示三层独立：额度展示类型纯展示、计费美元锚点、DIRECTORY 固定人民币

IK30C5/IK30PV 曾将展示层统一为人民币并移除 USD/CUSTOM 货币切换；IK8F4E 因运营需要恢复了 USD 展示选项。本文档记录系统内三层相互独立的「货币展示」口径，替代 IK30PP 中规划但从未落盘的 ADR-0021。

**Status:** accepted

## 背景

- 系统内部计费始终美元锚点：quota 是中性积分/游戏币，`QuotaPerUnit = 500000` quota = $1；数据库与计费核心算法从不带货币属性（IK30C5 红线，不动）。
- IK30C5（统一 RMB 计费）规划将所有面向用户的货币口径切为人民币，其中展示层部分（IK30PV）于 2026-07 落地：默认 `quota_display_type=CNY`，系统设置移除 USD/CUSTOM 切换。其依据文档 ADR-0021 与 CONTEXT.md 计费术语段（IK30PP）从未落盘。
- 2026-08 运营反馈需要美元展示（IK8F4E），恢复货币设置页的 USD 选项。

## 决策

### D1：`quota_display_type` 是纯展示层开关

只决定余额/账单/日志/报表等金额的展示符号与换算率，不进入任何计费计算。计费核心（quota 定义、倍率、价格模式存储）始终美元锚点。

### D2：展示层可选 USD / CNY / TOKENS，默认 CNY；CUSTOM 保持下线

IK8F4E 恢复 USD 选项；后端默认值保持 CNY。CUSTOM 常量保留以兼容历史值与 API，前端不暴露切换入口。

### D3：DIRECTORY 定价页固定人民币，不受 `quota_display_type` 影响

公开定价页（`/pricing`，默认 DIRECTORY 视图，IK3QRF/IK4BWE）用 `formatRmbPriceFromUSD` 渲染，恒为 ¥。这是独立产品决策（面向看人民币定价的场景），有意与全站展示类型解耦。

### D4：模型定价输入的 ¥/$ 开关是独立的管理员 UX

管理员填写模型定价（IK30R5）时可选 ¥ 或 $ 输入；¥ 输入由前端按汇率换算为 USD 后存储，内部存储值与提交链路始终 USD。与 `quota_display_type` 无关。

## 影响

- 选择 USD 展示时会出现「余额/账单是 $，DIRECTORY 定价页是 ¥」——预期行为，非缺陷。
- `USDExchangeRate` 字段在 USD 模式下仍显示且可编辑，但不影响 USD 渲染（换算率强制为 1），仅 CNY 模式生效；保留 IK30PV 之前的既有行为。
- 未来如需三层统一（例如 DIRECTORY 跟随展示类型），应另开工单并更新本 ADR。

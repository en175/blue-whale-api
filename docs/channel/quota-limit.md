# 渠道配额限额说明（IK9W58）

渠道支持两类配额限额，用于给单渠道设置成本护栏，超限的渠道会被路由过滤器自动剔除：

| 限额 | 字段 | 含义 | 计数来源 |
| --- | --- | --- | --- |
| 周期限额 | `period_quota_limit` / `period_hours` | 每 `period_hours` 小时一个窗口，窗口内已用额度达到上限即剔除 | `period_used_quota` |
| 累计硬上限 | `hard_quota_limit` | 累计消耗达到上限即剔除，永不清零（需人工调整） | `used_quota` |

- 限额单位与余额一致：**¥1 = 500,000 quota**。后台编辑时按金额输入会自动换算。
- 周期窗口在保存渠道时按 `period_hours` 重算 `period_reset_time`；后台改动 `period_hours` 会立即清零当前周期用量并把下次重置点推进到 `现在 + period_hours`。
- 硬上限是**累计**的，不受 `period_hours` 影响，改周期小时数不会恢复被硬上限剔除的渠道。

## 拦截机制与滞后窗口

路由过滤 `FilterChannelsByQuotaLimit`（`model/channel.go`）按 `ChannelIsQuotaExceeded` 判定。判定基于两层数据，各有一级滞后：

1. **内存渠道快照**：过滤器读取 `channelsIDM` 快照，该快照每 `SYNC_FREQUENCY`（默认 60s）从库重灌一次。→ 快照滞后最长一个 `SYNC_FREQUENCY`。
2. **批量落库**：`used_quota` / `period_used_quota` 经 `batchUpdate` 每 `BATCH_UPDATE_INTERVAL`（默认 5s）才写库。→ 落库滞后最长一个 `BATCH_UPDATE_INTERVAL`。

`ChannelIsQuotaExceeded` 会叠加**尚未批量落库的实时增量**（`pendingBatchDelta`），因此批量落库窗口（≤5s）已被堵住，剩余滞后主要来自内存快照（≤`SYNC_FREQUENCY`）。也就是说：首次超限到拦截最多约 `SYNC_FREQUENCY + BATCH_UPDATE_INTERVAL`。

超限后渠道被**静默剔除路由**，请求报「分组 X 下模型 Y 无可用渠道」，后台渠道列表需看「Period Quota Limit / Hard Quota Limit」列确认超限状态。

## 调优建议

- **收窄拦截窗口**：调小 `SYNC_FREQUENCY`（如 10s）与 `BATCH_UPDATE_INTERVAL`（如 2s）。代价是渠道缓存重灌与批量写库更频繁，DB/CPU 压力略升。
- **单笔大请求击穿**：限额按「预期消耗 × 3」留余量，给窗口滞后缓冲。
- **避免静默**：给限额留余量并定时巡检用量（如到 80% 告警），不要等超限才发现渠道被踢。
- **硬上限误配置**：若渠道被硬上限剔除，先核对 `used_quota` 与 `hard_quota_limit` 的实际值（可能把 ¥ 当 quota 原始单位填过小）。
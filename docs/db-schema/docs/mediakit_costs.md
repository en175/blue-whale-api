# mediakit_costs — MediaKit 任务审计

!!! info "模块 / 定位"
    **模块:** 计费与财务

    **定位:** 火山 AI MediaKit 异步任务（`/v1/mediakit/{tool}`）到达终态时的结算快照，供按工具 / 渠道 / 时段与上游账单对账。**不对用户扣 quota，不进渠道限额。**

    与 `enhance_costs` 的区别：`enhance_costs` 记的是**上游成本**（官方元/分钟价表 × 真实时长，人民币分）；本表**不记成本**，只记网关确知的归属与实扣，成本交给对账方用「`actual_seconds` × 官方费率」在 SQL 外算。原因见下。

## 关键列

| 列名 | 含义 |
|---|---|
| `task_id` | 网关公开任务 ID（`task_xxx`），非上游 ID |
| `channel_id` | 渠道 ID |
| `tool` | 上游工具 slug（如 `enhance-video-generative`），**非**对客模型名 `mediakit-<tool>` |
| `billing_mode` | `duration` / `per_call` / `volume`，取自工具注册表 |
| `upstream_job_id` | 上游真实任务号，关联对账用 |
| `status` | 网关终态：`success` / `failure` |
| `estimated_minutes` | 提交预扣用的预估分钟（时长类工具才有，其余为 0） |
| `actual_seconds` | 上游回包的真实处理时长（秒）。**0 表示取不到，不是时长为零** |
| `charged_quota` | 退款与差额结算**之后**的最终实扣额度，不是预扣值 |

## 业务规则

- 一个任务一行，在「推进到终态」的那次 CAS 成功时写入（`service/task_polling.go`）。重复轮询同一个已终态任务不会写重复行。
- 写入时机在差额结算与退款**之后**：失败退款的记 `charged_quota = 0`（官方规则「任务处理失败不触发计费」）。
- 超时被清扫的任务同样落一行（`status = failure`），因为这类任务上游可能仍在跑并计费，是本表最需要记录的形态之一；此时 `actual_seconds` 为 0。
- `actual_seconds` 取不到（上游时长字段名未命中或值非法）时记 0，对账方需按上游账单核对这类缺口。后端日志会打 `mediakit 任务 <id> 未取到真实时长…result 键：…` 供收敛字段名。
- 金额不进 `logs.quota`、不进 `channels.used_quota` / 周期限额 / 硬上限。
- 只有实现了 `service.TaskCostRecorder` 的适配器才落行；目前仅 MediaKit 渠道（类型 65）实现，其他渠道行为不变。

## 为什么没有 amount_fen

官方计价是「工具 × 输出档（按**短边**判定）× 帧率档」的矩阵，且三种计费模式的量纲不同（元/千次、元/分钟、元/GiB），GiB 档官方还是自然月阶梯。这套费率尚未逐工具核实。

把猜测的费率硬编进代码会把错误固化进**已迁移的库**，事后修正要写数据订正；而把用户实扣（quota）换算成人民币填进 `amount_fen` 更糟——名字说的是上游成本，内容却是收费，对账时差额会被读成「网关算错了」，实际那是加价策略，且表内没有任何字段能区分两者。

所以本表只记确知的数。官方价表落地后（详见 ADR-0018「后续」），再另加一列 `upstream_amount_fen` 与 `charged_quota` 并列，即可直接读出加价率。

## 易错点

- **不要**把本表当成成本表做盈亏统计。
- **不要**把 `tool` 与对客模型名混用：前者是上游 slug，后者带 `mediakit-` 前缀。
- 表名**不能**叫 `enhance_costs`：那是超分专用，管理台「仅超分」筛选依赖它做 `EXISTS`。

## 关联表

- `tasks`: 按 `task_id` 对账（`status` / `quota` 以 `tasks` 为准，本表是结算后的快照）
- `channels`: 按 `channel_id` 汇总
- `enhance_costs`: 超分任务会在两张表各留一行，超分那行的 `task_id` 与本表同值

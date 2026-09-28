# enhance_costs — 超分花费

!!! info "模块 / 定位"
    **模块:** 计费与财务

    **定位:** MediaKit 画质增强（大模型版）每次尝试的人民币花费（整数分）。对账用 SQL；管理台任务日志用 `EXISTS` 筛「仅超分」。**不对用户扣 quota，不进渠道限额。**

## 关键列

| 列名 | 含义 |
|---|---|
| `task_id` | 公开任务 ID |
| `channel_id` | 渠道 ID |
| `requested_resolution` | 请求档（超分输出档） |
| `upstream_resolution` | 上游档（Seedance 实际生成） |
| `output_duration_ms` | 输出时长（毫秒） |
| `fps` | 查价表用的帧率 |
| `rate_tier` | 价表档：`720p` / `1080p` / `2k` / `4k` |
| `amount_fen` | 整数分；失败行为 0 |
| `mediakit_job_id` | MediaKit 任务 ID |
| `status` | `succeeded` / `failed` |

## 业务规则

- 一行一次超分尝试。三次失败 = 三行 `failed`、金额 0。
- 成功金额：`round(分钟 × 官方元/分钟 × 100)`。例：5 秒、1080p、≤30fps → 42 分。
- **金额算得 0 的成功行**：回包时长缺失、或档位在价表里无价时金额为 0，适配器会打一条 `LogWarn`（「succeeded but was not priced」）避免静默。对账看到 0 的成功行应先查这条日志，不要直接当成免费。
- 金额不进 `logs.quota`、不进 `channels.used_quota` / 周期限额 / 硬上限。
- `tasks.private_data` 只留当前 `EnhanceJobID` 做关联，不存金额。
- 管理台任务日志：有任意一行即视为超分单（成功或失败都算）；用户任务列表不返回。

## 易错点

- 不要把超分花费折进用户扣费或渠道原始成本。
- 失败行必须落库（job id 可空，金额必须为 0）。

## 关联表

- `tasks`: 按 `task_id` 对账
- `channels`: 按 `channel_id` 汇总

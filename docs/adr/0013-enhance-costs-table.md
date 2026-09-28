# 超分花费落独立表 enhance_costs，不进用户 quota 也不进渠道限额

对账需要按时间/渠道/公开任务ID 汇总 MediaKit 人民币成本。任务 private_data 与消费日志 JSON 无法在三库上按「分」做 SUM。故新开 `enhance_costs`，一行一次超分；v1 不对用户展示、不做报表页。

**Status:** accepted

## 决策

表字段：公开任务ID、渠道ID、请求档、上游档、输出时长、分辨率档、金额（整数分）、MediaKit job id、状态、时间。`tasks.private_data` 只留 EnhanceJobID 关联。金额口径见词表「超分花费」。

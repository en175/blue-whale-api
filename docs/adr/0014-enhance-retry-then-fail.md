# 超分只重试 MediaKit，最多 3 次，耗尽则任务失败

超分失败不交付上游档成片。仅重试 MediaKit，不重跑 Seedance；固定 3 次提交（不可配）。三次皆失败则任务 FAILURE。enhance_costs 每次失败一行，金额 0。

**Status:** accepted

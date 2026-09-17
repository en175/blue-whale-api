# /api/log/self 的 type 收逗号分隔多值，非法值报错不降级

BFF 要把消耗(2)与退款(6)合成一条账单列表。两路各自分页再在客户端合并，第 N 页对齐需要「第 N 页之前各类各有多少条」，不存服务端状态算不准。故让 `/api/log/self` 的 `type` 收逗号分隔的整数列表，落到单条 `logs.type IN (...)`，一个游标一次分页。

`type` 缺省、空串、`0` 仍表示不过滤；`2,6` 取并集；` 2 , 6 ` 逐项去两侧空白；重复值不去重（`IN (2,2)` 与 `IN (2)` 等价）；数值域不校验——`type=999` 照旧过滤出空列表，与改动前一致。但任一项为空或非整数、或 `0` 与其他值同时出现，一律返回 `success:false`。这些输入若不拦，会落进「不过滤」分支静默返回**全部类型**，而调用方以为拿到的是「消耗+退款」。

`logs.quota` 是无符号量值：三个退款写入点（service 的 `RefundTaskQuota` 与 `RecalculateTaskQuota` 退款分支、controller 的 midjourney 任务回退）全记正数。合并列表里退款与消耗在 `quota` 列上无法区分，方向只写在 `type` 里——调用方必须按 `type` 定号，不能靠符号，也不能对混排结果直接求和当净额。

**Status:** accepted

## 决策

- 本期只改 `/api/log/self`。`GetAllLogs`、`GetLogsStat`、`GetLogsSelfStat` 不动：没有调用方需要多值，少一处就少一处与上游分叉。
- 解析收在 unexported 的 `controller.parseLogTypes`；错误走 `common.ApiError`，与本族其余端点一致的 HTTP 200 + `success:false` 信封，不为这一条引入真 400。
- 唯一行为变化：非法值从「静默 0 = 全部」变成失败。老调用方（new-api 自带前端、blue-whale-ai core 的三处 `type=2`）零影响。
- 模型层签名 `logType int` → `logTypes []int`，空列表不过滤；「0 = 不过滤」这个判断从 model 层上移到解析层（常量仍用 `model.LogTypeUnknown`，因为 0 就是它的值，但**语义由 `parseLogTypes` 解释**，model 不再做这个区分）。
- 表结构、索引、ClickHouse 分支、count / order 逻辑均未改动。

## 已知边界：分号与坏转义仍在解析器之前被丢掉

`?type=2;6`、`?type=1;2`、`?type=%zz` 会**静默返回全部类型**，不报错。原因不在本改动：Go 的 `url.ParseQuery` 遇到分号或非法百分号转义时把**整对键值丢弃**并返回 error，而 gin 的 `c.Query()` 吞掉该 error，`type` 于是看起来像「没传」，落进不过滤分支。

改动前这些输入走的是同一条路（`strconv.Atoi("")` 失败 → 0 → 全部），**行为与改动前完全一致**，故本次不修——修它要在 `GetUserLogs` 里重新解析一次查询串，会连带把 `?type=2&model_name=a;b` 从「静默忽略 model_name」变成报错，超出本单范围。真要在客户端传多个类型，用逗号。

因此工单验收里「`type=1;2` 返回 400」一条不成立：能到达 `parseLogTypes` 的非法输入（`abc`、`2,`、`,2`、`0,2`）都会失败，被 Go 提前丢掉的不经过它。


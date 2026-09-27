# 样片模式：两步独立计费，成片源只有网关能指定

Seedance 2.5 的样片模式是**两次独立计费的上游任务**（480p 样片 → 1080p 成片），不是一次请求的两种输出。网关必须把两步当成两条任务管理，并让第二步的成片源只由网关从原任务推导。

**Status:** accepted

## 背景

火山方舟 Seedance 2.5（`doubao-seedance-2-5-260628`）提供样片模式：先低价出 480p 预览验证镜头与节奏，确认后再基于该样片出 1080p 成片，**两步各自计费**。第二步的请求体是 `content:[{type:"draft_task", draft_task:{id}}]`，`id` 是第一步的上游任务 ID；Ark 规定第二步必须省略 `prompt` / `duration` / `ratio` / `seed` 等（自动沿用样片）。

接入前有两处已存在的缺口：

1. `doubao` 适配器的 `requestPayload` 已有 `Draft` 字段，`metadata.draft=true` 今天就能透到上游，**零把关**。在 64（火山增强）渠道上这会出事：1080p 抽签降到 480p → 判定为降档 → 轮询阶段对着一张样片跑 MediaKit 超分，真实花钱且交付物语义错乱。
2. 第二步完全不可用：`ContentItem` 没有 `draft_task` 字段，而 `taskcommon.UnmarshalMetadata` 走裸 `json.Unmarshal`（`relay/channel/task/taskcommon/helpers.go`）**静默丢弃**未知键 → 上游收到一个没有 id 的 `draft_task` 引用 → 报错，而预扣费已经发生。

## 决策

**1. 新开渠道类型 66 `VolcEngine25`，不碰 64。** 2.5 不需要超分——超分是 2.0 在降级抽奖后的补偿手段，对 2.5 无意义。因此不改 `volcengine_plus` 的任何行为，另建 `relay/channel/task/volcengine25/`，嵌入 `taskdoubao.TaskAdaptor` 只覆盖三个方法，内部没有抽奖、降级、超分、token 剥离。45 / 54 / 64 三个既有渠道行为逐位不变。

**2. 第二步走新路由 `POST /v1/videos/{公开任务ID}/finalize`。** 与已有 `/remix` 平级、共用 `middleware.Distributor` 的锁渠道路径。客户端传网关公开 ID，不接触上游 ID。

**3. `draft_task.id` 永远由网关从原任务推导，绝不接受客户端传入。** 成片请求体用白名单结构体构造，结构上不表达 `content` / `prompt` / `duration` / `ratio` / `seed`，客户端无论传什么都改不动成片内容与时长。

**4. 成片源必须是本人的、已成功的、7 天内的样片。** 校验失败在预扣费之前返回 400 / 409。上游 ID 明确取 `PrivateData.UpstreamTaskID`，**不用** `GetUpstreamTaskID()`——后者在 `PrivateData` 为空时回退到公开 ID `task_xxx`，把它当 `draft_task.id` 发出去上游必然报错。

**5. 步骤一记录「是否含视频输入」，步骤二继承这一维度做档位计费。** 成片请求体里没有 `content`，1080p 档单价取决于样片是否含视频输入，只能继承。为此在 `taskcommon` 拆出 `EstimateVideoTierBillingForTier(info, resolution, hasVideo)` 显式档位入口。

**6. 成片不继承样片的 `OtherRatios`。** remix 会整份复制原任务的 `OtherRatios`；样片的 `seconds` / `size` 会把成片的 token 结算放大 4~8 倍。finalize 只继承 hasVideo 这一个维度。

## 理由

- 渠道类型是运营可独立加权的单元。2.5 的对客契约（两步、无降级、无超分）与 64 不同，复用会把 2.5 拖进一条它不需要的超分链路里花钱。这与 ADR 0011 的结论一致：上游相同不是复用类型的理由。
- 上游 create 接口不是幂等的，而锁渠道路径没有 `specific_channel_id` 守卫。`shouldRetryTaskRelay` 对 `/finalize` 直接返回 false，避免运维调大重试次数后同一份样片出两段成片、扣两次费。

## 后果

- 样片与成片是两条独立的任务行，各自计费、各自可查。客户端通过任务 `properties.draft` 判断某条任务能否作为成片源。
- 管理员必须在 `video_tier_pricing.price_table` 为 `doubao-seedance-2-5-260628` 配齐 `base` / `1080p` / `1080p_video`：`base` 是 480p/无视频的基准价（等于 `ModelRatio`），缺 `1080p` / `1080p_video` 时成片会**按 480p 基准价结算**——不报错、不打日志。内置价格表没有 2.5 的单价，这是刻意的。
- 同一份样片调两次 finalize 会出两段成片、扣两次费。没有幂等键，靠禁重试 + 文档覆盖；彻底解决需要在原任务上 CAS 写 `PrivateData.FinalizedBy`，另开工单。
- 上线前创建的样片任务 `Properties.Draft=false`，不可 finalize。
- `data.id` 暴露上游 `cgt-` ID 是所有 doubao 任务的既有行为（`PublicData()` 在 `EnhanceStatus == ""` 时原样返回 `task.Data`），本次不加固：无 API key 该 ID 无用，网关始终从用户自己的任务推导。

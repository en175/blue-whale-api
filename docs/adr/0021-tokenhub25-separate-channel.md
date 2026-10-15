# 华鲲 Seedance 2.5 单开渠道类型 67，不扩 61

国际版 Seedance 2.5 的上游是华鲲（ai-tokenhub），与已经承载 61（`TokenHub`）走**同一套协议、同一个端点、同一把凭据**。即便如此仍新开渠道类型 67（`TokenHub25`），而不是把 2.5 的模型加进 61 再补两个分支。

**Status:** accepted

## 背景

需求 IKL2I0 要求把国内已完成的样片模式（`draft` 480p → `finalize` 1080p，ADR 0019）搬到国际版。国内那套建在渠道 66（`VolcEngine25`）上，上游是火山方舟；国际版的上游是华鲲。

两个既有渠道都不能直接用：

- **66** 的样片契约完整，但适配器嵌的是 `taskdoubao`，读的是 Ark `/api/v3` 的**顶层** `content.video_url` / `status`；
- **61** 走华鲲 `/api/v2`，结果包在 `resultSummary` 信封里，但没有样片能力。

需求的原始处置是「新建渠道类型 66 渠道、上游指向华鲲」（IKL2I0 §实施要点 2）。这条走不通：把 66 的 base_url 改到 `ai-tokenhub.ai` 之后，路径仍是 `/api/v3`、响应仍按顶层字段解析，而华鲲的 `/api/v3` 任务端点并不存在（实测 404 与 401 可以区分出：`/api/v2/...` 返回 401、`/api/v3/...` 返回 404）。

于是真正的选项只有两个：**扩 61**，还是**新开 67**。

## 决策

**新开渠道类型 67 `TokenHub25`，嵌入 `tasktokenhub.TaskAdaptor`，只覆盖五个方法。61 一行不改。**

覆盖的五个方法是 `GetChannelName`、`GetModelList`、`ValidateRequestAndSetAction`、`BuildRequestBody`、`EstimateBilling`；2.0 的三条模型（`dreamina-seedance-2-0-*`）留在 61，67 的模型列表只有 `dreamina-seedance-2-5-260628`。

## 理由

按 ADR 0011 立的规矩：**上游相同不是复用渠道类型的理由**，渠道是运营可独立加权的单元。但这次的理由不止于此，还多两条更硬的：

1. **扩 61 会让 61 的每一条既有路径都带上「是不是样片」的分支。** `ValidateRequestAndSetAction` 要加 finalize 短路（且必须放在最前，因为 `storeTaskRequest` 会把 `info.Action` 覆盖成 `generate`）、`BuildRequestBody` 要加 draft 改写、`EstimateBilling` 要加 1080p 档位。而 61 目前是**停用**状态，只有 3 条历史成功任务——为它注入三条新分支、再把它们暴露给所有 2.0 流量，收益与风险不成比例。

2. **类型号落库，难回头。** `channels.type` 是持久化的。生产上一旦建了 67 的渠道、跑过任务，回并进 61 就意味着数据迁移 + 任务归属重算。反过来说，如果先扩 61、事后发现要拆，代价一样大——但那时 2.0 与 2.5 的流量已经混在同一组渠道限额与模型映射里了。

3. **67 可以独立配限额、独立开关、独立灰度。** 2.5 样片模式上线初期需要按渠道观察成片成功率与扣费口径，混在 61 里做不到。

代价是承认一份重复：`relay/channel/task/tokenhub25/draft.go` 与 `volcengine25/draft.go` 内容几乎相同。这是**有意选择**的重复——`finalizePayload` 硬绑具体 ContentItem 类型、`buildFinalizePayload` 硬绑 `DraftTaskRef`，抽公共层需要泛型或接口，去不掉重复；而抽公共层必然改动 `volcengine25/`，与本仓「渠道包各自持有协议」的既有做法不符。选择复制而非抽象，是为了让 66 的行为在本次改动后**逐位不变**。

## 后果

- **61 的入站与出站行为逐位不变。** 成片的 content 项在 `tokenhub25` 包内本地定义为 `finalizeContentItem`，不碰 61 的 `ContentItem`——因此 61 既不会多出 `draft_task` 出站字段，也不会新增 `draft_task` 入站透传。
- **67 与 66 的对客契约一致，上游协议不同。** 校验、落库、计费链路全部复用渠道无关的既有实现（`ResolveOriginTask` / `InitTask` / `taskcommon` 档位估算），本渠道不新增链路代码。
- **任务类型校验不在本次范围。** 需求描述的「按 `content.role` + 提示词关键词判定五种任务类型、并对 `ratio` / `duration` 施加特殊限制」一律不校验。理由是那套判定规则会变；违反时上游自己会报错。`metadata.omni_reference_task_type` 只做**转发**（照 66），不做取值校验。
- **Remix 不特殊处理**，与 66 一致：静默降级为普通 generate。要改成「响铃拒绝」应开独立工单把 61 / 66 / 67 一起收口。
- **价格表由管理员配。** `video_tier_pricing.price_table` 里为 `dreamina-seedance-2-5-260628` 配 `base` / `video` / `1080p` / `1080p_video` 四个键；内置价格表不加 2.5 的单价（与 66 口径一致）。漏配 `1080p` / `1080p_video` 会静默按 `base` 结算。
- **上游契约已部分实测。** 2026-10-14 用真华鲲 key 走完整链路打了一发样片，四项里前两项确认：① 华鲲 2.5 的轮询响应与 `tokenhub.responseTask` 逐字段吻合（`resultSummary` 信封成立，「嵌 tokenhub 而非 doubao」的决策因此落地）；② `draft=true` + `resolution=480p` **确实出 480p**——上游自报 `resultSummary.resolution = "480p"`，成片 `ffprobe` 实测 854×480。原「唯一会静默少收」的风险已证伪。剩下两项（finalize 的 `draft_task` 引用、成片白名单字段）仍未验证，都只在出成片时触发且失败会响，**上线前必须测掉**。详见 `docs/channel/tokenhub-25.md` §13。

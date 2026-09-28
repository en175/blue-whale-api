# 新开 MediaKit 渠道（type 65）统管全部工具，透传上游协议

火山 AI MediaKit（`mediakit.cn-beijing.volces.com`）是独立协议 + 独立凭证的智能多媒体工具集：`Authorization: Bearer <apiKey>` 单头鉴权，图像工具同步返回（`/api/v1/tools-sync/*`），音视频工具异步（`/api/v1/tools/*` 提交 → `GET /api/v1/tasks/{id}` 轮询）。与已有渠道 45（方舟）、62（seed-audio）、63（HighwayAPI）协议各不相同，故不挂靠，新开独立渠道类型。

对客路由 `/v1/mediakit/{tool}`，**透传上游协议**：客户端按 MediaKit 原生请求体编写，网关只做鉴权、锁模型与计费。

**Status:** accepted

## 背景

master 上 MediaKit **已经接了一个工具**——「画质增强（大模型版）」`enhance-video-generative`，用途是 Seedance 分辨率抽签降档后的超分回档（ADR-0015）。本次是把其余工具接进来，属于**扩展现有集成**而非从零接入。

「全部工具」的口径以官方文档的**工具表**为准，共 **64 条**：视频 33、图像 27（14 + 图像基础编辑 13 子工具）、音频 4。计划阶段的「46 个」来自文档三个标题的相加（28 + 14 + 4），标题数与实际表行数不一致，以表为准。注册表**在这三个集合上一一对应，无遗漏**。

> 后续另接入 5 条**集合外**的工具：`extract-audio`（视频抽音轨）、`trim-audio`（音频截取）、`mix-audio`（音频混合）、`mux-audio-video`（音视频合成）、`adjust-audio-speed`（音频调速），故注册表总数为 69。它们不属于上述三个集合，是分别应下游「音频提取」「音频截取」「音频混合」「音视频合成」「音频调速」需求额外接入的，见文末「后续」。

接入时面临的形态选择：

1. **按模态挂到已有渠道**（视频→火山增强/方舟，图像→方舟/jiekouAI，音频→DoubaoAudio）。被否：MediaKit 是独立协议与独立凭证，挂靠任何一条已有渠道都会让同一渠道绑两套协议，任一方接口变动都牵连另一方。违背 ADR-0003 的渠道边界原则（边界由上游协议与端点决定）。
2. **一个工具一个渠道类型**。被否：69 个渠道类型会挤爆渠道枚举，且它们的协议、鉴权、计费框架完全相同，只有路径与费率不同——那是注册表的职责，不是渠道的职责。

## 决策

### 渠道：一个类型 65，同时挂两张适配器表

`relay/relay_adaptor.go` 有两张互不干扰的表：`GetAdaptor(apiType)`（同步）与 `GetTaskAdaptor(platform)`（异步）。同一渠道类型同时挂两张表**有先例**（45、51、24），谁被调用只由路由决定。故**不需要两个渠道类型**：同步工具走普通 `Adaptor`（`controller.Relay` 管道），异步工具走 `TaskAdaptor`（`controller.RelayTask` 管道）。

### 工具：69 条注册表，路径只来自服务端

`relay/channel/mediakit/registry.go` 是唯一事实来源，每条记录 `{Name, Path, Sync, Mode, ResultKind}`。客户端传的 `:tool` **只用于 `LookupTool` 查表**，查不到一律 400；上游 URL 由表项写死的 `Path` 拼成，绝不接受客户端字符串参与拼接——否则 `base + "/api/v1/tools/" + 客户端输入` 是明显的路径穿越面。

登记在表的工具才会出现在渠道模型列表（对客模型名 `mediakit-<tool>`），管理员按需勾选，实现分批放量。

### 请求：完全透传

MediaKit 原生请求体里没有 `model` 字段，逐字段映射既引维护成本又容易在客户端需要新参数时漏掉。网关一个字都不改写，原样转发（`common.GetBodyStorage` → `storage.Bytes()`）。计费靠**路径上的工具名 → 模型名 → 管理员配的 `model_price`**，不靠请求体。

### 计费：混合口径，失败不计费

官方计费分三档，对客按同一维度归口：

| 模式 | 覆盖 | 对客口径 |
|---|---|---|
| 按时长 | 视频 32 + 音频 7 | 管理员按元/分钟配 `model_price`；提交预扣，完成用上游回包的真实时长结算差额 |
| 按次 | 同步图像工具 14 + 视频抽帧 `extract-frames` + `probe-video-metadata` + `probe-audio-metadata` | 每次固定价 |
| 按量 GiB | 图像基础编辑 13 个子工具 | 直接按超出档单价 0.025 元/GiB，**不套官方「前 10 TiB 免费 + 自然月阶梯」** |

注册表共 69 条：同步 27（14 按次 + 13 按量，全部是图像工具）、异步 42（34 视频 + 8 音频，音视频两侧共含集合外的 5 条）。

GiB 档不套阶梯是为了让三类计费量**全部逐请求可算**，网关无需维护账号级月度累计表。

官方「任务处理失败不触发计费」与现有 `RefundTaskQuota` 语义天然对齐：异步任务失败全额退预扣。

### 差额结算需要显式 opt-in（解法 f）

`service/task_polling.go` 的差额结算第一句是「按次计费直接返回」，而按次计费的判定条件之一是「命中 `model_price`」。按时长计费的渠道恰恰也要把价格配在 `model_price` 上（元/分钟），于是会被误判为按次、整段跳过结算——`AdjustBillingOnComplete` 永远不生效，用户按预估时长被扣，真实时长从不参与计价。

解法是给差额结算加显式 opt-in：`TaskBillingContext.DurationSettlement`，由适配器在 `EstimateBilling` 里对时长类工具置位，判定条件改为 `PerCallBilling && !DurationSettlement`。纯追加，现有渠道行为逐字不变。

### 审计表 `mediakit_costs` 只记网关确知的数

对账需要按工具 / 渠道 / 时段汇总。但官方费率是「工具 × 输出档（按短边）× 帧率档」的矩阵，三种计费模式量纲还不同，尚未逐工具核实。把猜测的费率硬编进代码会把错误固化进已迁移的库；把用户实扣换算成人民币填进 `amount_fen` 更糟——名字说成本、内容是收费，对账时加价策略会被读成算错。

故该表只记归属与实扣（`tool` / `billing_mode` / `upstream_job_id` / `status` / `estimated_minutes` / `actual_seconds` / `charged_quota`），成本交给对账方用「`actual_seconds` × 官方费率」在 SQL 外算。详见 `docs/db-schema/docs/mediakit_costs.md`。

## 理由

**为什么透传而不是 OpenAI 兼容转换**：69 个工具的入参面宽且几乎不重叠（`video_url` / `image_url` / `audio_url` / 各种档位参数），逐工具写 DTO 是 69 套映射的维护成本，且上游加参数就要发版。透传让上游协议成为客户端契约，网关只负责鉴权、锁模型与计费——与 ADR-0004（DoubaoAudio）、ADR-0005（素材库）、ADR-0008（HighwayAPI）同构。MediaKit 比它们更纯：body 里没有 `model` 字段，一个字都不用改。

**为什么查询走本库而不代理上游**：客户端持有的是网关公开任务 ID（`task_xxx`），直连上游 `/api/v1/tasks/{id}` 既暴露上游任务号，也允许越权枚举他人任务。查询落本库 `tasks` 表（照抄 `videoFetchByIDRespBodyBuilder`），渠道由任务记录决定，不重新选渠道。

**为什么价表不内置**：媒体工具的档位矩阵比超分（单工具、720p/1080p × 帧率）宽一个数量级，内置进用户计费会让改价必须发版。`volcengine_plus/cost.go` 的超分价表**不动**——它有 ADR-0015 明确「价表内置」的定位。

## 影响

- 新增渠道类型 65 / `APITypeMediaKit`，注册进渠道常量、`ChannelBaseURLs`、`ChannelTypeNames`、`ChannelType2APIType` 映射与两张适配器表；渠道测试标记为不支持。
- 对客路由三组：`POST /v1/mediakit/sync/{tool}`（同步）、`POST /v1/mediakit/{tool}`（异步提交）、`GET /v1/mediakit/tasks/{task_id}`（查询）。
- 新增 `types.RelayFormatMediaKit`、`RelayModeMediaKitSync/Async/Fetch`、`dto.ChannelSettings.MediaKitDefaultMinutes`（时长类工具缺声明时长时的预估分钟，校验 0–60），以及 `ValidateMediaKit()`。
- **MediaKit key 走渠道 `key` 字段**而非 `setting.mediakit_api_key`：单头 `Authorization: Bearer` 天然适配渠道 key，且轮询层 `FetchTask(baseUrl, key, ...)` 拿到的正是渠道 key，走 setting 会让轮询拿不到。
- `service/task_polling.go` 新增可选接口 `TaskCostRecorder`，并调整差额结算的判定条件。未实现该接口的渠道行为不变。
- 与已有渠道 45 / 62 / 63 **并存互不干扰**；type 64 的超分链路（含 `enhance.go` 的上游客户端）保持公开符号与签名不变，超分零回归。

## 后续

1. **官方价表 `relay/channel/mediakit/cost.go`**：逐工具录入元/分钟、元/千次、元/GiB 及档位系数，供 (a) 审计表补 `upstream_amount_fen`、(b) 给管理员提供默认定价参考。前置是「各工具计费模式归类」逐条核实完成。
2. **各工具的结果键名与时长字段名**：结果位置已由注册表逐工具声明（`ResultPath` 或 `ResultKind` 约定键），取不到即空串；早期「兜底扫 `*_url` 按键名排序取第一个」的写法已删除，理由见 `registry.go` `ResultURL` 注释。时长字段靠 `TaskDurationSeconds` 未命中时打出的 `result 键：…` 日志逐工具收敛。**已实测**：`enhance-video-generative` 的真实回包为 `result.duration`（秒，如 `14.095`），候选表已命中，无需新增 `DurationResultField`；其余工具仍待实测。

   失败原因同理：上游把详情放在顶层 `error` 对象（`{code, type, message}`），**顶层 `message` 通常为空**，且 `success` 与 `status: "failed"` 可同时为真——只能认 `status`，原因走 `failureReason()`。已实测。
3. **同步工具批量（`batch_count > 1`）**：v1 一律 400。实现在 `relay/helper/valid_request.go` 的 `GetAndValidateRequest` → `RelayFormatMediaKit` 分支，**不是 `MediaKitSyncHelper`**——必须排在 `controller.Relay` 的定价与预扣之前。透传路径没有 DTO，拿不到 `TokenCountMeta.BillingRatios` 钩子参与计价；要做批量得给 `TokenCountMeta` 加「透传 body 预扫描」。注意倍率字段是 `batch_count` 而非 `n`——MediaKit 无工具使用 `n`。
   异步侧**无同类守卫，且确实构成少收面**——本文早期版本断言「异步唯一的按次工具 `extract-frames` 没有帧数/张数入参，产出规模不可由客户端线性放大」，**该断言是错的**。实测异步侧有三个多产物字段：`drama-recap.batch_count`、`drama-recap-vertical.max_count`（范围 [1, 100]，**默认值 3**——不传也会产出多条）。异步走 `controller.RelayTask`、不经过 `GetAndValidateRequest`（该函数全仓只有 `controller/relay.go:111` 一个调用点，在 `Relay()` 内），所以同步侧那道守卫不覆盖异步。上游会按倍数真的产出，网关只按一次计费。待补：在 `TaskAdaptor.ValidateRequestAndSetAction`（`task_adaptor.go:54`）里加 `> 1` 拒绝；未做是因为它会拒掉现有多产物调用，属对客行为变更。
   另有一处**无风险的口径差**：`extract-frames` 官方按**截图总张数**计费（0.1 元/千次），网关按次收固定价——它的入参里确实没有帧数/张数字段，产出规模由视频内容决定，客户端无法放大，所以只是计价维度不同、不构成少收面。
4. **确认生产 `TaskPricePatches` 是否含 MediaKit 工具名**：命中会强制 `PerCallBilling=true`，把时长类工具拖进不结算分支（`DurationSettlement` 已可放行，但仍应确认）。
5. **未登记的工具**：三个集合下的工具**已全部登记**，无遗留。集合**之外**另接入了 5 条，它们不属于上述三个集合，上游各有独立 slug 与独立产物，故按同一适配器接入而非另行开洞。按证据强度分两档——**这个区分要紧**：本仓已踩过「官方文档写的 slug 是错的」（文档写 `scale-image`，真 slug 是 `resize-image`，只有实测才发现），所以文档来源的条目在实测前不能当已知用。

   **已实测核对**：

   - `extract-audio`（视频抽音轨，供下游「音频提取」）：`/api/v1/tools/extract-audio`，产物 `result.audio_url` + `duration`。
   - `trim-audio`（音频截取，供下游「音频截取」）：`/api/v1/tools/trim-audio`，入参 `audio_url` + `start_time` / `end_time`（秒），产物同为 `result.audio_url` + `duration`——注意这里的 `duration` 是**裁剪后片段**的时长，不是源文件时长。

   **取自官方开发指南、尚待实测**（IKIT9I）：

   - `mix-audio`（音频混合，供下游「音频混合」）：`/api/v1/tools/mix-audio`，入参 `audio_urls` 数组（1–100 个），产物 `result.audio_url` + `duration`（秒）。输出时长由**最长输入**决定。
   - `mux-audio-video`（音视频合成，供下游「音视频合成」）：`/api/v1/tools/mux-audio-video`，入参 `video_url` + `audio_url`（可选 `is_audio_reserve` / `is_video_audio_sync` / `sync_mode` / `sync_method`），产物 `result.video_url` + `resolution` + `duration`（秒）。⚠️ **计费口径与本注册表声明不一致**：官方按「输出时长 × 分辨率换算系数（4K 24 / 2K 12 / 1080P 6 / 720P 3 / 540P 2 / 480P 1.5 / 360P 含纯音频 1）× 单价」计费，系数取**输出**分辨率；而注册表只声明 `BillingDuration`（平坦元/分钟），结算倍率的现有结构里没有承载「响应派生倍率」的位置。本期零价两种口径都得 0，收费前须决定（详见 `docs/channel/mediakit.md` §6.5）。已开工单 **IKITTA** 跟踪。
   - `adjust-audio-speed`（音频调速，供下游「音频调速」）：`/api/v1/tools/adjust-audio-speed`，入参 `audio_url` + `speed`（> 1 快放、< 1 慢放），产物 `result.audio_url` + `duration`（**非**视频调速的 `video_url`；两者流程一致，只差路径与这个键名）。输出为纯音频，落在系数表最低档 `1`。

   三者的 `duration` 一律以**秒**为单位。slug 是否真实存在由真机兜底：slug 错时上游回 NotFound，与「缺字段」的 400 可区分。

   本文早期版本写「`get-audio-meta`（音频元信息获取）刻意未登记——官方也不按处理量计费」，**双重错误**：真实 slug 是 `probe-audio-metadata`（`get-audio-meta` 不存在），且官方**确实计费**——视频/音频元信息按成功处理次数 0.1 元/千次，图像元信息归在「图像基础编辑」计费单元（0.025 元/GiB，自然月 10 TiB 免费额度后累进）。三者现均已登记并按官方口径定价：视频/音频 0.0001 元/次；图像因网关不测体积，按单图 10 MiB 上限折成 0.00024 元/次近似。同类的 `video-understand-router` / `chat` / 大模型高光剪辑 / 剪辑套件（拼接、调速、滤镜等约 19 个）仍未登记：前者依赖方舟按 token 计费（MediaKit 侧只按时长收），后者官方未给出逐工具的独立 slug 表。

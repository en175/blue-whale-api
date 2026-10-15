# 华鲲 Seedance 2.5 渠道（样片模式）

Seedance 2.5 在国际版上游（**华鲲 = ai-tokenhub.ai**）上的专用渠道，承载样片模式的两步流程。它与国内渠道 66（`VolcEngine25`）的对客契约逐条一致，差别只在上游协议：华鲲走 `/api/v2`、结果包在 `resultSummary` 信封里。契约以词表和 ADR 为准，本文是渠道说明书。

- **术语：** [`CONTEXT.md`](../../CONTEXT.md)
- **ADR：** [0019](../adr/0019-seedance-draft-mode.md) 样片模式：两步独立计费，成片源只有网关能指定 · [0021](../adr/0021-tokenhub25-separate-channel.md) 为何单开类型 67 而不是扩 61
- **上游：** 华鲲 ai-tokenhub `POST /api/v2/contents/generations/tasks`（模型 `dreamina-seedance-2-5-260628`）
- **姊妹渠道：** 66 `VolcEngine25`（同一套两步契约，上游是火山方舟 `/api/v3`）

---

## 1. 需求

新增渠道类型 **TokenHub25**（`ChannelTypeTokenHub25 = 67`）。对客接口与其它视频渠道相同（`/v1/videos`），在此之上支持样片模式：

1. **样片（draft）**：`metadata.draft=true`（或顶层 `draft`）提交，**只允许 480p**，拿到低价预览。
2. **成片（finalize）**：`POST /v1/videos/{公开任务ID}/finalize` 基于一份已完成的样片出 1080p 成片。
3. **两步独立计费**：样片与成片各是一条任务行、各自预扣与结算。

不接：任务类型校验、Remix、2.0 系列模型（留在 61）。

---

## 2. 术语

以 `CONTEXT.md` 为准，本渠道用到的：

| 术语 | 含义 |
|---|---|
| 样片（draft） | 第一步。强制 480p 的低价预览，独立计费、独立任务行。 |
| 成片（finalize） | 第二步。基于样片的 1080p 输出，独立计费、独立任务行。 |
| 样片源 | 成片所引用的样片任务。必须是**本人的、SUCCESS 的、7 天内**的 draft 任务（窗口从**提交时刻**起算）。 |
| 上游样片 ID | 样片在华鲲方的任务 ID，存 `private_data.upstream_task_id`，只由网关读取。 |
| 含视频输入 | 计费用语：样片请求 `metadata.content` 里出现 `video_url`。成片的 1080p 档单价取它。 |

---

## 3. 主链路

```
第一步  用户 POST /v1/videos
        { model:"dreamina-seedance-2-5-260628", prompt, metadata:{ draft:true } }
          └─ 校验：draft 只允许 480p，其它分辨率 400（预扣费之前）
          └─ payload 强制 draft=true + resolution=480p
          └─ 落库：properties.draft=true
                   private_data.has_video_input=<本次是否含视频输入>
          └─ 轮询到 SUCCESS → 480p 预览

第二步  用户 POST /v1/videos/{公开任务ID}/finalize   请求体可为空
          └─ 归属校验：必须是本人的任务
          └─ 样片校验：properties.draft 为真 / SUCCESS / 未超 7 天 / 有上游 ID
          └─ 锁定原任务渠道；模型名沿用原任务
          └─ payload 白名单：只发 model + content[0].draft_task.id + 输出规格参数
             （draft_task.id = 原任务的 private_data.upstream_task_id）
          └─ 轮询到 SUCCESS → 1080p 成片
```

两步的校验、落库、计费链路全部复用渠道无关的既有实现（`relay/relay_task.go` 的 `ResolveOriginTask`、`model/task.go` 的 `InitTask`、`taskcommon/video_tier.go`），**本渠道不新增任何链路代码**。

### 3.1 任务类型引导（`omni_reference_task_type`）

客户端可在 `metadata.omni_reference_task_type` 显式引导任务类型，取值 `auto` / `reference` / `edit` / `extend`。

- 该字段不在 `tokenhub` 包的共用 `requestPayload` 白名单里，由本渠道在 `BuildRequestBody` 补写；客户端没传时不改写 payload。
- 非字符串 → 400 `invalid_omni_reference_task_type`。此前该字段被静默丢弃，客户端无从得知自己的任务类型意图没有生效。
- **取值本身不校验**，交给上游判定。**本渠道也不校验各个任务类型各自的参数约束**（`ratio` 必须 `adaptive`、`duration` 必须 `-1`、`content` 至少要几个 `reference_video` 等）——那套规则会变，且违反时上游自己会报错。详见 §12 与 §13。

---

## 4. 样片（第一步）

### 4.1 触发

两条入口等价，任一为真即为样片：顶层 `draft`（`true` 或 `"true"`）与 `metadata.draft`（`true` 或 `"true"`，大小写不敏感）。

### 4.2 分辨率守卫

| 请求 | 结果 |
|---|---|
| draft 且未传 `resolution` | 通过，上游收到 `480p` |
| draft 且 `resolution=480p` | 通过 |
| draft 且 `resolution` 为 `720p` / `1080p` / `4k` | **400 `invalid_resolution`** |

明确拒绝而不是静默改写成 480p：静默改写会让客户端以为自己拿到了 1080p。拒绝发生在预扣费之前。

### 4.3 落库

| 字段 | 值 | 可见性 |
|---|---|---|
| `properties.draft` | `true` | 公开，客户端据此判断该任务能否 finalize |
| `private_data.has_video_input` | 本次请求是否含视频输入 | 内部（`private_data` 是 `json:"-"`） |

---

## 5. 成片（第二步）

### 5.1 入口

`POST /v1/videos/{公开任务ID}/finalize`。请求体可为空、可为 `{}`，也可带输出规格参数。

渠道被锁定为原任务的渠道，模型名沿用原任务——客户端不传 `model`。

### 5.2 样片源校验（全部在预扣费之前）

| 条件 | 失败响应 |
|---|---|
| 任务是本人的 | 400 `task_not_exist` |
| `properties.draft == true` | 400 `origin_not_draft` |
| `status == SUCCESS` | 409 `origin_task_not_finished` |
| 提交时刻在 7 天内 | 400 `origin_draft_expired` |
| 原任务模型名可推导 | 400 `origin_model_name_missing` |
| `private_data.upstream_task_id` 非空 | 400 `origin_upstream_id_missing` |

校验由 `relay/relay_task.go` 的 `ResolveOriginTask` 完成，**与 66 共用同一份实现**。

### 5.3 请求体白名单

客户端可传（全部可选）：`resolution`（只允许 `1080p`，否则 400）、`return_last_frame`、`output_format`、`watermark`、`service_tier`、`execution_expires_after`、`priority`、`callback_url`、`safety_identifier`。

客户端**不能**影响：`model`、`content`、`prompt`、`duration`、`ratio`、`seed`、`generate_audio`、`draft`、`omni_reference_task_type`——这些字段在上游 payload 的结构体上根本不存在，客户端无论传什么都进不去。`draft_task.id` 只来自原任务的 `private_data.upstream_task_id`。

### 5.4 不继承样片的 OtherRatios

remix 会整份复制原任务的 `OtherRatios`；成片**不能**——样片的 `seconds` / `size` 会把成片的 token 结算放大 4~8 倍。成片只继承 `has_video_input` 这一个维度。

---

## 6. 计费

样片与成片各扣各的，互不重叠。

| 步骤 | 档位 | 说明 |
|---|---|---|
| 样片 | 480p（`video_input` 取本次请求） | 上游强制 480p，与请求档一致 |
| 成片 | **1080p**（`video_input` 继承样片） | 请求体里没有 content，输入维度只能继承 |

档位倍率来自 `video_tier_pricing.price_table`，按上游模型名查、回退对客名。倍率 = 档位价 ÷ `base`。

> ⚠️ **管理员必须配齐。** 内置价格表**不含** 2.5 的单价（刻意为之，见 ADR 0021）。需要为 `dreamina-seedance-2-5-260628` 配四个键：
> - `base` = 480p / 不含视频输入的基准价
> - `video` = 480p / 含视频输入
> - `1080p` = 1080p / 不含视频输入
> - `1080p_video` = 1080p / 含视频输入
>
> **直接照抄 `doubao-seedance-2-5-260628` 那一行**（同一模型系列，档位结构相同）：
>
> ```json
> {"base": 70, "video": 42, "1080p": 77, "1080p_video": 46}
> ```
>
> ⚠️ **`base` 不是该模型的 `ModelRatio`，而是它的 2 倍。** 四个键与 `base` 必须是**同一尺度**的绝对价，倍率 = 档位价 ÷ `base`。本仓生产实测：**所有已配模型的 `base` 都恰好等于 2 × `ModelRatio`**（`doubao-seedance-2-5-260628` = 70 vs 35、`doubao-seedance-2-0-260128` = 46 vs 23、`-fast` = 37 vs 18.5、`-mini` = 23 vs 11.5），生产消费日志里 2.5 的 `video_input` 倍率实测为 **0.60**（= 42/70）、`finalizeDraft` 为 **1.10**（= 77/70），与这套数一致。
>
> 若照 `setting/billing_setting/video_tier_pricing.go` 里「`base` 应等于 `ModelRatio`」那句注释配成 `base=35`，却把档位价照抄成 77，倍率会变成 77/35 = **2.2**，1080p 直接多收一倍。`base` 与档位价是等比缩放关系（同乘同除不改变倍率），**要么整行照抄，要么整行按同一比例缩放**，不能只改一个。
>
> 缺 `1080p` / `1080p_video` 时，成片会**按 480p 基准价结算**——不报错、不打日志、静默少收。`4k*` 对 2.5 无意义（2.5 没有 4k 档）。

> ⚠️ **480p 与 720p 共用同一个价格槽。** `videoTierRatio` 只区分 `1080p` 与 `4k`，其余分辨率一律落到 `base` / `video`。因此样片（480p）与非样片的 720p 请求同价，这一点改不了。

结算链路不变：按 `ModelRatio` 配置时 `PerCallBilling=false`，任务完成时走 `RecalculateTaskQuotaByTokens`，用落库的 `OtherRatios` 乘积作 multiplier。

---

## 7. 防重复扣费

`shouldRetryTaskRelay` 对 `/finalize` 直接返回 false（渠道无关）。上游 create 接口不是幂等的，而锁渠道路径没有 `specific_channel_id`，`controller/relay.go` 的守卫拦不住。重试次数默认 0 所以今天不会触发，但运维一旦调大，超时 / 5xx 重试会让同一份样片出两段成片、扣两次费。

同一份样片调两次 finalize **仍会**出两段成片、扣两次费——没有幂等键。彻底解决需在原任务上 CAS 写 `PrivateData.FinalizedBy`，另开工单。

---

## 8. 渠道注册

- `ChannelTypeTokenHub25 = 67`，插在 `ChannelTypeDummy` **之前**。
- `ChannelBaseURLs[67] = "https://ai-tokenhub.ai"`（下表寻址，必须补齐，否则越界 panic）。**只配主机**，`/api/v2` 前缀由 `BuildRequestURL` 拼。
- `ChannelTypeNames[67] = "TokenHub25"`。
- `GetTaskAdaptor`：新类型返回 `tasktokenhub25.TaskAdaptor`。
- `controller/channel-test.go` 的 `unsupportedTestChannelTypes` 含 67（视频任务渠道不做联通性测试）。
- **无渠道 `setting`**：本类型不读任何渠道配置。
- 与 61 完全独立：61 的模型列表、payload、行为逐位不变，61 上加不了样片能力。

前端：`CHANNEL_TYPES` / 展示序 / 类型配置加 67；渠道图标复用 `tokenhub`。

---

## 9. 适配器

新建包 `relay/channel/task/tokenhub25/`：

```go
type TaskAdaptor struct {
    tasktokenhub.TaskAdaptor
}
```

只覆盖五个方法，其余（`BuildRequestURL` / `BuildRequestHeader` / `DoRequest` / `DoResponse` / `FetchTask` / `ParseTaskResult` / `ConvertToOpenAIVideo` / `ExtractTaskUsage`）全部继承 tokenhub：

| 方法 | 行为 |
|---|---|
| `GetChannelName` / `GetModelList` | 返回本渠道的名字与模型列表 |
| `ValidateRequestAndSetAction` | finalize 走白名单校验并**提前返回**；否则委托 tokenhub 后补 draft 的 480p 守卫、`omni_reference_task_type` 类型守卫与 `has_video_input` 采集 |
| `BuildRequestBody` | finalize 构造白名单 payload；其余委托 tokenhub 后补写 `omni_reference_task_type`，若为样片再强写 `draft=true` + `resolution=480p` |
| `EstimateBilling` | finalize 用 `EstimateVideoTierBillingForTier(info, 1080p, originHasVideoInput)`；其余委托 tokenhub |

### 为什么嵌 `tokenhub` 而不是 `doubao`（66 的那条路）

必须嵌 tokenhub。华鲲把结果包在 `resultSummary` 信封里（`resultSummary.content.video_url` + `resultSummary.usage`），并靠 `effectiveStatus()` / `terminalFailureReason()` 处理 cancelled / expired 与**字符串形态**的 error code；而 `doubao` 读的是**顶层** `content.video_url` / `status`。嵌 doubao 会让 `status` 永远解析为空 → 任务卡在 `IN_PROGRESS` 直到 24h 超时扫尾，`video_url` 也永远取不到。

### 为什么 finalize 短路必须在委托之前

tokenhub 的 `ValidateRequestAndSetAction` 写死调用 `ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)`，而它内部的 `storeTaskRequest` 会 `info.Action = "generate"`——把 `ResolveOriginTask` 设的 `finalizeDraft` **覆盖掉**。成片请求体里也没有 `prompt`，走 tokenhub 的 `validatePrompt` 必然 400。两条都要求在委托之前短路。66 上有同一个坑（那里覆盖 action 的是 doubao 的 `inferDoubaoVideoAction`）。

### `draft.go` 为什么复制而不抽公共层

`finalizePayload` 硬绑具体 ContentItem 类型、`buildFinalizePayload` 硬绑 `DraftTaskRef`，抽出来要泛型或接口，去不掉重复；且抽公共层必然改动 `volcengine25/`，与本仓「渠道包各自持有协议」的既有做法（tokenhub 本就逐字复刻 doubao 的 `requestPayload` / `ParseTaskResult`）不符。

成片的 content 项在本包**本地定义**（`finalizeContentItem` / `finalizeDraftTaskRef`），不动 61 的 `ContentItem`——61 的入站与出站行为因此逐位不变。

---

## 10. 实现落点

| 动作 | 路径 |
|---|---|
| 新适配器 | `relay/channel/task/tokenhub25/`（`adaptor.go` / `constants.go` / `draft.go`） |
| 渠道常量与注册 | `constant/channel.go`、`relay/relay_adaptor.go` |
| 测试白名单 | `controller/channel-test.go` |
| 前端 | `web/default/src/features/channels/`、`web/default/src/i18n/locales/*.json` |
| 决策 | `docs/adr/0021-tokenhub25-separate-channel.md` |

复用、不改动：`relay/relay_task.go` `ResolveOriginTask`、`middleware/distributor.go`、`router/video-router.go`、`model/task.go`、`relay/channel/task/taskcommon/`、`setting/billing_setting/video_tier_pricing.go`。

**无需 DB 迁移**：`Properties.Draft` / `PrivateData.HasVideoInput` / `PrivateData.UpstreamTaskID` 由 ADR 0019 引入。

---

## 11. 测试

`relay/channel/task/tokenhub25/adaptor_test.go`：

1. 样片三入口（顶层 `draft`、`metadata.draft:true`、`metadata.draft:"true"`）都强写 `draft=true` + `resolution=480p`，且 `RequestedResolution` / `UpstreamResolution` = 480p。
2. 样片传 `720p` / `1080p` / `4k` → 400 `invalid_resolution`，`info.Draft` 保持 false。
3. 非样片 payload 无 `draft` 键、`resolution` 原样。
4. `metadata.content` 含 `video_url` → `info.HasVideoInput=true`。
5. finalize body `""` / `{}` / `{"resolution":"1080p"}` 通过；`{"resolution":"720p"}` → 400。
6. 成片 payload 白名单 + 反劫持：客户端塞 `content` / `draft_task` / `prompt` / `duration` / `ratio` / `seed` / `draft` 都进不了 payload，且 `content` 只有一项、只有 `type` 与 `draft_task` 两个键。
7. `info.Action` 不被覆盖（守住 tokenhub 写死的 `TaskActionGenerate`）。
8. `BuildRequestURL` == `https://ai-tokenhub.ai/api/v2/contents/generations/tasks`。
9. `omni_reference_task_type`：出现且取值原样；未传时不出现；非字符串 → 400。
10. finalize 且 `OriginUpstreamTaskID` 为空 → 报错。

```bash
go test ./relay/channel/task/tokenhub25/... ./relay/channel/task/tokenhub/... ./relay/channel/task/taskcommon/... ./relay/...
```

---

## 12. 明确不做

- **任务类型校验**：`content.role` + 提示词关键词判定的五种任务类型（文生视频 / 参考生视频 / 视频编辑 / 视频延长 / 首帧·首尾帧生视频），以及各自对 `ratio` / `duration` 的特殊限制，**本次一律不校验**。理由：那套判定规则会变，复制进网关就要跟着上游改；违反限制时上游自己会报错（显式传 `omni_reference_task_type` 时提交期同步报错，不传时任务启动后异步报错）。后续如需把校验前置，另开工单。
- **Remix**：`POST /v1/videos/{id}/remix` 不特殊处理，静默降级为普通 generate（继承原任务的计费倍率）。与 66 行为一致。要「响铃拒绝」应开独立工单把 61 / 66 / 67 一起收口。
- **对客文档**：`web/api-docs` 本次不动，等本地验收通过后另开文档单。
- 2.0 三条模型搬进本渠道（继续由 61 承载）。
- 把 2.5 的档位单价写进内置价格表。
- finalize 幂等键。
- 改 61 / 66 的任何行为。

---

## 13. 上游契约验证状态

实现按 66 的形状照抄。四项契约里**两项已用真华鲲 key 实测确认，两项仍未验证**。未验证的两项都是「会响」的失败（上游直接报错），没有静默错。

### 13.1 已实测确认（2026-10-14）

样本：`POST /v1/videos`，`{"model":"dreamina-seedance-2-5-260628","prompt":"a red ball on a table","metadata":{"draft":true}}`，走完整网关链路（渠道 67 → 华鲲 `https://ai-tokenhub.ai/api/v2/contents/generations/tasks`）。

| # | 项 | 结论 | 证据 |
|---|---|---|---|
| 1 | 华鲲 2.5 的轮询响应是否同 2.0 的 `resultSummary` 结构 | **是，逐字段吻合** | 回包含 `id` / `upstreamTaskId` / `status` / `model` / `createdAt` / `updatedAt` 与 `resultSummary.{content.video_url, duration, resolution, upstreamStatus, usage.{completion_tokens,total_tokens}}`，与 `tokenhub.responseTask` 结构体一一对应 |
| 2 | `draft=true` + `resolution=480p` 是否真出 480p | **是，上游确实出 480p** | 两条独立证据：① 上游自报 `resultSummary.resolution = "480p"`；② 下载成片 `ffprobe` 实测 **854×480**、时长 10.04s（与 `resultSummary.duration = "10"` 一致） |

第 2 条是原「唯一会静默少收」的形态，**已证伪**：华鲲不忽略 `draft`。

实测样本的完整回包（终态）：

```json
{
  "id": "01a13b04-...",
  "upstreamTaskId": "cgt-20261014232501-nhq4l",
  "status": "succeeded",
  "model": "dreamina-seedance-2-5-260628",
  "resultSummary": {
    "content": { "video_url": "https://ark-acg-ap-southeast-1.tos-.../cgt-....mp4?X-Tos-..." },
    "duration": "10",
    "resolution": "480p",
    "upstreamStatus": "succeeded",
    "usage": { "completion_tokens": 96475, "total_tokens": 96475 }
  }
}
```

计费链路随之核对无误：预扣费 ¥54.75（`ModelRatio/2`），终态按 token 重算 `96475 × 15 = 1,447,125` quota（¥2.89），`OtherRatios` multiplier = 1.0（480p 落 base 槽）——与「样片按 480p 档收」一致。

### 13.2 仍未验证

| # | 未验证项 | 失败形态 |
|---|---|---|
| 3 | finalize 的 `content:[{type:"draft_task",draft_task:{id}}]` 华鲲是否认 | 会响：上游报错 |
| 4 | 成片白名单字段（`return_last_frame` / `output_format` / `watermark` / `service_tier` / `execution_expires_after` / `priority` / `callback_url` / `safety_identifier`）华鲲是否全收 | 会响：上游报错 |

两项都只在**出成片**时才会触发，需要一份已完成的样片 + 一次真实 finalize 调用（另一次计费的上游任务）。**上线前必须测掉**——会响，但会让「出成片」整条路不可用。

### 13.3 运行时兜底：字段已确认存在，但尚未加

原先不加「样片回包档位 ≠ 480p 就记 warning」的理由是「回包里到底有没有档位字段还没见过」。**现在已知：有，就是 `resultSummary.resolution`**（见 13.1）。因此这条兜底从「循环论证」变成了可实施项。

本次仍未加，理由是它保护的正是刚刚被证伪的那条风险，当前没有已知的失败形态可捕获；但若日后出现「上游改了 `draft` 语义」或换了上游，这一行 warning 是唯一能在**扣费前**发现档位不符的信号。要加时落点在 `tokenhub25` 的 `ParseTaskResult` 覆写里比对 `resultSummary.resolution` 与 `info.UpstreamResolution`。

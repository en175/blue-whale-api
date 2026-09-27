# 火山 Seedance 2.5 渠道（样片模式）

Seedance 2.5 专用渠道，承载样片模式的两步流程。**没有抽奖、没有降级、没有 MediaKit 超分、没有 token 剥离**——2.5 不需要超分补偿，越权跑上去只会白花钱。契约以词表和 ADR 为准，本文是渠道说明书。

- **术语：** [`CONTEXT.md`](../../CONTEXT.md)
- **ADR：** [0019](../adr/0019-seedance-draft-mode.md) 样片模式：两步独立计费，成片源只有网关能指定
- **上游：** 火山方舟 Seedance 2.5（`doubao-seedance-2-5-260628`）

---

## 1. 需求

新增渠道类型 **VolcEngine25**（`ChannelTypeVolcEngine25 = 66`）。对客接口与豆包视频直连相同（OpenAI 兼容 `/v1/videos` + 任务式 `/v1/video/generations`），在此之上支持样片模式：

1. **样片（draft）**：`metadata.draft=true`（或顶层 `draft`）提交，**只允许 480p**，拿到低价预览。
2. **成片（finalize）**：`POST /v1/videos/{公开任务ID}/finalize` 基于一份已完成的样片出 1080p 成片。
3. **两步独立计费**：样片与成片各是一条任务行、各自预扣与结算。

不接：抽签降档、MediaKit 超分、token 剥离、Remix（本渠道未实现 remix 语义）。

---

## 2. 术语

以 `CONTEXT.md` 为准，本渠道用到的：

| 术语 | 含义 |
|---|---|
| 样片（draft） | 第一步。强制 480p 的低价预览，独立计费、独立任务行。 |
| 成片（finalize） | 第二步。基于样片的 1080p 输出，独立计费、独立任务行。 |
| 样片源 | 成片所引用的样片任务。必须是**本人的、SUCCESS 的、7 天内**的 draft 任务。 |
| 上游样片 ID | 样片在火山方的任务 ID（`cgt-...`），存 `private_data.upstream_task_id`，只由网关读取。 |
| 含视频输入 | 计费用语：样片请求 `metadata.content` 里出现 `video_url`。成片的 1080p 档单价取它。 |

---

## 3. 主链路

```
第一步  用户 POST /v1/videos
        { model, prompt, metadata:{ draft:true, resolution:"480p" } }
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

---

## 4. 样片（第一步）

### 4.1 触发

两条入口等价，任一为真即为样片：Ark 顶层 `draft`（`true` 或 `"true"`）与 `metadata.draft`（`true` 或 `"true"`，大小写不敏感）。

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

`action` 列沿用推断出的提交形态（`textGenerate` / `generate` / `videoGenerate`），样片身份只由 `properties.draft` 表达。

---

## 5. 成片（第二步）

### 5.1 入口

`POST /v1/videos/{公开任务ID}/finalize`。请求体可为空、可为 `{}`，也可带输出规格参数。

渠道被锁定为原任务的渠道，模型名沿用原任务的上游模型名——客户端不传 `model`。

### 5.2 样片源校验（全部在预扣费之前）

| 条件 | 失败响应 |
|---|---|
| 任务是本人的 | 400 `task_not_exist` |
| `properties.draft == true` | 400 `origin_not_draft` |
| `status == SUCCESS` | 409 `origin_task_not_finished` |
| 完成时间在 7 天内 | 400 `origin_draft_expired` |
| 原任务模型名可推导 | 400 `origin_model_name_missing` |
| `private_data.upstream_task_id` 非空 | 400 `origin_upstream_id_missing` |

最后一条**不用** `Task.GetUpstreamTaskID()`：它在 `PrivateData` 为空时回退到公开 ID `task_xxx`，把它当 `draft_task.id` 发出去上游必然报错。

### 5.3 请求体白名单

客户端可传（全部可选）：`resolution`（只允许 `1080p`，否则 400）、`return_last_frame`、`output_format`、`watermark`、`service_tier`、`execution_expires_after`、`priority`、`callback_url`、`safety_identifier`。

客户端**不能**影响：`model`、`content`、`prompt`、`duration`、`ratio`、`seed`、`generate_audio`、`omni_reference_task_type`——这些字段在上游 payload 的结构体上根本不存在，客户端无论传什么都进不去。`draft_task.id` 只来自原任务的 `private_data.upstream_task_id`。

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

> ⚠️ **管理员必须配齐。** 内置价格表**不含** 2.5 的单价（刻意为之）。若只配了 `base`：
> - `base` = 480p / 不含视频输入的基准价，应等于该模型的 `ModelRatio`
> - `1080p` = 1080p / 不含视频输入
> - `1080p_video` = 1080p / 含视频输入
>
> 缺 `1080p` / `1080p_video` 时，成片会**按 480p 基准价结算**——不报错、不打日志、静默少收。

结算链路不变：按 `ModelRatio` 配置时 `PerCallBilling=false`，任务完成时走 `RecalculateTaskQuotaByTokens`，用落库的 `OtherRatios` 乘积作 multiplier。

---

## 7. 防重复扣费

`shouldRetryTaskRelay` 对 `/finalize` 直接返回 false。上游 create 接口不是幂等的，而锁渠道路径没有 `specific_channel_id`，`controller/relay.go` 的守卫拦不住。重试次数默认 0 所以今天不会触发，但运维一旦调大，超时 / 5xx 重试会让同一份样片出两段成片、扣两次费。

同一份样片调两次 finalize **仍会**出两段成片、扣两次费——没有幂等键。彻底解决需在原任务上 CAS 写 `PrivateData.FinalizedBy`，另开工单。

---

## 8. 渠道注册

- `ChannelTypeVolcEngine25 = 66`，插在 `ChannelTypeDummy` **之前**。
- `ChannelBaseURLs[66] = "https://ark.cn-beijing.volces.com"`（下表寻址，必须补齐，否则越界 panic）。
- `ChannelTypeNames[66] = "VolcEngine25"`。
- `GetTaskAdaptor`：新类型返回 `taskvolc25.TaskAdaptor`。
- `controller/channel-test.go` 的 `unsupportedTestChannelTypes` 含 66（视频任务渠道不做联通性测试）。
- **无渠道 `setting`**：本类型不读任何渠道配置（无彩票百分比、无 MediaKit key）。

前端：`CHANNEL_TYPES` / 展示序 / 类型配置加 66；渠道标签映射到 `Volcengine`。

---

## 9. 适配器

新建包 `relay/channel/task/volcengine25/`：

```go
type TaskAdaptor struct {
    taskdoubao.TaskAdaptor
}
```

只覆盖三个方法，其余（`BuildRequestURL` / `BuildRequestHeader` / `DoRequest` / `DoResponse` / `FetchTask` / `ParseTaskResult`）全部继承豆包：

| 方法 | 行为 |
|---|---|
| `ValidateRequestAndSetAction` | finalize 走白名单校验并**提前返回**；否则委托豆包后补 draft 的 480p 守卫与 `has_video_input` 采集 |
| `BuildRequestBody` | finalize 构造白名单 payload；draft 改写豆包的 payload 强制 `draft=true` + `resolution=480p`；其余原样 |
| `EstimateBilling` | finalize 用 `EstimateVideoTierBillingForTier(info, 1080p, originHasVideoInput)`；其余委托豆包 |

⚠️ finalize 分支必须在委托豆包**之前**判定：豆包的 `ValidateRequestAndSetAction` 末尾会用 `inferDoubaoVideoAction` 覆盖 `info.Action`，且成片请求体里没有 `prompt`，走豆包的 `validatePrompt` 必然 400。

`doubao` 包只有一处改动：导出 `DraftTaskRef` 并给 `ContentItem` 加 `DraftTask`（`omitempty`，45/54 渠道 payload 逐位不变）。

---

## 10. 实现落点

| 动作 | 路径 |
|---|---|
| 新适配器 | `relay/channel/task/volcengine25/`（`adaptor.go` / `constants.go` / `draft.go`） |
| 渠道常量与注册 | `constant/channel.go`、`relay/relay_adaptor.go` |
| action 常量 | `constant/task.go`（`TaskActionFinalize = "finalizeDraft"`） |
| 路由 | `router/video-router.go` |
| 中间件锁渠道路径 | `middleware/distributor.go` |
| 样片源解析与校验 | `relay/relay_task.go` `ResolveOriginTask` |
| 新增字段 | `relay/common/relay_info.go`（`TaskRelayInfo` / `TaskSubmitReq`）、`model/task.go`（`Properties` / `TaskPrivateData` / `InitTask`） |
| 计费档位入口 | `relay/channel/task/taskcommon/video_tier.go` |
| 禁重试 | `controller/relay.go` `shouldRetryTaskRelay` |
| 测试白名单 | `controller/channel-test.go` |
| 前端 | `web/default/src/features/channels/`、`features/usage-logs/constants.ts` |
| 决策 | `docs/adr/0019-seedance-draft-mode.md` |

---

## 11. 测试

1. **样片入口**：顶层 `draft` 与 `metadata.draft`（含字符串 `"true"`）都置 `info.Draft`，payload 同时含 `draft=true` 与 `resolution=480p`（即使客户端没传 resolution）。
2. **样片分辨率守卫**：`720p` / `1080p` / `4k` → 400 `invalid_resolution`，且不置 `Draft`。
3. **非样片不受影响**：分辨率原样透传，payload 不出现 `draft` 键。
4. **含视频输入采集**：样片请求带 `video_url` → `info.HasVideoInput` 为真。
5. **成片请求体**：空 body / `{}` / `{"resolution":"1080p"}` 通过；`{"resolution":"720p"}` → 400。
6. **成片 payload 白名单 + 反劫持**：客户端塞 `content` / `draft_task` / `prompt` / `duration` / `ratio` / `seed` / `generate_audio` / `omni_reference_task_type` 都改不动 payload；`content[0].draft_task.id` 等于原任务的上游 ID。
7. **action 不被覆盖**：finalize 路径 `info.Action` 保持 `finalizeDraft`。
8. **样片源校验**：非 draft → `origin_not_draft`；状态非 SUCCESS → `origin_task_not_finished`；超 7 天 → `origin_draft_expired`；上游 ID 缺失 → `origin_upstream_id_missing`；他人任务 → `task_not_exist`；窗口内少 1 秒仍可用。
9. **不继承 OtherRatios**：finalize 的 `PriceData.OtherRatios()` 为空。
10. **档位继承**：`EstimateVideoTierBillingForTier` 在 `("1080p", true)` 与 `("1080p", false)` 下倍率必须不同；未知模型 / 倍率 1 → nil；原 `EstimateVideoTierBilling` 行为不变。

```bash
go test ./relay/channel/task/volcengine25/... ./relay/channel/task/taskcommon/... ./relay/ -run 'Volcengine25|Draft|Finalize|VideoTier|ResolveOriginTask'
```

---

## 12. 明确不做

- 抽签降档、MediaKit 超分、token 剥离（2.5 不需要补偿；这些能力留在 64 渠道）。
- Remix。
- `requestPayload` 补 `omni_reference_task_type` / `output_format`（属 2.5 的视频编辑 / 延长能力，另开工单）。
- finalize 幂等键。
- `PublicData()` 对 `data.id` 做脱敏（所有 doubao 任务的既有行为）。
- 把 2.5 的档位单价写进内置价格表。
- 改 45 / 54 / 64 三个既有渠道的任何行为。

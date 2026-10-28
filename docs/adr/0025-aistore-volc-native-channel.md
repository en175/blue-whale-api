# 新开渠道 71 运河启源，对客原样透传火山原生协议

下游要照抄《运河启源 AI Store Seedance 接入手册》直接接入（`https://aistore.gsiig.cn/v1`）。该手册是火山方舟协议的转售封装，与渠道 70 同形不同源。本次新开渠道类型 **71（AistoreVolc）**，对客只暴露 `/api/v3/contents/generations/tasks`，请求体与响应体按火山原文透传。

**Status:** proposed

## 背景

- 渠道 54 是火山官方直连，但对客是 OpenAI 视频形状，且 `requestPayload` 只建模了部分字段：`output_format` 会被静默丢弃，`duration: -1` 被通用时长校验直接 400，上游 `expired` 终态未映射（任务会一直当进行中轮询），`content.last_frame_url` 未解析。
- 渠道 70 已经证明「华鲲上游 + 火山原生对客协议」这条路可行，但它的上游是 `/api/v2` + `resultSummary` 信封，与运河启源的 `/v1` 直出形状不同。
- 直接改 54 会破坏其存量下游；ADR 0024 已记录过同类理由：类型号落库，事后合并意味着数据迁移和任务归属重算。

## 决策

1. **新开渠道类型 71 `AistoreVolc`，不改 54 / 66 / 70。**
   - `ChannelBaseURLs[71] = "https://aistore.gsiig.cn/v1"`，适配器拼 `{base}/contents/generations/tasks`。
   - 上游把版本前缀放在 base URL 里，与火山的 `/api/v3` 不同；`BuildRequestURL` 与 `FetchTask` 都不能沿用 doubao 的实现。
   - 不新增 APIType：与 66 / 70 一样是任务专用渠道，`common.ChannelType2APIType` 返回 false，模型注册循环自动跳过。

2. **对客只开火山原生协议，不兼容旧视频接口。**
   - `constant.ChannelServesRequestPath`：71 只服务 `NativeArkVideoTaskPath`；`/v1/video/generations`、`/v1/videos/{id}/finalize`、remix 一律不路由。
   - 样片成片走同一个提交入口的 `content[].draft_task`，与火山官方一致。

3. **请求体原样透传，只覆盖 `model`。**
   - 以中间件解析出的 `native.Raw` 为底，未知字段（`output_format`、`return_last_frame`、`service_tier`、`priority`、`callback_url`…）全部保留。
   - `duration` 由适配器判边界：`-1` 放行，其余必须落在 `1..MaxTaskDurationSeconds`。

4. **响应体保留上游全部字段，只换 id。**
   - `id` 换成网关公开任务 ID；上游 `cgt-` 号放进 `upstreamTaskId`。
   - `status` 直接采用上游状态词（`queued` / `running` / `succeeded` / `failed` / `expired`），缺字段时按任务表兜底。
   - `expired` 映射为任务失败，触发既有失败退款。

5. **样片模式：2.5 独有、固定 480p，两档计费。**
   - 只有模型名含 `2.5` / `2-5` 才能开 `draft`；样片强制 `draft=true` + `resolution=480p`。
   - 成片固定 1080p，「是否含视频输入」继承第一步的样片，复用 `taskcommon.EstimateVideoTierBillingForTier`。
   - 价表 `video_tier_pricing.price_table` 按厂商模型名（如 `cdance2.5`）配置。

6. **视频地址直出，不做网关代理。** 与 `CONTEXT.md` 的直链口径一致；上游 24 小时过期，由下游自行转存。

## 理由

1. **对客契约是渠道的一部分。** 54 已经用 OpenAI 形状服务存量下游；把「原样透传」塞进 54 会让每条既有路径都带分支，与 ADR 0021 / 0024 记录的问题同类。
2. **70 的转换层不能直接复用。** 70 的上游是华鲲 `/api/v2` + `resultSummary` 信封，运河启源是 `/v1` 直出；共用适配器会让两边的请求/响应解析互相污染。
3. **透传优先于建模。** 用结构体建模火山字段，新增字段会被静默丢弃（ADR 0021 的既有教训）。下游要的是「照文档接入」，原样转发是最小维护成本的实现。
4. **独立渠道可独立限额与灰度。** 新上游上线初期需要单独观察成功率与扣费口径。
5. **网关公开 ID 不可让渡。** 对客若直接用 `cgt-` 号，网关需要额外维护 cgt→任务的映射表才能做归属校验、轮询与计费；当前公开 ID 方案只在下游可见性上退让一步，用 `upstreamTaskId` 补足排障需求。

## 后果

- **54 / 66 / 70 的行为逐位不变。** 新适配器只处理类型 71，新路径判定只多一个 `case`。
- **响应形状随上游漂移。** 上游新增或改名厂商字段会直接出现在对客响应里；这是透传的代价，若上游开始返回与火山冲突的同名字段，需要改为白名单建模。
- **样片与成片是两条独立任务行，各自计费。** 同一份样片连续成片两次会出两段成片、扣两次费（与渠道 66 / 70 相同的已知限制）。
- **管理员必须按厂商模型名配置档位价表。** `cdance2.5` 的 `base` / `1080p` / `1080p_video` 漏配会静默按基准价结算。
- **上游状态词表以探针为准。** 已映射 `queued` / `running` / `succeeded`（含 `success`）/ `failed` / `expired` / `cancelled`（含 `canceled`）；出现新的终态词若不补映射会按进行中处理，直到全局任务超时才判失败并退款，需在上游变更时同步。
- **不提供素材上传 REST**（手册 4.2 的 `/api/assets/*`）。`asset://` 引用可以透传，但素材登记仍走现有素材库接口。
- **不做文本模型。** 手册里 Qwen 的示例属于厂商文档残留；文本接入另起需求。

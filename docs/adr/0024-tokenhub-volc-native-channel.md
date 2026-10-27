# 华鲲上游单开渠道 70 并对客暴露火山原生协议；素材库新开根路径接口

网关现有三个火山/华鲲视频渠道（61 TokenHub、66 VolcEngine25、67 TokenHub25）对客都是 OpenAI 形状 `/v1/videos`。下游拿着火山方舟官方文档无法直接接入。本次新开渠道类型 **70（TokenHubVolc）**，上游仍是华鲲，对客改成火山原生 `/api/v3/contents/generations/tasks`；素材库另开一套根路径接口 `POST /?Action=X&Version=2024-01-01`，现有 `/api/material/{Action}` 原样保留。

**Status:** proposed

## 背景

- 66 的上游是火山方舟，67 的上游是华鲲；两者的对客协议都由网关统一成 `/v1/videos`（`prompt` + `metadata`）。下游要接华鲲，必须先把火山的 `content[]` 拆成 `prompt` 和 `metadata`，响应也要按 OpenAI 视频对象重新解析。
- 需求方希望「下游照抄火山官方文档即可接入」：URL、请求体、响应体、错误信封全部保持火山形状，只换 host 和 API key。
- 素材库现状（ADR 0007）是把上游归一成 tokenhub 的实际格式：Action 在路径、会话类剥壳、错误统一 `ResponseError`。与火山管控面 `?Action=X&Version=...` + `ResponseMetadata` 信封不一致。

## 决策

### 1. 新开渠道类型 70，不改造 61 / 66 / 67

- 类型 70 `TokenHubVolc`，上游 base URL `https://ai-tokenhub.ai`，一个类型同时承载 2.0 与 2.5。
- 对客新增独立路由：

  ```
  POST /api/v3/contents/generations/tasks
  GET  /api/v3/contents/generations/tasks/:id
  ```

- 请求进入时由新中间件把火山原生 body 归一成内部 `TaskSubmitReq`，适配器再按华鲲 `/api/v2` 重建 payload；响应侧做反向转换。
- `/api/v3` 只路由类型 70；`/v1/videos` 的渠道选择排除类型 70。判定收敛在 `constant.ChannelServesRequestPath` 一处，由亲和检查、内存缓存过滤、数据库能力过滤三条选路共用。

### 2. 素材库新开根路径接口，旧接口不动

- 正式路径 `POST /?Action=X&Version=2024-01-01`，别名 `POST /api/ark?Action=...`（防异常部署根路径不可达）。
- 鉴权仍是网关 `sk-`；`Version` 不校验，与其余 query 一起原样透传上游（缺省补 `2024-01-01`），上游错误码不改写。
- 对客响应统一火山信封：华鲲的顶层 `ResponseError` 由网关搬迁进 `ResponseMetadata.Error`；会话类接口上游不包信封，由网关补 `ResponseMetadata` / `Result`。volcengine 直连本就符合火山形状，两步均为空操作。
- 已知差异不补偿：游标分页（`NextToken` / `MaxResults`）上游不支持，只提供页码分页（火山同样支持该模式），因此深翻页（`PageSize * PageNumber > 20000`）不可用。`UpdateAssetGroup` 改 `Name` 经 2026-10-24 真机联调验证可用（`GetAssetGroup` 读回新名称），此前记录的 `UnsupportedField` 结论不成立。
- 保留归属校验、组列表的本地隔离与真人认证回调改写；去掉认证结果缓存、名称过滤。`ListAssetGroups` 查本地归属表；`GetAsset` / `UpdateAsset` / `DeleteAsset` 经上游 `GroupId` 反查归属；`GetVisualValidateResult` 先校验会话属于当前 API key。

## 理由

1. **对客协议是渠道契约的一部分。** 61/66/67 已经用 `/v1/videos` 服务存量下游，切换形状会直接打断它们。把两套形状塞进同一个 adaptor，会让每条路径都要判断「这次请求是哪种形状」，与 ADR 0021 记录的「扩 61 会让每条既有路径都带上分支」是同一类问题。
2. **66 不能复用。** 66 的上游是火山官方；需求要的是华鲲上游 + 火山形状。把 66 的 base URL 指向华鲲后，路径和响应解析都会错位——这正是 ADR 0021 实测过的坑（华鲲 `/api/v3` 返回 404）。
3. **67 只承载 2.5，且对客是 OpenAI 形状。** 需求要 2.0 + 2.5 共用一套原生协议；改造 67 会同时破坏存量 2.5 下游和渠道语义。
4. **类型号落库，难回头。** `channels.type` 持久化在任务表与渠道表里；事后合并意味着数据迁移和任务归属重算。新开 70 的代价只是一份转换代码，回滚成本低。
5. **70 可以独立限额、独立灰度。** 原生协议上线初期需要单独观察请求成功率、draft 两步成功率和扣费口径；混在 66/67 里做不到。

素材库部分：

6. **旧接口已外发，改形状等于破坏契约。** 新根路径与旧接口共用 provider、凭证与归属表，只多一层形状转换，不增加运维配置。
7. **保留归属校验与回调改写是安全底线。** 官方 API 没有网关侧归属概念（凭证即租户），但网关是单租户共享凭证，去掉归属校验后任意下游都能删别人的素材组；回调改写是真人认证闭环能拿到结果的唯一手段。这两项与「响应形状一致」不冲突。
8. **组列表与素材读取必须走网关归属，结果缓存和名称过滤可以去掉。** 兼容入口曾把 `ListAssetGroups` 直接转给共享上游账号，任意 API key 都能看到同一份组列表；`GetVisualValidateResult` 也未校验会话归属。共享凭证下，列表和读取是跨租户边界，不能为了「行为与上游一致」放弃隔离。认证结果缓存和名称校验不涉及跨租户可见性，去掉后行为仍可预期。

## 后果

- **61 / 66 / 67 的行为逐位不变。** 新中间件只挂在 `/api/v3` 路由组，新适配器只处理类型 70。
- **新增一份请求/响应双向转换。** 火山原生字段与内部 `TaskSubmitReq` 的映射是本方案的主要维护成本；火山新增字段时需要同步更新映射表，否则字段会被静默丢弃。
- **任务 ID 统一用网关公开 ID。** 2.5 第二步的 `draft_task.id` 也只接受公开 ID，网关查库换成华鲲的 `cgt-` ID。下游不能直接使用上游 ID。
- **`duration` 由适配器校验，不进内部任务的时长字段。** 火山用 `-1` 表示「编辑/延长由模型决定时长」，内部任务校验只接受正数；原生请求的 `duration` 在适配器里判定边界（`-1` 或 1..`MaxTaskDurationSeconds`），原值随原始 body 透传上游。
- **只有 2.5 能开样片，成片固定 1080p。** 上游成片档位只有 1080p，价表只需 `1080p` 与 `1080p_video`；漏配会静默按基准价少收。
- **素材库两套接口并存，行为有差异。** 两套接口都按 API key 做归属隔离：`ListAssetGroups` 都查本地归属表，仅 `GetVisualValidateResult` 不读缓存、名称过滤只由旧接口承担。文档必须分别说明，不能用一套说明覆盖两者。
- **单素材 Get / Update / Delete 的归属校验依赖上游 `GetAsset` 返回 `GroupId`。** 不返回的 provider 按拒绝处理（fail-closed）。
- **根路径 `POST /` 被占用。** 后续不能再在根路径挂其它 POST 路由；`/api/ark` 是异常部署时的退路。
- **需求规格、验收场景与开放问题以 PRD 为准：** `docs/prd/2026-10-23-tokenhub-volc-native-channel.md`。其中「`ListAssetGroups` 直查上游」「`GetAsset` 不校验归属」「`GetVisualValidateResult` 不校验会话」三处已被下面的修订推翻，以本 ADR 与 IKQFXR 为准。

**2026-10-26 修订（IKQFXR）：** 兼容入口的 `ListAssetGroups` 改为查本地归属表；`GetVisualValidateResult` 补会话归属校验，并按会话记录的 provider 路由、补建会话时的 `ProjectName`；`GetAsset`（两个入口）补归属校验且复用探测响应；本地列表失败返回对应格式的错误信封。归属字段按大小写不敏感匹配，与上游 JSON 解码口径一致。

**2026-10-27 修订（渠道 70 兼容旧视频接口）：** 类型 70 不再只服务 `/api/v3`，同时兼容网关旧视频任务接口 `/v1/video/generations`（提交、查询）与 `/v1/videos/{id}/finalize`（成片第二步）。路径判定仍收敛在 `constant.ChannelServesRequestPath`；旧提交入口接受 `prompt + seconds + metadata`，由适配器还原成火山形状后与原生入口共用校验、draft 守卫、上游构造与计费。`/v1/videos`（OpenAI 视频创建）与 `/v1/videos/{id}/remix` 仍不路由给类型 70：remix 会锁定原任务渠道、绕过 Distribute，因此在 `ResolveOriginTask` 里补了一次路径判定。旧提交入口显式拒绝 `content[].draft_task`，成片只能走带归属与 7 天窗口校验的 finalize 入口。响应按入口分流：原生只回 `{"id": ...}`，旧接口回 OpenAI 视频对象。

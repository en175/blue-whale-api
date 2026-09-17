# MediaKit 渠道

把火山 AI MediaKit 的工具集接进网关，对客透传上游协议。契约以词表和 ADR 为准，本文是渠道说明书。

- **术语：** [`CONTEXT.md`](../../CONTEXT.md)
- **ADR：**
  - [0003](../adr/0003-xfai-video-channel.md) 渠道边界由上游协议与端点决定（同系列原则见 0004）
  - [0004](../adr/0004-doubao-audio-channel.md) 透传而非字段映射
  - [0018](../adr/0018-mediakit-channel-and-tool-registry.md) 本渠道的类型、注册表、计费与审计表决策
- **上游：** [AI MediaKit 文档库 6448](https://docs.volcengine.com/docs/6448)

---

## 1. 需求

新增渠道类型 **MediaKit**（`ChannelTypeMediaKit = 65`），把官方工具集的 **64 条**工具对客开放（视频 33 / 图像 27 / 音频 4，与官方在这三个集合下登记的条数一一对应）：

1. **一条渠道类型统管全部工具**。同步工具走普通 `Adaptor`（`controller.Relay` 管道），异步工具走 `TaskAdaptor`（`controller.RelayTask` 管道）；两张适配器表互不干扰。
2. **透传上游协议**。客户端按 MediaKit 原生请求体写，网关一个字都不改写（body 里没有 `model` 字段）。
3. **工具名只用于查表**。客户端传的 `:tool` 查不到一律 400；上游路径只来自服务端写死的注册表。
4. **混合计费口径**。按时长 / 按次 / 按量 GiB 三档，用户实际扣费由管理员配的 `model_price` 决定。

**3 个元信息查询工具**（`probe-image-metadata` 同步、`probe-video-metadata` / `probe-audio-metadata` 异步）已登记。它们在**上游并非免费**，网关按官方口径单独定价：

| 工具 | 官方口径 | 网关 `model_price` |
|---|---|---|
| `probe-video-metadata` / `probe-audio-metadata` | 按成功处理次数 0.1 元/千次 | 0.0001 元/次（一致） |
| `probe-image-metadata` | 按处理量 0.025 元/GiB | 0.00024 元/次（**近似**，见下） |

> 图像那条是近似值：模型定价只有「元/次」一档，**网关不测量图片体积**（同步链路里没有任何体积项），故按单图 10 MiB 上限折算 `10/1024 GiB × 0.025 = 0.00024 元/次`。大图不亏、小图略多收。

不接：`video-understand-router` / `chat` / 大模型高光剪辑 / 剪辑套件（拼接、调速、滤镜等约 19 个）——依赖方舟按 token 计费，或官方未给独立 slug。

---

## 2. 术语

以 `CONTEXT.md` 为准，本渠道用到的：

| 术语 | 含义 |
|---|---|
| MediaKit 工具 | 官方 slug（如 `enhance-video-generative`），是路径段也是注册表的键 |
| 工具模型名 | 对客模型名 `mediakit-<工具>`；加了前缀避免与任何上游真实模型 id 冲突 |
| 同步工具 | 一次 `POST` 直接返回产物（图像类 `/api/v1/tools-sync/*`） |
| 异步工具 | 提交拿 `task_id`，轮询取结果（音视频类 `/api/v1/tools/*`） |
| 公开任务ID | 网关下发的 `task_xxx`；客户端凭它查询，不接触上游任务号 |
| 任务审计行 | `mediakit_costs` 的一行，任务终态时写入 |
| 实扣额度 | 退款与差额结算**之后**的最终扣费，非预扣值 |

---

## 3. 对客路由

| 方法 | 路径 | 用途 |
|---|---|---|
| `POST` | `/v1/mediakit/sync/{tool}` | 同步工具，一次往返返回产物 |
| `POST` | `/v1/mediakit/{tool}` | 异步工具提交，落 `tasks` 表 |
| `GET` | `/v1/mediakit/tasks/{task_id}` | 查询任务状态与结果 |

三组都带 `middleware.TokenAuth()` + `middleware.Distribute()`。

**查询走本库而非代理上游**：客户端持有的是网关公开任务 ID，直连上游 `/api/v1/tasks/{id}` 既暴露上游任务号，也允许越权枚举他人任务。查询从 `tasks` 表读（`relay/relay_task.go` 的 `mediaKitFetchRespBodyBuilder`），渠道由任务记录决定，`shouldSelectChannel = false`。

**同步/异步路由互斥**：同步路由收到异步工具（或反之）在**分销阶段**就以 400 拒绝。放到适配器里拒绝会让客户端的用法错误表现为 500。

---

## 4. 工具注册表

`relay/channel/mediakit/registry.go` 是唯一事实来源，共 64 条：

| 分类 | 条数 | 路径前缀 | 计费模式 |
|---|---|---|---|
| 视频 | 30 | `/api/v1/tools/` | 按时长（`extract-frames` 按次） |
| 图像 | 14 | `/api/v1/tools-sync/` | 按次 |
| 图像基础编辑 | 12 | `/api/v1/tools-sync/` | 按量 GiB |
| 音频 | 3 | `/api/v1/tools/` | 按时长 |

每条记录 `{Name, Path, Sync, Mode, ResultKind}`。**路径不接受客户端输入**：客户端传的 `:tool` 只用于 `LookupTool` 查表，上游 URL 由表项写死的 `Path` 拼成，否则 `base + "/api/v1/tools/" + 客户端输入` 是明显的路径穿越面。

登记在表的工具才会出现在渠道模型列表，管理员按需勾选实现分批放量。

---

## 5. 请求与响应

**请求侧完全透传**。MediaKit 原生请求体没有 `model` 字段，逐字段映射既引维护成本又容易在客户端需要新参数时漏掉。`BuildRequestBody` 走 `common.GetBodyStorage` 原样转发。

模型名由**路径上的工具名**决定（`mediakit-<tool>`），不来自请求体——所以 `middleware/distributor.go` 的 MediaKit 分支**必须排在「读 JSON body 取 model」的兜底分支之前**，否则会拿到空模型名，请求随后以「无可用渠道」失败。

**异步提交的对客响应**沿用 MediaKit 信封（`{success, task_id, message}`），但 `task_id` 换成网关公开 ID；上游任务号只落 `private_data`。

**上游以 HTTP 200 + `success=false` 表达业务失败**，必须在 `DoResponse` 里拦下——`relay_task.go` 只拦非 200 响应码。

**结果键名不固定**（`video_url` / `image_url` / `audio_url` / 多产物工具的 `images[]` / `video_urls`），故按 `map[string]any` 解析，位置由注册表逐工具声明：`ResultPath` 非空按 gjson 路径精确取值（可指向数组元素与嵌套字段），否则按 `ResultKind` 取约定键名。取不到一律返回空串，不为 64 个工具各写 struct。

> **曾有过、已移除**：早期实现对取不到的键兜底扫任意以 `_url` 结尾的字符串键、按键名排序取第一个。该兜底对多产物工具（`separate-voice` 的人声/伴奏、Drama 场景的 voice/music/sfx）会随键名字典序漂移到语义无关的轨道上，故删除。详见 `registry.go` `ResultURL` 的注释与 `TestResultURLRealToolShapes`。

---

## 6. 计费

### 6.1 三档口径

| 模式 | 覆盖 | 对客口径 |
|---|---|---|
| 按时长 | 视频 30 + 音频 3 | 管理员按元/分钟配 `model_price`；提交按预估分钟预扣，完成用上游回包真实时长结算差额 |
| 按次 | 图像 14 + `extract-frames` | 每次固定价 |
| 按量 GiB | 图像基础编辑 12 | 直接按超出档单价 0.025 元/GiB，**不套官方「前 10 TiB 免费 + 自然月阶梯」** |

GiB 档不套阶梯是为了让三类计费量**全部逐请求可算**，网关无需维护账号级月度累计表。

### 6.2 差额结算需要显式 opt-in

`service/task_polling.go` 的差额结算第一句是「按次计费直接返回」，而按次计费的判定条件之一是「命中 `model_price`」。按时长计费的渠道恰恰也要把价格配在 `model_price` 上（元/分钟），于是会被误判为按次、整段跳过结算——`AdjustBillingOnComplete` 永远不生效，用户按预估时长被扣，真实时长从不参与计价。

解法：`TaskBillingContext.DurationSettlement` 由适配器在 `EstimateBilling` 里对时长类工具置位，判定条件改为 `PerCallBilling && !DurationSettlement`。纯追加，现有渠道行为逐字不变。

### 6.3 结算取值

`AdjustBillingOnComplete` **签名没有 ctx**、`TaskInfo` 也没有 duration 字段，故真实时长只能从 `task.Data` 里挖——轮询层在写库前已把查询响应存进去。取不到时**返回 0、保持预扣不变**（宁可少结算，不靠猜收费），并打一行 `result 键：…` 供收敛字段名。

上游回包时长属上游可控数据，取到后按 `MaxTaskDurationSeconds` 钳制；换算走 `common.QuotaRoundChecked`，饱和事件经 `TaskQuotaClampReporter` 交回轮询层写进结算日志的 `admin_info.quota_saturation`。

**失败不计费**：官方「任务处理失败不触发计费和抵扣」与现有 `RefundTaskQuota` 语义天然对齐，异步任务失败全额退预扣。

### 6.4 `model_price` 缺失即拒

同步链路（`MediaKitSyncHelper`）在预扣前检查 `model_price` 是否命中，未命中返 400。

原因：MediaKit 请求体里没有 token，按次计价的唯一来源是模型定价。只配了倍率时，预扣按倍率算、结算却按上游返回的 1 个 token 折算并把差额退回，**实际等于白送**。管理员确实要免费就把 `model_price` 配成 0。

---

## 7. 审计表 `mediakit_costs`

一个任务一行，在「推进到终态」的那次 CAS 成功时写入，重复轮询不会写重复行。写入时机在差额结算与退款**之后**，`charged_quota` 是最终实扣（失败退款记 0）。超时被清扫的任务同样落一行——这类任务上游可能仍在跑并计费。

**本表不记上游成本**。官方费率是「工具 × 输出档（按短边）× 帧率档」的矩阵，三种计费模式量纲还不同，尚未逐工具核实；把猜测的费率硬编进代码会把错误固化进已迁移的库，把用户实扣换算成人民币填进 `amount_fen` 更糟（名字说成本、内容是收费）。成本交给对账方用「`actual_seconds` × 官方费率」在 SQL 外算。

落点是 `service/task_polling.go` 的可选接口 `TaskCostRecorder`（目前仅本渠道实现），详见 `docs/db-schema/docs/mediakit_costs.md`。

---

## 8. 渠道配置

- **API Key 走渠道 `key` 字段**（`info.ApiKey`），不用 `setting.mediakit_api_key`：MediaKit 是单头 `Authorization: Bearer <apiKey>`，天然适配渠道 key 机制，且轮询层 `FetchTask(baseUrl, key, ...)` 拿到的正是渠道 key，走 setting 会让轮询拿不到。
- **上游域名走渠道自身的 `base_url` 字段**（`info.ChannelBaseUrl`，默认 `https://mediakit.cn-beijing.volces.com`）。**不要**配 `setting.mediakit_base_url`——那是 type 64 的键，type 65 的两条取值路径（`adaptor.GetRequestURL` / `task_adaptor.BuildRequestURL`）都只读 `info.ChannelBaseUrl`，配了不生效。
- `mediakit_default_minutes`：时长类工具**提交时拿不到真实时长**（入参多为 URL），故用这个预估分钟数参与预扣，校验 0–60。不配（或填 0）时预扣退回 `model_price` 的基准额度（≈1 分钟量），真实时长的差额在任务完成时才补扣——**不是**「预扣为 0、先用后付」。该字段在控制台 MediaKit 渠道抽屉里可配；只在 API 里写、之后从控制台保存一次会被清掉（`buildSettingJSON` 现已有 type 65 分支，正常路径不会丢）。
- `proxy`：渠道级代理。
- 保存校验：`ValidateMediaKit()`，在 `model/channel.go` 的 `ValidateSettings()` 里按 `channel.Type == ChannelTypeMediaKit` 调用。
- 渠道测试（余额查询等）标记为不支持。

---

## 9. 实现落点

| 动作 | 路径 |
|---|---|
| 工具注册表 / 常量 / 价表（待补） | `relay/channel/mediakit/registry.go`、`constants.go` |
| 上游客户端 | `relay/channel/mediakit/client.go` |
| 同步适配器 | `relay/channel/mediakit/adaptor.go` |
| 异步适配器 | `relay/channel/mediakit/task_adaptor.go` |
| 同步 handler | `relay/mediakit_handler.go` |
| 路由 | `router/mediakit-router.go`、`router/main.go` |
| 分销分支 | `middleware/distributor.go` |
| 中继模式 | `relay/constant/relay_mode.go`、`types/relay_format.go` |
| async fetcher | `relay/relay_task.go`（`mediaKitFetchRespBodyBuilder`） |
| 差额结算 opt-in | `model/task.go`、`service/task_polling.go` |
| 审计表 | `model/mediakit_cost.go`（新）、`model/main.go` AutoMigrate |
| 渠道常量与注册 | `constant/channel.go`、`constant/api_type.go`、`common/api_type.go`、`relay/relay_adaptor.go` |
| 渠道 setting 校验 | `dto/channel_settings.go`、`model/channel.go` |
| 测试白名单 | `controller/channel-test.go` |
| 前端类型 + i18n | `web/default/src/features/channels/`、`src/i18n/locales/*.json` |
| 表字典 | `docs/db-schema/docs/mediakit_costs.md` |
| 术语 | `CONTEXT.md` |
| 决策 | `docs/adr/0018` |

复用：`taskcommon.BaseBilling`、`channel.DoTaskApiRequest`、`common.GetBodyStorage`、`common.QuotaFromFloatChecked` / `QuotaRoundChecked`、`logger.Log*`（带 ctx）、现有预扣 / 结算 / 退款 / 轮询链路。

---

## 10. 测试

表驱动 + `testify`。

1. **注册表**：64 条工具名唯一；`LookupTool` 未登记返回 false；`ToolForModel` 只认 `mediakit-` 前缀；路径由 `Sync` 决定前缀。
2. **客户端**：鉴权头正确；非 2xx 保留上游 body（回归 `UpstreamError`）；空 baseURL 回退默认域名；拒绝不以 `/` 开头的路径。
3. **分销**：同步/异步路由各自解析出 `mediakit-<tool>` 模型名；未登记工具 400；同步工具走异步路由 400（反之亦然）；三者都要覆盖 `../../etc/passwd` 一类的 hostile 输入。
4. **中继模式**：三个前缀有包含关系，顺序错了会把同步与查询吞成异步。
5. **时长提取**：毫秒字段换算成秒且优先于秒字段；缺字段 / 非数 / 零 / 负 / 字符串一律返回 0；超上界钳到 `MaxTaskDurationSeconds`。
6. **审计行**：字段全部从 task 推出；`charged_quota` 取轮询层传入值而非 `task.Quota`；非本渠道的 action 不落行。
7. **超分零回归**：`go test ./relay/channel/task/volcengine_plus/...`。

验证：`go test ./relay/... ./middleware/... ./router/... ./service/... ./model/...`；前端 `bun run typecheck`（`web/default/`）。

---

## 11. 明确不做

- 官方价表内置进用户计费（改价要发版）；`cost.go` 落地后只用于审计与默认定价参考。
- 同步工具批量 v1 一律 400：**上游真实的多产物倍率字段是 `batch_count`**（目前只有 `generate-product-scene-image` 有，官方按生成张数计费、回包为 `result.images[]`）。守卫在 `relay/helper/valid_request.go` 的 `GetAndValidateRequest` → `RelayFormatMediaKit` 分支（`gjson` 扫 `batch_count > 1` 返 400），**不在 `MediaKitSyncHelper` 里**——它必须排在 `controller.Relay` 的定价与预扣之前，落到 handler 阶段再拦已经来不及参与计价。透传路径没有 DTO，拿不到 `TokenCountMeta.BillingRatios` 钩子参与计价，所以不放开。
  早期文档写的守卫字段是 `n`——MediaKit 没有任何工具用这个字段名，按 `n` 写守卫等于不设防。
- 异步链路的同类守卫：**目前没有，这是一个已确认的少收面**。异步走 `controller.RelayTask`、不经过 `GetAndValidateRequest`，因此同步侧那道 `batch_count` 守卫**不覆盖异步提交**（已核实：`GetAndValidateRequest` 全仓只有 `controller/relay.go:111` 一个调用点，在 `Relay()` 内）。而异步侧确实存在等价的可控倍率字段——上一版文档断言「异步工具没有等价的可控倍率字段」是**错的**，实测有三个多产物字段：

| 工具 | 倍率字段 | 说明 |
|---|---|---|
| `drama-recap` | `batch_count` | 一次生成 N 个解说版本，结果在 `result.video_urls` 返回 N 条 |
| `drama-recap-vertical` | `max_count` | 范围 [1, 100]，**默认值 3**——不传也会产出多条 |
| `generate-highlights-*` 族 | 无 | 产出条数由视频内容决定，客户端改不出线性倍率 |

  上游会真的按倍数产出，网关只按**一次**计费。待补：在 `TaskAdaptor.ValidateRequestAndSetAction`（`task_adaptor.go:54`，目前只校验工具存在性与 JSON 形状）里加同样的 `> 1` 拒绝。**本期未做**——它会拒掉现有多产物调用，属对客行为变更。在此之前调用方需自行传 `1`。
  但存在口径差需留意：官方对抽帧是**按截图总张数**计费（`截图总张数 / 1000 × 0.1 元`；开启雪碧图时按合成雪碧图所用的帧截图张数，不按雪碧图文件数），网关登记为 `BillingPerCall` 按次收费。这不是漏洞而是定价取舍——管理员配 `model_price` 时应知道单个长视频可能产出上万张截图。
- 代理上游 `/api/v1/tasks/{id}`。
- 由客户端字符串拼上游路径。
- 媒体上传地址 API（`/api/v1/media/upload-address`）与事件回调配置。
- 改已有渠道 45 / 62 / 63 / 64 的默认行为。
- `TaskPricePatches` 命中 MediaKit 工具名时的特殊处理（`DurationSettlement` 已可放行，上线前确认生产取值即可）。

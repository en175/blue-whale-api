# 火山（增强）渠道

Seedance 生成可抽签降档，再用 MediaKit 超分回请求档。契约以词表和 ADR 为准，本文是渠道说明书。

- **术语：** [`CONTEXT.md`](../../CONTEXT.md)
- **ADR：**
  - [0011](../adr/0011-doubao-video-lottery-channel.md) 对客档位契约不同则新开渠道类型
  - [0012](../adr/0012-lottery-channel-quota-uses-upstream-tier.md) 渠道限额只计 Seedance 上游档生成
  - [0013](../adr/0013-enhance-costs-table.md) 超分花费落 `enhance_costs`
  - [0014](../adr/0014-enhance-retry-then-fail.md) 超分最多 3 次，耗尽失败
  - [0015](../adr/0015-mediakit-resolution-and-rate-card.md) MediaKit 传请求档 + 官方元/分钟价表
  - [0016](../adr/0016-charge-requested-tier-when-enhance-fails.md) 已出片但超分失败仍按请求档扣费
  - [0017](../adr/0017-video-token-peel-from-upstream.md) 对客 token 剥皮
- **上游：** [画质增强（大模型版）](https://docs.volcengine.com/docs/6448/2407223)

---

## 1. 需求

新增渠道类型 **火山（增强）**（`ChannelTypeVolcEngineEnhanced = 64`）。对客接口与豆包视频直连相同（OpenAI 兼容 `/v1/videos` + 任务式 `/v1/video/generations`）。内部：

1. **分辨率抽签**：仅 1080p / 720p 按渠道百分比降到更低上游档；未命中则走原生请求档。4k、480p 不抽签。
2. **超分回请求档**：仅当上游档 **低于** 请求档时，调用火山 AI MediaKit「画质增强（大模型版）」把成片拉回请求档。未降档不超分。
3. **对客口径**：`resolution` 与视频 token 按请求档；成功单的结果 URL 为超分成片。用户扣费按请求档倍率 × 剥皮后的视频 token。
4. **超分花费**：按 MediaKit 官方元/分钟表记人民币分，落独立表，不进用户 quota，不进渠道限额。

不接：Remix、把降档成片当成功单、用档位单价比伪装 token、独立时长公式重算 token、用即梦 HMAC 调 MediaKit、v1 超分报表页。

---

## 2. 术语

以 `CONTEXT.md` 为准，本渠道用到的：

| 术语 | 含义 |
|---|---|
| 请求档 | 用户请求的 `resolution`。对客计费、响应、超分目标都认它。 |
| 上游档 | 抽签后发给 Seedance 的档。只进 `private_data`、admin 日志、渠道限额。 |
| 分辨率抽签 | 每次提交独立随机；百分比为 0 等于该路径关闭。 |
| 计费画布 | 模型系列 × 档 × `ratio` 的宽×高。剥皮用像素比，不是成片文件像素。 |
| 视频 token | 未降档 = 回包 token；降档 = 回包 token × 请求档像素 ÷ 上游档像素。 |
| 超分 | MediaKit 画质增强（大模型版）。最多 3 次；耗尽失败不交片。 |
| 超分花费 | 成功单按官方元/分钟 × 输出时长，四舍五入到分。失败行金额 0。 |
| 渠道原始成本 | 只计 Seedance 上游档：上游原始 token × 上游档单价（去分组折扣）。不含超分花费。 |

---

## 3. 主链路

```
用户请求档
  ├─ 4k / 480p / 抽签未命中
  │     → 只打 Seedance（上游档 = 请求档）
  │     → 不超分
  │     → SUCCESS；对客 resolution / token = 回包（未剥皮）
  └─ 1080p 或 720p 且抽签命中（上游档 < 请求档）
        → Seedance 按上游档生成（对客仍 IN_PROGRESS）
        ├─ Seedance 失败 → FAILURE，全额退用户预扣
        └─ Seedance 成功
              → MediaKit 超分到请求档（最多 3 次，进度约 80%）
              ├─ 超分成功 → SUCCESS；结果 URL = 超分成片
              │              对客 resolution = 请求档；token = 剥皮值
              │              用户按请求档 × 剥皮 token 结算
              └─ 3 次皆失败 → FAILURE，不交成片
                               用户仍按请求档扣费（预扣不退）
```

Seedance 已成功、超分尚未完成时：对客仍是进行中。**不得**把上游档成片 URL、上游 `resolution`、上游原始 token 写入任何用户可见字段。

---

## 4. 抽签规则

配置在该渠道 `setting` JSON。每次提交抽 `[0, 100)` 一次，按前缀区间命中。

| 请求档 | 配置项 | 行为 |
|---|---|---|
| 1080p | `1080p_to_720p_percent`（N）、`1080p_to_480p_percent`（M） | `[0, N)` → 720p；`[N, N+M)` → 480p；其余保持 1080p |
| 720p | `720p_to_480p_percent`（P） | `[0, P)` → 480p；其余保持 720p |
| 4k | 无 | **不抽签**（MediaKit 最高出 2k，无法超分回 4k） |
| 480p | 无 | 不抽签 |

约束：`N ≥ 0`、`M ≥ 0`、`N+M ≤ 100`；`P ≥ 0` 且 `P ≤ 100`；缺省或 0 = 关闭。保存渠道时校验，违反则拒绝保存。

只改发给 Seedance 的 `resolution` 字符串。`ratio` / `duration` / 输入 content 不动。原始 `metadata.resolution`（请求档）不得改写。

---

## 5. 视频 token（剥皮）

不使用 `(输入时长+输出时长)×宽×高×帧率/1024` 独立重算。时长与帧率已含在 Seedance 回包 token 里。

```
未降档：视频 token = 回包 completion_tokens（或 total_tokens，与现有豆包提取一致）
降档：  视频 token = round(回包 token × 请求档画布宽 × 请求档画布高
                                ÷ (上游档画布宽 × 上游档画布高))
```

`round` 为四舍五入到整数。画布按 **同一模型系列、同一 `ratio`** 查 §5.1 表。缺 `ratio` 按 `16:9`。未知 `ratio` 提交时 **400**。模型名包含 `2.5` 走 2.5 表，否则走 2.0 系列。

例：16:9、回包 108000（720p）→ `108000 × (1920×1080) / (1280×720) = 243000`。

用户结算：剥皮后的视频 token × 请求档 `video_input` 倍率（现有 `video_tier` 表）。**禁止**用上游原始 token 做用户结算。渠道原始成本 **必须**用上游原始 token × 上游档单价。

配额换算走 `common.QuotaFromFloatChecked`，禁止裸 `int` 强转。

### 5.1 计费画布

#### 480p

| 宽高比 | Seedance 2.5 | Seedance 2.0 系列 |
|---|---|---|
| 16:9 | 854 × 480 | 864 × 496 |
| 4:3 | 752 × 560 | 752 × 560 |
| 1:1 | 640 × 640 | 640 × 640 |
| 3:4 | 560 × 752 | 560 × 752 |
| 9:16 | 480 × 854 | 496 × 864 |
| 21:9 | 992 × 432 | 992 × 432 |

#### 720p（所有模型）

| 宽高比 | 画布 |
|---|---|
| 16:9 | 1280 × 720 |
| 4:3 | 1112 × 834 |
| 1:1 | 960 × 960 |
| 3:4 | 834 × 1112 |
| 9:16 | 720 × 1280 |
| 21:9 | 1470 × 630 |

#### 1080p（fast/mini 不支持 1080p；用户请求了 1080p 时仍用本表剥皮）

| 宽高比 | Seedance 2.5 / 2.0 |
|---|---|
| 16:9 | 1920 × 1080 |
| 4:3 | 1664 × 1248 |
| 1:1 | 1440 × 1440 |
| 3:4 | 1248 × 1664 |
| 9:16 | 1080 × 1920 |
| 21:9 | 2206 × 946 |

#### 4k（仅 Seedance 2.0；不抽签、不超分、不剥皮）

| 宽高比 | Seedance 2.0 |
|---|---|
| 16:9 | 3840 × 2160 |
| 4:3 | 3326 × 2494 |
| 1:1 | 2880 × 2880 |
| 3:4 | 2494 × 3326 |
| 9:16 | 2160 × 3840 |
| 21:9 | 4398 × 1886 |

本表内置于适配器，不做渠道可配项。

---

## 6. 用户扣费与渠道限额

| 场景 | 用户 quota | 渠道原始成本 |
|---|---|---|
| Seedance 失败（无成片） | 全额退预扣 | 预记若已发生：退款不回滚（ADR 0006） |
| 未降档成功 | 请求档倍率 × 回包 token | 同口径（上游档 = 请求档） |
| 降档且超分成功 | 请求档倍率 × 剥皮 token | 上游原始 token × 上游档单价 |
| 降档且超分 3 次失败 | **仍按请求档扣费，预扣不退** | 上游原始 token × 上游档单价 |

预扣：`EstimateBilling` 读未改写的 `metadata.resolution`（请求档）查 `video_tier`。与豆包直连同一张档位价表。

结算：成功单用剥皮 token × 请求档倍率做差额（预扣按估算 token，结算按回包剥皮）。超分失败 **不要**走 `RefundTaskQuota` 全额退；保持请求档预扣为终价（若结算前已能用剥皮 token 更准，可按剥皮值差额结算，但不得退到上游档或 0）。

渠道 `DeductChannelQuota` 必须按 **上游档口径** 计，不能直接拿用户 1080 预扣去除以分组倍率。超分花费不进 `used_quota` / 周期限额 / 硬上限。

禁止：`AdjustBillingOnComplete` 把用户额度打回上游 720p/480p token 口径。

---

## 7. 超分（MediaKit）

仅「上游档 < 请求档」时进入。

| 项 | 值 |
|---|---|
| 提交 | `POST https://mediakit.cn-beijing.volces.com/api/v1/tools/enhance-video-generative` |
| 查询 | `GET https://mediakit.cn-beijing.volces.com/api/v1/tasks/{task_id}` |
| 鉴权 | `Authorization: Bearer {MediaKit API Key}`（不是豆包 ARK key，不是即梦 HMAC） |
| 请求体 | `video_url` = Seedance 结果直链；`resolution` = 请求档（**只允许 `720p` 或 `1080p`**） |
| 不传 | `fps`、`bitrate`（帧率跟 Seedance 片源，码率走 MediaKit 默认 medium） |
| 完成 | `status=completed`，`result.video_url` 为超分成片（默认 MP4，有效期 24h，落库后走现有结果 URL / 代理 URL） |

可选 `setting.mediakit_base_url` 覆盖默认域名。

**重试：** 只重试 MediaKit，不重跑 Seedance。固定 **3 次提交**（第 1 次 + 2 次重试），渠道不可配。三次皆失败 → 任务 FAILURE，不把上游档成片当成功单。

**状态机**（复用现有视频轮询，不新开任务表）：

1. Seedance `succeeded` 且需要超分：提交超分 job，`EnhanceJobID` 写入 `private_data`，任务保持 `IN_PROGRESS`，progress `80%`。
2. 轮询超分 job。`completed` → 写入超分结果 URL，SUCCESS。`failed` / 超时 → 若未满 3 次则再提交；满 3 次 → FAILURE。
3. 不需要超分：Seedance 成功即 SUCCESS，结果 URL 为 Seedance 直链（或代理 URL）。

超分进行中不得把 Seedance 720p/480p URL 写入对客 `ResultURL`；上游档成片 URL 只存在 `private_data` 内部字段。

对客失败原因泛化为处理失败，不暴露「实际打了 720p」。

---

## 8. 超分花费与 `enhance_costs`

口径：人民币，整数分，不对用户扣，不进渠道限额。只为后期 SQL 对账。v1 不做后台报表页。

**成功单：**

```
金额（分）= round(输出时长分钟 × 该档该帧率的「换算后单价（元/分钟）」× 100)
```

输出分辨率档 = 请求档（MediaKit 实际输出）。帧率 = Seedance 片源 fps（不传则回包 `framespersecond`，再缺省 24）。输出时长优先 Seedance/超分回包 duration（秒）/ 60。

官方价表（内置，不跟渠道配）：

| 输出分辨率 | 帧率 | 计费换算系数 | 换算后单价（元/分钟） |
|---|---|---|---|
| 720P（1280×720）及以下 | ≤ 30fps | 1 | 2.5 |
| | > 30 且 ≤ 60 | 2 | 5 |
| | > 60 且 ≤ 120 | 4 | 10 |
| 1080P（1920×1080）及以下 | ≤ 30fps | 2 | 5 |
| | > 30 且 ≤ 60 | 4 | 10 |
| | > 60 且 ≤ 120 | 8 | 20 |
| 2K（2560×1440）及以下 | ≤ 30fps | 4 | 10 |
| | > 30 且 ≤ 60 | 8 | 20 |
| | > 60 且 ≤ 120 | 16 | 40 |

v1 超分目标只有 720p / 1080p，2K 行代码带着不用。例：5 秒、1080p、≤30fps → `5/60 × 5 = 0.417 元` → 42 分。

**失败行：** 每次失败的 MediaKit 提交都插一行，`status=failed`，**金额 0**。job id 照样存。成功则再插（或更新最后一次为）成功行，按上表记分。三次失败 = 三行 failed、0 分。

表（三库可迁移，GORM AutoMigrate / SQLite `ADD COLUMN` 惯例）：一行一次超分尝试。

| 列 | 含义 |
|---|---|
| id | 主键 |
| created_at | 写入时间 |
| task_id | 公开任务ID |
| channel_id | 渠道ID |
| requested_resolution | 请求档 |
| upstream_resolution | 上游档 |
| output_duration_ms | 输出时长（毫秒，便于整数） |
| fps | 用于查价表的帧率 |
| rate_tier | 价表档（720p / 1080p / 2k） |
| amount_fen | 整数分；失败为 0 |
| mediakit_job_id | MediaKit 任务 ID |
| status | `succeeded` / `failed` |

`tasks.private_data` 只留当前 `EnhanceJobID` 做关联，不存金额。

---

## 9. 渠道配置与注册

### 9.1 类型

- `ChannelTypeVolcEngineEnhanced = 64`，插在 `ChannelTypeDummy` **之前**（当前 `HighwayAPI = 63`）。
- `ChannelBaseURLs[64] = "https://ark.cn-beijing.volces.com"`（生成走方舟，可被渠道 BaseURL 覆盖）。
- `ChannelTypeNames[64] = "VolcEngineEnhanced"`。
- `GetTaskAdaptor`：新类型返回本渠道 adaptor，**不要**并进 `ChannelTypeDoubaoVideo` 分支。
- `controller/channel-test.go` 的 `unsupportedTestChannelTypes` 追加 64。

前端：`CHANNEL_TYPES` / 展示序 / 类型配置加 64；i18n 全语言加 `"VolcEngineEnhanced": "火山（增强）"`。模型列表透传豆包 `ModelList`。

### 9.2 `setting` JSON

```json
{
  "1080p_to_720p_percent": 30,
  "1080p_to_480p_percent": 20,
  "720p_to_480p_percent": 10,
  "mediakit_api_key": "<MediaKit API Key>",
  "mediakit_base_url": ""
}
```

豆包 ARK key 仍走渠道 `key`。MediaKit key 不得写入用户可见日志。

保存校验：百分比约束见 §4；本类型若配置了非零降档百分比，则 `mediakit_api_key` 必填。

---

## 10. 适配器与轮询钩子

新建包 `relay/channel/task/volcengine_plus/`：

```go
type TaskAdaptor struct {
    taskdoubao.TaskAdaptor
}
```

| 方法 | 行为 |
|---|---|
| `BuildRequestBody` | 抽签后只改上游 payload 的 `resolution`；请求档留在 context / private_data。 |
| `EstimateBilling` | 委托内嵌实现（请求档）。 |
| `ParseTaskResult` / `ExtractTaskUsage` / `ConvertToOpenAIVideo` | 对客 resolution 与 token 用请求档 + 剥皮；URL 用超分后（或未降档时的 Seedance）结果。 |
| `AdjustBillingOnComplete` | 成功：剥皮 token × 请求档倍率。超分失败：不返回 0 去触发全额退，保持请求档扣费。 |
| `FetchTask` / `DoRequest` / URL | 委托豆包。 |

新增可选接口（`relay/channel/adapter.go`）：

```go
type TaskEnhancer interface {
    // EnsureEnhanced 在 Seedance 成片就绪后确保超分到请求档。
    // done=false：超分进行中或将重试，本轮不要标 SUCCESS。
    EnsureEnhanced(task *model.Task, upstreamResultURL string) (finalURL string, done bool, err error)
}
```

`service/task_polling.go` 在拿到生成结果 URL、写入对客 `ResultURL` **之前**调用。仅本渠道实现该接口。超分最终失败时标 FAILURE，且 **跳过** `RefundTaskQuota`。

### 10.1 `private_data`（`json:"-"`，用户 API 不返回）

在 `TaskPrivateData` 增加：

- `RequestedResolution`
- `UpstreamResolution`
- `UpstreamCompletionTokens` / `UpstreamTotalTokens`
- `EnhanceJobID`
- `EnhanceAttempt`（1–3）
- `EnhanceStatus`（`skipped` / `pending` / `succeeded` / `failed`）
- `EnhanceSourceURL`（上游档成片，仅内部）

Admin 日志 `admin_info` 带请求档、上游档、上游原始 token、剥皮 token、超分状态与尝试次数。

对客 `task.Data` 在落库前改写：`resolution` 与 `usage` 为请求档口径。

---

## 11. 错误与泄漏

| 场景 | 处理 |
|---|---|
| 百分比非法 | 保存渠道 400 |
| 未知 ratio | 提交 400 |
| 回包无 token 无法剥皮 | 任务失败 + ctx 日志（未降档且无 token 则保持现有豆包行为） |
| Seedance 失败 | 原样失败，全额退用户预扣 |
| 超分失败 / 超时，未满 3 次 | 再提交 MediaKit，对客仍 IN_PROGRESS |
| 超分 3 次失败 | 任务 FAILURE，不交片，用户按请求档扣费 |
| fast 不支持 1080 且抽签未降档 | 上游错误原样返回，全额退 |
| MediaKit 输入超规格 | 计入一次失败，按重试规则 |

用户可见出口不得出现上游档字符串、上游原始 token、上游档成片 URL：任务查询、OpenAI Video 转换、`usage`、用户用量日志。Admin、`private_data`、`enhance_costs` 除外。

超分成功后成片像素应对齐请求档（1080 或 720）。未抽中的请求与豆包直连一致。本渠道 v1 **不接** `/v1/videos/:id/remix`。

---

## 12. 实现落点

| 动作 | 路径 |
|---|---|
| 新建适配器 | `relay/channel/task/volcengine_plus/`（adaptor、抽签、画布、剥皮、MediaKit 客户端、价表、测试） |
| 渠道常量 | `constant/channel.go` |
| 注册 | `relay/relay_adaptor.go` |
| `TaskEnhancer` + 轮询扣费分支 | `relay/channel/adapter.go`、`service/task_polling.go` |
| private_data | `model/task.go` |
| `enhance_costs` 模型与迁移 | `model/enhance_cost.go`（新）、`model/main.go` AutoMigrate |
| 渠道 setting 校验 | `model/channel.go` / `dto/channel_settings.go`（本类型扩展） |
| 测试白名单 | `controller/channel-test.go` |
| 前端类型 + 表单 | `web/default/src/features/channels/`、`src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` |
| 表字典 | `docs/db-schema/docs/enhance_costs.md` |
| 术语 | `CONTEXT.md` |
| 决策 | `docs/adr/0011`–`0017` |

复用：`taskdoubao.TaskAdaptor`、`taskcommon.EstimateVideoTierBilling`、`common.QuotaFromFloatChecked`、`logger.Log*`（带 ctx）、现有结果 URL / 代理 URL。

---

## 13. 测试

表驱动，`testify`；抽签注入可重复 RNG（不要 `time.Now`）。

1. **抽签区间**：1080p 的 N/M 边界、N+M=100、全 0、720p 的 P；4k/480p 永不降档。
2. **请求体**：请求 1080p 且命中 720 → 上游 JSON `resolution=720p`，请求档仍为 1080p。
3. **剥皮**：16:9 回包 108000 → 243000；2.0 与 2.5 的 480p 画布不同则剥皮比不同；4:3 与 16:9 像素比不同。
4. **计费成功**：请求 1080、上游 720、超分成功 → 用户 1080 档 × 243000；渠道限额用 108000 × 720 档价。
5. **计费超分失败**：Seedance 成功、超分 3 次失败 → 用户不退预扣（请求档）；三行 `enhance_costs` 金额 0。
6. **Seedance 失败**：全额退用户预扣。
7. **超分**：降档会提交 MediaKit 且 `resolution` 为请求档；未降档不调用；失败会再试直至 3 次。
8. **泄漏**：用户向序列化 body 不含上游档、不含上游原始 token。
9. **保存校验**：N+M>100 拒绝；非零降档百分比缺 MediaKit key 拒绝。
10. **价表**：5s / 1080p / 24fps → 42 分。

验证：`go test ./relay/channel/task/volcengine_plus/... ./model/... ./service/...`；前端 `bun run typecheck`（`web/default/`）。

---

## 14. 明确不做

- 4k 抽签或超分到 4k / 2k。
- MediaKit 极速版 / 标准版；二次插帧。
- 超分失败后把上游档成片标 SUCCESS 交给用户。
- 超分失败全额退用户（Seedance 已出片时）。
- 独立时长公式重算对客 token；用档位单价比（约 1.11）伪装 token。
- 超分花费折进 `logs.quota` 或渠道限额。
- v1 超分对账 UI；Remix。
- 用即梦 `Sign` / `serviceName=cv` 调 MediaKit。
- 改豆包直连渠道（type 54 / 45）的默认行为。

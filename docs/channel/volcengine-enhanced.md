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
  - [0019](../adr/0019-seedance-draft-mode.md) Seedance 2.5 样片模式（本渠道不支持 draft）
  - [0020](../adr/0020-enhanced-channel-model-tier-and-4k.md) 4k 抽签三档、超分到 4k、按模型分档校验
- **上游：** [画质增强（大模型版）](https://docs.volcengine.com/docs/6448/2407223)

---

## 1. 需求

新增渠道类型 **火山（增强）**（`ChannelTypeVolcEngineEnhanced = 64`）。对客接口与豆包视频直连相同（OpenAI 兼容 `/v1/videos` + 任务式 `/v1/video/generations`）。内部：

1. **分辨率抽签**：1080p / 720p / 4k 按渠道百分比降到更低上游档；未命中则走原生请求档。480p 不抽签（无处可降）。
2. **超分回请求档**：仅当上游档 **低于** 请求档时，调用火山 AI MediaKit「画质增强（大模型版）」把成片拉回请求档。未降档不超分。超分目标可为 4k。
3. **按模型分档校验**：请求档超出模型能力时**提交期 400**，不等上游报错。4k 只允许 Seedance 2.0；fast / mini 不支持 1080p 与 4k；1.0 / 1.5 系列不在本渠道服务范围（见 §4.1）。
4. **对客口径**：`resolution` 与视频 token 按请求档；成功单的结果 URL 为超分成片。用户扣费按请求档倍率 × 剥皮后的视频 token。
5. **超分花费**：按 MediaKit 官方元/分钟表记人民币分，落独立表，不进用户 quota，不进渠道限额。
6. **不支持样片模式**：`draft` 请求（顶层 `draft` 或 `metadata.draft`）一律提交期 400 `draft_not_supported`。原因见 §4.2。

不接：Remix、把降档成片当成功单、用档位单价比伪装 token、独立时长公式重算 token、用即梦 HMAC 调 MediaKit、v1 超分报表页。

---

## 2. 术语

以 `CONTEXT.md` 为准，本渠道用到的：

| 术语 | 含义 |
|---|---|
| 请求档 | 用户请求的 `resolution`。对客计费、响应、超分目标都认它。 |
| 上游档 | 抽签后发给 Seedance 的档。只进 `private_data`、admin 日志、渠道限额。 |
| 分辨率抽签 | 每次提交独立随机；百分比为 0 等于该路径关闭。1080p / 720p / 4k 参与，480p 不参与。 |
| 模型系列 | 计费画布与档位能力的分组依据：Seedance 2.5、Seedance 2.0 系列（含 fast / mini）。1.0 / 1.5 不在本渠道。 |
| 计费画布 | 模型系列 × 档 × `ratio` 的宽×高。剥皮用像素比，不是成片文件像素。 |
| 视频 token | 未降档 = 回包 token；降档 = 回包 token × 请求档像素 ÷ 上游档像素。 |
| 超分 | MediaKit 画质增强（大模型版）。最多 3 次；耗尽失败不交片。 |
| 超分花费 | 成功单按官方元/分钟 × 输出时长，四舍五入到分。失败行金额 0。 |
| 渠道原始成本 | 只计 Seedance 上游档：上游原始 token × 上游档单价（去分组折扣）。不含超分花费。 |

---

## 3. 主链路

```
用户请求档
  ├─ 480p / 抽签未命中（含所有全 0 配置）
  │     → 只打 Seedance（上游档 = 请求档）
  │     → 不超分
  │     → SUCCESS；对客 resolution / token = 回包（未剥皮）
  └─ 1080p / 720p / 4k 且抽签命中（上游档 < 请求档）
        → Seedance 按上游档生成（对客仍 IN_PROGRESS）
        ├─ Seedance 失败 → FAILURE，全额退用户预扣
        └─ Seedance 成功
              → MediaKit 超分到请求档（最多 3 次，对客进度保持 50%）
              ├─ 超分成功 → SUCCESS；结果 URL = 超分成片
              │              对客 resolution = 请求档；token = 剥皮值
              │              用户按请求档 × 剥皮 token 结算
              └─ 3 次皆失败 → FAILURE，不交成片
                               用户仍按请求档扣费（预扣不退）
```

抽签命中的组合（4k 可降三档）：`4k→1080p` / `4k→720p` / `4k→480p` / `1080p→720p` / `1080p→480p` / `720p→480p`。

Seedance 已成功、超分尚未完成时：对客仍是进行中。**不得**把上游档成片 URL、上游 `resolution`、上游原始 token 写入任何用户可见字段。

---

## 4. 抽签规则

配置在该渠道 `setting` JSON。每次提交抽 `[0, 100)` 一次，按前缀区间命中。

| 请求档 | 配置项 | 行为 |
|---|---|---|
| 1080p | `1080p_to_720p_percent`（N）、`1080p_to_480p_percent`（M） | `[0, N)` → 720p；`[N, N+M)` → 480p；其余保持 1080p |
| 720p | `720p_to_480p_percent`（P） | `[0, P)` → 480p；其余保持 720p |
| 4k | `4k_to_1080p_percent`（A）、`4k_to_720p_percent`（B）、`4k_to_480p_percent`（C） | `[0, A)` → 1080p；`[A, A+B)` → 720p；`[A+B, A+B+C)` → 480p；其余保持 4k |
| 480p | 无 | 不抽签（无处可降） |

约束：`N ≥ 0`、`M ≥ 0`、`N+M ≤ 100`；`P ≥ 0` 且 `P ≤ 100`；`A ≥ 0`、`B ≥ 0`、`C ≥ 0` 且 `A+B+C ≤ 100`；缺省或 0 = 关闭。保存渠道时校验，违反则拒绝保存。

只改发给 Seedance 的 `resolution` 字符串。`ratio` / `duration` / 输入 content 不动。原始 `metadata.resolution`（请求档）不得改写。

`resolutionRank` 已含 4k，抽签命中后 `IsDowngrade` 自动成立，剥皮 / 超分 / 对客改写无需分支。

**注意 `4k→480p` 是 20 倍上采样。** 上游是否接受该组合未实测；建议该百分比先配 0，小流量验证后再开。

---

## 4.1 模型档位能力（提交期校验）

`ValidateRequestAndSetAction` 在预扣费之前校验，违反返回 **400 `invalid_resolution`**（draft 见 §4.2）。放在这一层的原因：`BuildRequestBody` 在预扣费之后，且其错误会被 `relay_task` 一律包成 500、错误码不可自定义。

| 模型系列 | 480p | 720p | 1080p | 4k |
|---|---|---|---|---|
| Seedance 2.5 | ✓ | ✓ | ✓ | ✗ |
| Seedance 2.0 | ✓ | ✓ | ✓ | ✓ |
| Seedance 2.0 fast | ✓ | ✓ | ✗ | ✗ |
| Seedance 2.0 mini | ✓ | ✓ | ✗ | ✗ |
| Seedance 1.0 / 1.5 | ✗ | ✗ | ✗ | ✗ |

- 系列判定：模型名（小写、去空白）含 `1-0` / `1.0` / `1-5` / `1.5` → 1.x（不支持）；含 `2.5` 或 `2-5` → 2.5；其余 → 2.0 系列（未知模型名沿用宽松默认）。
- fast / mini 判定：模型名含 `fast` 或 `mini`。
- 空 `resolution` 与未知名档位（如 `2k`）一律放行，交上游报错——本次不收紧。
- 校验用的是**映射前**的请求模型名（`ValidateRequestAndSetAction` 跑在 `helper.ModelMappedHelper` 之前）。本渠道无模型别名，对客名 = 上游名，故不构成问题。

400 不触发渠道重试（非 429 / 307 / 5xx），直接终止并返回明确错误码。

---

## 4.2 样片模式（draft）不支持

`metadata.draft=true` 在豆包适配器上零把关透传。若放行，Ark 会出 480p 样片，但本渠道对客 `resolution` 与扣费按请求档写死、`EstimateBilling` 读未改写的 `metadata.resolution`、剥皮又因未降档而跳过——结果是**按请求档全额扣费、API 回请求档、交付一段样片**。叠加抽签时上游还会同时收到 `draft=true` 与降档后的 `resolution`，Ark 行为未定义。

故提交期一律 **400 `draft_not_supported`**。判定入口两条都覆盖：顶层 `draft` 与 `metadata.draft`（bool 或字符串 `"true"`）。共用实现在 `taskcommon.IsDraftRequest`，渠道 66（Seedance 2.5）同样调用它。

---

## 5. 视频 token（剥皮）

不使用 `(输入时长+输出时长)×宽×高×帧率/1024` 独立重算。时长与帧率已含在 Seedance 回包 token 里。

```
未降档：视频 token = 回包 completion_tokens（或 total_tokens，与现有豆包提取一致）
降档：  视频 token = round(回包 token × 请求档画布宽 × 请求档画布高
                                ÷ (上游档画布宽 × 上游档画布高))
```

`round` 为四舍五入到整数。画布按 **同一模型系列、同一 `ratio`** 查 §5.1 表。缺 `ratio` 按 `16:9`。未知 `ratio` 提交时 **400**。模型名小写后含 `2.5` 或 `2-5` 走 2.5 表，否则走 2.0 系列表（fast / mini 与 2.0 像素相同）。

例：16:9、回包 108000（720p）→ `108000 × (1920×1080) / (1280×720) = 243000`。

用户结算：剥皮后的视频 token × 请求档 `video_input` 倍率（现有 `video_tier` 表）。**禁止**用上游原始 token 做用户结算。渠道原始成本 **必须**用上游原始 token × 上游档单价。

配额换算走 `common.QuotaFromFloatChecked`，禁止裸 `int` 强转。

### 5.1 计费画布

#### 480p

| 宽高比 | Seedance 2.5 | Seedance 2.0 系列（含 fast / mini） |
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

#### 1080p（fast/mini 不支持 1080p，已在 §4.1 提交期 400；本表供 2.5 / 2.0 使用）

| 宽高比 | Seedance 2.5 / 2.0 |
|---|---|
| 16:9 | 1920 × 1080 |
| 4:3 | 1664 × 1248 |
| 1:1 | 1440 × 1440 |
| 3:4 | 1248 × 1664 |
| 9:16 | 1080 × 1920 |
| 21:9 | 2206 × 946 |

#### 4k（仅 Seedance 2.0；§4.1 的档位能力已排除 2.5 / fast / mini）

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
| 请求体 | `video_url` = Seedance 结果直链；`resolution` = 请求档（**只允许 `720p` / `1080p` / `4k`**；480p 请求永不降档，不会是超分目标） |
| 不传 | `fps`、`bitrate`（帧率跟 Seedance 片源，码率走 MediaKit 默认 medium） |
| 完成 | `status=completed`，`result.video_url` 为超分成片（默认 MP4，有效期 24h，落库后走现有结果 URL / 代理 URL） |

可选 `setting.mediakit_base_url` 覆盖默认域名；可选 `setting.mediakit_submit_path` 覆盖默认提交路径 `/api/v1/tools/enhance-video-generative`。查询路径仍为 `/api/v1/tasks/{task_id}`。

**重试：** 只重试 MediaKit，不重跑 Seedance。固定 **3 次提交**（第 1 次 + 2 次重试），渠道不可配。三次皆失败 → 任务 FAILURE，不把上游档成片当成功单。

**状态机**（复用现有视频轮询，不新开任务表）：

1. Seedance `succeeded` 且需要超分：提交超分 job，`EnhanceJobID` 写入 `private_data`，任务保持 `IN_PROGRESS`，progress `50%`（不写 `80%`）。
2. 轮询超分 job。`completed` → 源链写入 `private_data.ResultURL` 供 `/content` 代理拉取，SUCCESS。`failed` / 超时 → 若未满 3 次则再提交；满 3 次 → FAILURE。
3. 不需要超分：Seedance 成功即 SUCCESS。对客 `result_url` / `content.video_url` 一律本站 `/v1/videos/{task_id}/content`；TOS/VOD 源链只存在 `private_data.ResultURL`，给代理拉片，不进用户或管理台 API。

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

| 表 | 档 | 行 |
|---|---|---|
| 1 | 720P（1280×720）及以下 | ≤ 30fps → 2.5；> 30 且 ≤ 60 → 5；> 60 且 ≤ 120 → 10 |
| 2 | 1080P（1920×1080）及以下 | ≤ 30fps → 5；> 30 且 ≤ 60 → 10；> 60 且 ≤ 120 → 20 |
| 3 | 2K（2560×1440）及以下 | ≤ 30fps → 10；> 30 且 ≤ 60 → 20；> 60 且 ≤ 120 → 40 |
| 4 | 4K（3840×2160）及以下 | ≤ 30fps → 20；> 30 且 ≤ 60 → 40；> 60 且 ≤ 120 → 80 |

原表给的是「换算系数」（1 系 = 2.5 元/分钟），代码里直接存系数 × 2.5 后的单价。第 3 表（2K）代码带着不用——本渠道不出 2k。4k 档价表已于 2026-09 补齐：官方 4k 系数为 8 / 16 / 32，折合 20 / 40 / 80 元/分钟。

例：5 秒、1080p、≤30fps → `5/60 × 5 = 0.417 元` → 42 分。5 秒、4k、≤30fps → `5/60 × 20 = 1.667 元` → 167 分。

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
| rate_tier | 价表档（`720p` / `1080p` / `2k` / `4k`） |
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

前端：`CHANNEL_TYPES` / 展示序 / 类型配置加 64；i18n 全语言加 `"VolcEngineEnhanced": "火山（增强）"`。模型列表用**本包自己的 `ModelList`**（比照渠道 66 的 `GetModelList()` 覆写），**不改 `task/doubao` 包**——那份列表渠道 45 / 54 也在用。

本包 `ModelList`：

```
doubao-seedance-2-5-260628
doubao-seedance-2-0-260128
doubao-seedance-2-0-fast-260128
doubao-seedance-2-0-mini-260615
```

不含 1.0 / 1.5 系列（§4.1 会在提交期 400）。

**这份列表不会自动出现在管理端模型下拉里。** 64 与 66 都没有 `common.ChannelType2APIType` 映射，`controller/model.go` 的 `channelId2Models` 与 `ChannelListModels` 都按 APIType 构建，不含任务视频渠道。要让新模型在本渠道真正可路由，需在渠道配置里填写模型名落 `abilities` 表——属运维动作。

### 9.2 `setting` JSON

```json
{
  "1080p_to_720p_percent": 30,
  "1080p_to_480p_percent": 20,
  "720p_to_480p_percent": 10,
  "4k_to_1080p_percent": 0,
  "4k_to_720p_percent": 0,
  "4k_to_480p_percent": 0,
  "mediakit_api_key": "<MediaKit API Key>",
  "mediakit_base_url": "",
  "mediakit_submit_path": ""
}
```

豆包 ARK key 仍走渠道 `key`。MediaKit key 不得写入用户可见日志。

保存校验：百分比约束见 §4；本类型若**六项中任一**降档百分比非零，则 `mediakit_api_key` 必填。

前端 `buildSettingJSON` 必须写入全部六个 `*_percent` 键，漏写会在保存一次后被清空。

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
| `ValidateRequestAndSetAction` | 在豆包实现之上加三条守卫：`remix_not_supported`、`draft_not_supported`（§4.2）、`invalid_ratio` / `invalid_resolution`（§4.1）。全部 400。 |
| `GetModelList` | 返回本包 `ModelList`（§9.1）。 |
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

对客任务/视频 API 不写请求档/上游档差、超分状态。管理台任务日志按 `enhance_costs` 显示超分徽章，并可筛「仅超分」（有花费行即算，含失败）。消费日志不写这些字段。

对客 `task.Data` 在落库前改写：`resolution` 与 `usage` 为请求档口径；成片 URL 为本站代理路径。

---

## 11. 错误与泄漏

| 场景 | 处理 |
|---|---|
| 百分比非法 | 保存渠道 400 |
| 未知 ratio | 提交 400 `invalid_ratio` |
| 请求档超出模型能力（4k 非 2.0、fast/mini 请求 1080p、1.0/1.5 系列） | 提交 400 `invalid_resolution`（§4.1） |
| draft 请求 | 提交 400 `draft_not_supported`（§4.2） |
| 回包无 token 无法剥皮 | 任务失败 + ctx 日志（未降档且无 token 则保持现有豆包行为） |
| Seedance 失败 | 原样失败，全额退用户预扣 |
| 超分失败 / 超时，未满 3 次 | 再提交 MediaKit，对客仍 IN_PROGRESS |
| 超分 3 次失败 | 任务 FAILURE，不交片，用户按请求档扣费 |
| MediaKit 输入超规格 | 计入一次失败，按重试规则 |
| 超分成功但金额算得 0（回包时长缺失、档位无价） | 记 0 + `LogWarn`（§8） |

`invalid_resolution` / `draft_not_supported` 是本地 400，不触发渠道重试（非 429 / 307 / 5xx）。此前「fast 不支持 1080 且抽签未降档」会走上游报错再全额退，现在改为提交期 400——客户端不再看到上游那句具体报错，这是**对客行为变更**。

用户与管理台 API 都不得出现上游档字符串、上游原始 token、上游档/超分成片直链、超分进度（`80%`）：任务查询、OpenAI Video 转换、`usage`、用量日志。TOS/VOD 源链与超分痕迹只在 `private_data`、`enhance_costs`。

超分成功后成片像素应对齐请求档（4k / 1080 / 720）。未抽中的请求与豆包直连一致。本渠道 v1 **不接** `/v1/videos/:id/remix`。

---

## 12. 实现落点

| 动作 | 路径 |
|---|---|
| 新建适配器 | `relay/channel/task/volcengine_plus/`（adaptor、模型档位、抽签、画布、剥皮、MediaKit 客户端、价表、测试） |
| 渠道常量 | `constant/channel.go` |
| 注册 | `relay/relay_adaptor.go` |
| `TaskEnhancer` + 轮询扣费分支 | `relay/channel/adapter.go`、`service/task_polling.go` |
| private_data | `model/task.go` |
| `enhance_costs` 模型与迁移 | `model/enhance_cost.go`（新）、`model/main.go` AutoMigrate |
| 渠道 setting 校验 | `model/channel.go` / `dto/channel_settings.go`（本类型扩展） |
| draft 判定（与渠道 66 共用） | `relay/channel/task/taskcommon/draft.go` |
| 测试白名单 | `controller/channel-test.go` |
| 前端类型 + 表单 | `web/default/src/features/channels/`、`src/i18n/locales/{en,zh,zh-TW,fr,ja,ru,vi}.json` |
| 表字典 | `docs/db-schema/docs/enhance_costs.md` |
| 术语 | `CONTEXT.md` |
| 决策 | `docs/adr/0011`–`0017`、`0019`、`0020` |

复用：`taskdoubao.TaskAdaptor`、`taskcommon.EstimateVideoTierBilling`、`common.QuotaFromFloatChecked`、`logger.Log*`（带 ctx）、现有结果 URL / 代理 URL。

---

## 13. 测试

表驱动，`testify`；抽签注入可重复 RNG（不要 `time.Now`）。

1. **抽签区间**：1080p 的 N/M 边界、N+M=100、全 0、720p 的 P；4k 的 A/B/C 边界、A+B+C=100、全 0、负值归 0；480p 永不降档。
2. **模型档位**：`SeedanceSeriesOf` 覆盖 2.5（点号与连字符两种写法）、2.0 / fast / mini、1.0 / 1.5、未知名；`ResolutionSupported` 覆盖 4k 仅 2.0、fast/mini 无 1080p、1.x 全拒、未知名档位放行。
3. **提交期守卫**：4k + 非 2.0 → 400 `invalid_resolution`；fast/mini + 1080p → 400；draft（顶层与 metadata 两路）→ 400 `draft_not_supported`；`ModelList` 不含 1.0 / 1.5，且未污染 `task/doubao` 的列表。
4. **请求体**：请求 1080p 且命中 720 → 上游 JSON `resolution=720p`，请求档仍为 1080p；请求 4k 且命中 1080 → 上游 1080p、请求档 4k、`metadata.resolution` 不动。
5. **剥皮**：16:9 回包 108000 → 243000；4k 请求按上游档 1080p / 720p / 480p 分别 → 432000 / 972000 / 2090323；2.0 与 2.5 的 480p 画布不同则剥皮比不同；4:3 与 16:9 像素比不同。
6. **计费成功**：请求 1080、上游 720、超分成功 → 用户 1080 档 × 243000；渠道限额用 108000 × 720 档价。
7. **计费超分失败**：Seedance 成功、超分 3 次失败 → 用户不退预扣（请求档）；三行 `enhance_costs` 金额 0。
8. **Seedance 失败**：全额退用户预扣。
9. **超分**：降档会提交 MediaKit 且 `resolution` 为请求档（含 4k）；未降档不调用；失败会再试直至 3 次；4k 成功行 `rate_tier=4k`。
10. **泄漏**：用户向序列化 body 不含上游档、不含上游原始 token。
11. **保存校验**：N+M>100 拒绝；A+B+C>100 拒绝；负值拒绝；任一非零百分比缺 MediaKit key 拒绝。
12. **价表**：5s / 1080p / 24fps → 42 分；5s / 4k / 24fps → 167 分；4k 各帧率档的单价边界。
13. **共享守卫**：`taskcommon.IsDraftRequest` 表驱动覆盖 nil、顶层 draft、`metadata.draft` 的 bool / 字符串 / 其它类型；渠道 66 全量测试保持通过（`isDraftRequest` 上移后零回归）。

验证：`go test ./relay/channel/task/volcengine_plus/... ./model/... ./service/...`；前端 `bun run typecheck`（`web/default/`）。

---

## 14. 明确不做

- 超分到 2k。
- 480p 抽签（无处可降）。
- 样片（draft）模式接入；2.5 的样片走渠道 66。
- MediaKit 极速版 / 标准版；二次插帧。
- 超分失败后把上游档成片标 SUCCESS 交给用户。
- 超分失败全额退用户（Seedance 已出片时）。
- 独立时长公式重算对客 token；用档位单价比（约 1.11）伪装 token。
- 超分花费折进 `logs.quota` 或渠道限额。
- v1 超分对账 UI；Remix。
- 用即梦 `Sign` / `serviceName=cv` 调 MediaKit。
- 改豆包直连渠道（type 54 / 45）的默认行为与模型列表。
- 模型别名（对客名 = 上游名）。

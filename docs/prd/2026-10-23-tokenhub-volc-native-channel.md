# V待确认【视频渠道】华鲲上游火山原生协议渠道与素材库接口

| 项目 | 内容 |
|---|---|
| 文档类型 | 标准 PRD（涉及新增对外协议、计费、渠道路由、前端） |
| 状态 | 待评审 |
| 版本 | v0.1 |
| 日期 | 2026-10-23 |
| 产品负责人 | 待指定 |
| 研发负责人 | 待指定 |
| 测试负责人 | 待指定 |
| 目标产品版本 | **待确认** |
| 关联业务父需求 | **待建（Gitee）** |
| 关联开发子任务 | 待建（前端 / 后端 / 测试） |
| 影响范围 | `blue-whale-api` 渠道注册、视频任务链路、计费、素材库代理、`web/default` 渠道配置页 |

> 评审前置：本文档需产品确认分类、范围、目标版本、模型清单和验收口径后，才可进入业务代码修改。PRD 未确认前只做只读调研。

---

## 1. 背景与目标

### 1.1 现状

网关已有三个与火山/华鲲相关的视频渠道，对客接口全部是 OpenAI 形状 `/v1/videos`：

| 渠道 | 上游 | 承载模型 | 对客形状 |
|---|---|---|---|
| 61 TokenHub | 华鲲 ai-tokenhub | `dreamina-seedance-2-0-*` | `/v1/videos` |
| 66 VolcEngine25 | 火山方舟 | `doubao-seedance-2-5-260628` | `/v1/videos` |
| 67 TokenHub25 | 华鲲 ai-tokenhub | `dreamina-seedance-2-5-260628` | `/v1/videos` |

问题：下游如果手里是火山方舟官方文档，无法直接接入。必须把 `content[]` 改写成 `prompt` + `metadata`，响应也要按 OpenAI 视频对象重新解析。这不满足“按火山官方文档直接接入”的诉求。

### 1.2 目标

1. 新增渠道类型 **70（TokenHubVolc）**，上游仍是华鲲，对客请求体、响应体、URL 形状与火山方舟 `POST /api/v3/contents/generations/tasks` 一致，覆盖 Seedance 2.0 与 2.5。
2. 新增一套素材库对客接口，形状与火山管控面 API 一致：`POST /?Action=X&Version=2024-01-01`，成功返回 `ResponseMetadata` + `Result`，错误返回 `ResponseMetadata.Error`。
3. 不改变现有 61 / 66 / 67 渠道与 `/api/material/{Action}` 的对外行为。

### 1.3 非目标

- 不改写上游华鲲的 `/api/v2` 协议；只在网关内做形状转换。
- 不引入新的上游供应商、新的模型能力。
- 不实现火山视频的任务列表、取消任务接口（本期只做创建 + 查询单个）。
- 不改变现有渠道 61/66/67 的计费公式和已外发接口契约。
- 不把火山 AK/SK 交给下游；素材库仍由网关注入凭证。
- 不做素材库的跨租户隔离改造（仍为单租户共享凭证 + 网关侧归属表）。

---

## 2. 已确认的产品决策

| # | 决策 | 结论 |
|---|---|---|
| D1 | 上游 | 华鲲 ai-tokenhub；火山形状由网关转换 |
| D2 | 对客视频路径 | `POST/GET /api/v3/contents/generations/tasks[/{id}]`，鉴权仍用网关 `sk-` |
| D3 | 任务 ID | 对客一律使用网关公开任务 ID；`draft_task.id` 也只接受公开 ID |
| D4 | 渠道粒度 | 一个渠道类型 70，同时承载 2.0 全系与 2.5 |
| D5 | 模型名 | 对客使用火山官方名 `doubao-*`，通过渠道 `model_mapping` 映射到华鲲 `dreamina-*`，不改代码 |
| D6 | 样片模式 | 只有 2.5 可开启；其它模型带 `draft` 直接拒绝；第二步固定 1080p，忽略客户端传值 |
| D7 | 计费 | 沿用现有视频档位价格表 + 完成后按上游 usage 的 token 结算 |
| D8 | 响应保真 | 对齐火山字段，能从落库数据回填的回填，拿不到就省略 |
| D9 | 错误格式 | `/api/v3` 下统一火山错误信封 |
| D10 | 路径隔离 | `/api/v3` 只路由类型 70；`/v1/videos` 的渠道选择排除类型 70 |
| D11 | 素材库路径 | 主路径 `POST /?Action=...&Version=...`，别名 `POST /api/ark?...` |
| D12 | 素材库鉴权 | 网关 `sk-`；`Version` 与其余 query 原样透传上游，缺省补 `2024-01-01` |
| D13 | 素材库行为 | 表内 Action 做归属校验，表外透传；保留回调改写；去掉本地列表、结果缓存、名称过滤 |
| D14 | 旧接口 | `/api/material/{Action}` 原样保留，两套并存 |
| D15 | 租户一致性 | 文档与管理端界面提示，不做运行时拦截 |

---

## 3. 前置条件

| # | 前置条件 | 是否具备 | 缺失影响 | 补齐人 |
|---|---|---|---|---|
| P1 | 目标产品版本 | 未知 | 无法确定提测与发布窗口 | 产品 |
| P2 | 业务父需求与 PRD 评审通过 | 待确认 | 不能进入业务代码修改 | 产品 |
| P3 | 华鲲账号已开通 Seedance 2.0 / 2.5 且余额满足开通条件 | 待确认 | 渠道无法实际提交任务 | 运维 / 产品 |
| P4 | 素材库凭证与华鲲视频 key 同属一个账号 | 待确认 | `asset://` 素材引用解析失败 | 运维 |
| P5 | 2.5 的 1080p / 1080p_video 档位价格 | 待确认 | 成片按 480p 基准价静默少收 | 产品 / 财务 |
| P6 | 反代允许 `POST /` 转发到网关 | **已确认支持** | — | 运维 |

---

## 4. 用户与权限

| 角色 | 权限 |
|---|---|
| 下游 API 客户端 | 使用网关 API key（`Bearer sk-`）调用 `/api/v3` 与素材库根路径；只能查询/引用自己创建的任务与素材组 |
| 管理员 | 在渠道配置页创建类型 70 渠道，配置 key、`model_mapping`、档位价格 |
| 普通用户 | 与现有视频渠道一致，受分组与额度限制 |
| 上游凭证 | 仅网关持有；下游不可见 |

---

## 5. 需求详述

### 5.1 渠道 70

- 渠道类型名：`TokenHubVolc`，编号 70。
- 默认 Base URL：`https://ai-tokenhub.ai`。
- 对客模型名只影响路由与计费，不要求与火山官方逐字一致；由管理员按定价配置，并通过 `model_mapping` 映射到华鲲名。示例：
  - `doubao-seedance-2-0-260128` → `dreamina-seedance-2-0-260128`
  - `doubao-seedance-2-0-fast-260128` → `dreamina-seedance-2-0-fast-260128`
  - `doubao-seedance-2-0-mini-260615` → `dreamina-seedance-2-0-mini-260615`
  - `doubao-seedance-2-5-260628` → `dreamina-seedance-2-5-260628`
- 上表只是示例，不是代码硬编码；新增模型或改名只改渠道配置，不改代码。
- `model_mapping` 配置示例：

```json
{
  "doubao-seedance-2-5-260628": "dreamina-seedance-2-5-260628",
  "doubao-seedance-2-0-260128": "dreamina-seedance-2-0-260128"
}
```

- 类型 70 不参与渠道联通性测试（与 66 一致）。

### 5.2 视频创建接口

```
POST /api/v3/contents/generations/tasks
Authorization: Bearer sk-xxx
Content-Type: application/json
```

请求体字段：

| 字段 | 类型 | 必选 | 说明 |
|---|---|---|---|
| `model` | string | 是 | 对客模型名 |
| `content` | array | 是 | 输入内容，见下表 |
| `resolution` | string | 否 | `480p` / `720p` / `1080p` / `4k` |
| `ratio` | string | 否 | 如 `16:9` / `adaptive` |
| `duration` | int | 否 | 秒；编辑/延长场景用 `-1` |
| `seed` | int | 否 | 随机种子 |
| `draft` | bool | 否 | 样片模式，仅 2.5 可用 |
| `watermark` | bool | 否 | 水印 |
| `generate_audio` | bool | 否 | 是否生成音频 |
| `camera_fixed` | bool | 否 | 固定镜头 |
| `frames` | int | 否 | 帧数 |
| `output_format` | string | 否 | `mp4` / `mov` |
| `callback_url` | string | 否 | 完成回调地址 |
| `service_tier` | string | 否 | 服务等级 |
| `execution_expires_after` | int | 否 | 超时阈值 |
| `priority` | int | 否 | 执行优先级 |
| `safety_identifier` | string | 否 | 用户标识 |
| `return_last_frame` | bool | 否 | 返回尾帧 |
| `tools` | array | 否 | 如 `web_search` |

`content[]` 项：

| 类型 | 结构 | 说明 |
|---|---|---|
| 文本 | `{"type":"text","text":"..."}` | 提示词 |
| 图片 | `{"type":"image_url","image_url":{"url":"..."},"role":"..."}` | URL / Base64 / `asset://<id>` |
| 视频 | `{"type":"video_url","video_url":{"url":"..."},"role":"..."}` | 参考视频 |
| 音频 | `{"type":"audio_url","audio_url":{"url":"..."}}` | 参考音频 |
| 样片 | `{"type":"draft_task","draft_task":{"id":"task_xxx"}}` | 仅第二步 |

成功响应：

```json
{"id": "task_xxx"}
```

`id` 为网关公开任务 ID，用于查询与后续 `draft_task` 引用。

### 5.3 样片模式（仅 2.5）

**第一步：生成 480p 样片**

- 请求带 `draft: true`。
- 模型必须是 2.5；其它模型 → 400 `invalid_draft_model`。
- `resolution` 只允许 `480p` 或省略；传 `720p` / `1080p` / `4k` → 400 `invalid_resolution`。
- 上游实际收到 `draft=true` + `resolution=480p`。

**第二步：生成成片**

- 请求带 `content[]` 中 `type="draft_task"` 的项，`draft_task.id` 为第一步返回的公开任务 ID。
- 校验（全部在预扣费之前）：
  1. 任务是本人的 → 否则 400 `task_not_exist`
  2. 原任务 `draft=true` → 否则 400 `origin_not_draft`
  3. 原任务已成功 → 否则 409 `origin_task_not_finished`
  4. 提交时刻在 7 天内 → 否则 400 `origin_draft_expired`
  5. 原任务有上游任务 ID → 否则 400 `origin_upstream_id_missing`
- 上游 `draft_task.id` 只来自网关落库的上游 ID，不接受客户端直接传上游 ID。
- 上游成片**只支持 1080p**；网关固定 `resolution=1080p`，客户端传任何值都被覆盖，不报错。
- 成片不继承样片的时长/尺寸等倍率，只继承“是否含视频输入”。

### 5.4 视频查询接口

```
GET /api/v3/contents/generations/tasks/{id}
Authorization: Bearer sk-xxx
```

成功响应字段：

```json
{
  "id": "task_xxx",
  "model": "doubao-seedance-2-5-260628",
  "status": "queued | running | succeeded | failed | cancelled | expired",
  "content": {"video_url": "https://..."},
  "usage": {"completion_tokens": 0, "total_tokens": 0},
  "created_at": 0,
  "updated_at": 0,
  "resolution": "1080p",
  "ratio": "16:9",
  "duration": 5,
  "seed": 0,
  "error": {"code": "...", "message": "..."}
}
```

- 查不到或非本人 → 400 `task_not_exist`。
- `resolution` / `ratio` / `duration` / `seed` 等字段能从落库数据回填的才返回，拿不到直接省略，不返回零值占位。
- 失败任务返回 `error` 对象。

### 5.5 错误格式

`/api/v3` 下所有 4xx / 5xx 统一：

```json
{"error": {"code": "InvalidParameter", "message": "...", "param": "", "type": "BadRequest"}}
```

- 网关主动校验失败使用火山错误码语义（`InvalidParameter` / `InvalidRequest` / `NotFound` 等）。
- 上游错误透传时保留上游 message，code 做映射；不把上游内部任务 ID 或签名信息暴露给下游。

### 5.6 计费

- 预扣：按 `resolution` 档位 × 是否含视频输入查 `video_tier_pricing`，得到倍率。
- 结算：任务完成后按上游 `usage` 的 token 数结算，与 66/67 一致。
- 样片：按 480p 档计费；成片：按 1080p 档 + 继承样片的“含视频输入”维度。上游成片档位只有 1080p，因此不存在其它成片档位的价格配置需求。
- 缺少档位价格配置时不得静默按基准价结算；需在管理员配置页提示 2.5 必须配齐 `1080p` 与 `1080p_video`。

### 5.7 渠道隔离

- 请求路径以 `/api/v3/` 开头 → 只允许选择渠道类型 70。
- 请求路径属于 `/v1/*` 视频接口 → 渠道选择排除类型 70。
- 隔离失败时返回明确错误，不能把火山形状的请求体发给 OpenAI 形状的适配器。

### 5.8 素材库接口

```
POST /?Action=CreateAsset&Version=2024-01-01
POST /api/ark?Action=CreateAsset&Version=2024-01-01   （别名）
Authorization: Bearer sk-xxx
Content-Type: application/json
```

- `Action` 必传；缺失 → 400 火山错误。
- `Version` 完全透传：下游传什么上游就收到什么；未传时补默认 `2024-01-01`。上游拒绝时原样返回官方错误码。
- 其余 query 参数同样透传，网关只负责补 `Action`。
- 上游 provider 沿用素材库全局配置（`tokenhub` / `volcengine`），凭证由网关注入。
- 成功响应保留 `{"ResponseMetadata":...,"Result":...}`；会话类接口（`CreateVisualValidateSession` / `GetVisualValidateResult`）在上游不包信封时由网关补 `ResponseMetadata` / `Result`。
- 错误响应：上游华鲲返回的顶层 `ResponseError` 由网关搬迁为 `ResponseMetadata.Error`（火山形状），`Result` 与其余字段原样保留。

```json
{"ResponseMetadata": {"Error": {"Code": "...", "Message": "..."}, "RequestId": "..."}}
```

支持的 Action 与网关行为：

| Action | 网关行为 |
|---|---|
| `CreateAssetGroup` | 归属记录 + 归属校验 |
| `GetAssetGroup` | 归属校验 |
| `UpdateAssetGroup` | 归属校验 |
| `DeleteAssetGroup` | 归属校验 |
| `ListAssetGroups` | 查本地归属表（按 API key 过滤），不查上游 |
| `CreateAsset` | 目标组归属校验 |
| `GetAsset` | 先 `GetAsset` 反查 `GroupId`，校验组归属后再返回；探测响应直接复用 |
| `UpdateAsset` | 先 `GetAsset` 反查 `GroupId`，校验组归属后再透传 |
| `DeleteAsset` | 先 `GetAsset` 反查 `GroupId`，校验组归属后再透传 |
| `ListAssets` | GroupIds 归属校验；空 GroupIds 拒绝 |
| `CreateVisualValidateSession` | 客户 `CallbackURL` 替换为网关回调，认证后 302 回客户 |
| `GetVisualValidateResult` | 先校验会话属于当前 API key；通过后按会话记录的 provider 查上游（不读本地缓存），并补建会话时的 `ProjectName` |
| 其它 Action | 原样透传上游 |

- 分页只支持页码模式：`PageNumber` + `PageSize` 原样透传（火山同样支持这两种参数）。`MaxResults` / `NextToken` 不做映射，上游会忽略。火山要求 `PageSize * PageNumber > 20000` 时改用 `NextToken` 深翻页，本期不支持。
- 名称过滤（`<` `>`、255 字）本期在新接口**不做**；旧接口保持现状。
- 单素材 `GetAsset` / `UpdateAsset` / `DeleteAsset` 的归属校验：先调上游 `GetAsset` 取 `GroupId`，再复用组归属校验；`GetAsset` 直接复用探测响应，不重复读取。新旧两套接口共用同一个 helper，旧接口下越权操作会从“成功”变为 404。
- 该路径已在 tokenhub 与 volcengine 两个 provider 的单测中覆盖；若上游不返回 `GroupId`，按拒绝处理。
- 归属字段按大小写不敏感读取，与上游 JSON 解码口径一致；同名字段大小写冲突时 fail-closed。
- **2026-10-26 修订（IKQFXR）：** 兼容入口 `ListAssetGroups` 改为查本地归属表；`GetAsset` 两个入口补归属校验；`GetVisualValidateResult` 补会话归属校验、按会话 provider 路由并补 `ProjectName`；本地列表失败也返回对应错误信封。安全边界优先于「行为与上游一致」。
- 30 秒上游超时沿用。
- 限流沿用 `GlobalAPIRateLimit`（按 ClientIP，默认 360 次 / 180 秒），不新增独立配额；根路径与 `/api/ark` 都必须显式挂载，不能依赖 `/api` 组的继承。

### 5.9 素材引用

- 生成任务中的图片/视频/音频均可使用 `asset://<ASSET_ID>`。
- 前提：素材库凭证与视频渠道 key 同属一个账号。管理端在素材库配置页与渠道配置页展示该提示。
- 网关不转写 `asset://`，原样透传上游。

---

## 6. 页面与交互

- 无新增业务页面。
- 前端渠道配置页需新增类型 70：
  - 渠道类型下拉可选 `TokenHubVolc`
  - 标签映射显示为火山系
  - 可编辑 `model_mapping` 与档位价格
- 素材库管理页新增一条提示：视频走华鲲渠道时，素材库 provider 建议选 `tokenhub`。

---

## 7. 异常与边界

| 场景 | 期望 |
|---|---|
| 非 2.5 模型带 `draft=true` | 400 `invalid_draft_model`，预扣费之前 |
| 样片请求 `resolution=720p` | 400 `invalid_resolution`，预扣费之前 |
| 成片请求 `draft_task.id` 不存在或非本人 | 400 `task_not_exist` |
| 成片请求原任务未完成 | 409 `origin_task_not_finished` |
| 样片超过 7 天 | 400 `origin_draft_expired` |
| 成片请求客户端传 `resolution=720p` | 忽略，按 1080p 提交与计费 |
| 上游华鲲超时 / 5xx | 返回火山错误信封；不自动重试成片请求（防重复扣费） |
| 同一份样片重复 finalize | 会生成两段成片、扣两次费（与 66/67 已知限制一致，本期不解决） |
| 素材库 `Action` 缺失 | 400 火山错误 |
| 素材库 `Version` 传错 | 原样透传上游；上游失败时原样返回官方错误码 |
| 素材库引用他人 `GroupId` | 404 / 400 火山错误，不泄露资源是否存在 |
| 根路径异常不可达 | 使用 `/api/ark` 别名，接口行为完全一致 |

---

## 8. 验收场景

| # | 场景 | 预期 |
|---|---|---|
| A1 | 2.0 文生视频，`POST /api/v3/.../tasks` | 返回 `{"id":"task_xxx"}`；查询到 `succeeded` 与 `video_url` |
| A2 | 2.5 图生视频，`content` 含 `image_url` | 上游收到 `content[]` 与 `role` 原样；计费含对应档位 |
| A3 | 2.5 样片第一步 `draft=true` | 返回任务 ID；查询到 480p 结果 |
| A4 | 2.5 样片第二步 `draft_task.id` | 生成 1080p 成片；计费按 1080p 档 |
| A5 | 2.0 模型带 `draft=true` | 400 `invalid_draft_model` |
| A6 | 样片第一步 `resolution=1080p` | 400 `invalid_resolution` |
| A7 | 成片第二步传 `resolution=720p` | 成功，上游收到 1080p |
| A8 | 查询他人任务 ID | 400 `task_not_exist` |
| A9 | `/v1/videos` 请求命中模型同时配在 66 和 70 | 选中 66，不发给 70 |
| A10 | `/api/v3` 请求命中模型同时配在 66 和 70 | 选中 70 |
| A11 | 素材库 `CreateAssetGroup` | 返回 `ResponseMetadata` + `Result`；组归属被记录 |
| A12 | 素材库 `GetAssetGroup` 查他人组 | 火山错误信封，不返回组内容 |
| A13 | 素材库 `Version=2023-01-01` | 上游收到 `Version=2023-01-01`，按其返回结果原样回给下游 |
| A14 | 素材库未知 Action | 原样透传上游并返回上游结果 |
| A15 | `CreateVisualValidateSession` | 上游收到网关回调地址；认证完成后 302 回客户 |
| A16 | `/api/material/{Action}` 旧接口回归 | 行为与改造前一致 |
| A17 | `GetAsset` / `UpdateAsset` / `DeleteAsset` 操作他人素材 | 火山错误信封 404，不泄露素材是否存在 |
| A18 | 旧接口 `GetAsset` / `UpdateAsset` / `DeleteAsset` 操作他人素材 | 404，与 A17 共用同一 helper |
| A19 | `GetAsset` 查询他人素材 | 404，与 A17 同一 helper |

---

## 9. 开放问题

1. 目标产品版本与发布窗口。

已确认、不再作为开放问题：

- 对客模型名不要求与火山逐字一致，全部通过 `model_mapping` 解决。
- 2.5 成片上游只支持 1080p，价表只需 `1080p` 与 `1080p_video` 两档。
- 素材库接口对外暴露 `POST /`，根路径为正式入口；`/api/ark` 仅作异常部署时的别名。
- 素材库不单独限流，沿用网关全局 API 限流（按 ClientIP，默认 360 次 / 180 秒）；根路径必须显式挂载该中间件，不能裸奔。
- 单素材保护范围：`GetAsset` + `DeleteAsset` + `UpdateAsset`（先 `GetAsset` 反查组归属），新旧接口共用同一 helper。

---

## 10. 目标版本与里程碑

| 阶段 | 内容 | 版本 | 状态 |
|---|---|---|---|
| PRD 评审 | 本文档 | 待确认 | 待评审 |
| 后端开发 | 渠道 70、视频转换、素材库新接口 | 待确认 | 未开始 |
| 前端开发 | 渠道类型 70、配置页提示 | 待确认 | 未开始 |
| 测试 | A1–A19 | 待确认 | 未开始 |
| 发布 | 测试环境 → 生产 | 待确认 | 未开始 |

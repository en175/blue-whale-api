# 素材库（Material）章节 · 设计文档

- **日期**: 2026-08-27
- **仓库**: `blue-whale-api`（`https://gitee.com/hangzhou-xuanfu_1/blue-whale-api.git`）
- **范围**: `web/api-docs/content/docs/{zh,en}/` 新增 `material/` 章节 + 根导航 meta 注册
- **状态**: 已批准（用户确认：方案 B 全量 12 个 Action / 方案 A 顶层章节 / 中英双语 / 以代码+ADR 实操为准）

---

## 背景

### 现状

- api-docs 站（Fumadocs/Next，中英双语，`web/api-docs/content/docs/{zh,en}/`）目前**没有独立的素材库文档章节**。
- 素材能力只在「视频生成 API」里顺带提到 `asset://` 引用与 `metadata.content` 参考素材，没有可给下游开发者直接对接的接口文档。
- 已有参考文档 `临时文件/素材模块-对外API接口文档.md`（v1.0），但**已过时**：Base URL 写的 `43.156.28.30:3000`、漏列 `ListAssetGroups`（号称 11 个 Action 实为 12 个）、`ListAssets.Filter.GroupIds` 标「否」（实际必填）、归属隔离规则未提及。

### 代码/ADR 事实源（已核对）

- 路由 `router/api-router.go`：对外 `/api/material/*`（TokenAuth，API key）；管理端 `/api/materials/*`（UserAuth）；回调 `/v1/material/h5-callback`（公开）。
- 服务 `service/material_library/`：`provider.go`（Provider 抽象）、`proxy.go`（分发 + 归属校验 + 响应归一化）、`ark.go`（动作封装）。
- ADR `docs/adr/0007-material-library-provider-and-unified-format.md`：对外统一 tokenhub 实际格式、归属隔离、真人认证闭环。

## 目标

1. 文档站新增顶层「素材库 Material」章节（与「视频生成 API」平级），中英双语。
2. 覆盖对外 `/api/material/{Action}` **全部 12 个 Action**，内容严格对齐代码/ADR 行为。
3. 下游开发者凭此文档可完整闭环：建组 → 传素材（含真人认证）→ `asset://` 引用生成视频。

## 非目标

- 不写管理端接口 `/api/materials/*`（默认前端控制台用 UserAuth 登录态，不对下游开放）。
- 不展开 `WaitVisualValidateGroupId` / `h5-callback` 内部实现——对外只讲「传 CallbackURL 认证后 302 回跳」这一可观察行为。
- 不触碰 `web/default/`（AGENTS.md 保护）。
- 不修改 `content/docs/{zh,en}/api/ai-model/...` 下的 OpenAPI 自动生成内容。
- 不改主网关、计费、SDK、控制台相关文案与代码。

## 关键发现（决定文档准确性）

1. **12 个 Action，不是 11 个**：v1.0 漏列 `ListAssetGroups`。代码里它有本地实现（查归属表返回，不走上游）。
2. **`ListAssets.Filter.GroupIds` 必填**：空 `GroupIds` 网关直接 400 `InvalidParameter`（防越权列出共享账号全部素材）。
3. **Base URL**（用户 2026-08-25 定）：国内 `https://yunjizi.cn`、国外 `https://api.yunjizi.cn`。废弃 v1.0 的 `43.156.28.30:3000`。
4. **两类响应格式**：CRUD（建/列/改/删 组与素材）保留 `{ResponseMetadata, Result}` 信封；会话类（`CreateVisualValidateSession` / `GetVisualValidateResult`）直接格式（无信封）。错误统一 `{ResponseMetadata:{}, ResponseError:{Code,Message}}`。
5. **归属隔离（IDOR 防护）**：`Get/Update/DeleteAssetGroup`、`ListAssets.Filter.GroupIds`、`CreateAsset.GroupId`、`CreateVisualValidateSession` 的会话归属均校验「API key 所属账号」；组/素材单操作 `GetAsset`/`DeleteAsset` **暂不校验**（素材不落归属表，需「素材→组」映射，ADR 0007 已知限制延后处理）。

## 决策

| # | 决策 | 原因 |
|---|---|---|
| D1 | 顶层章节 `material/`，icon=Library，pages=[index, groups, assets, real-person] | 与 `video/` 三页风格一致，独立成章（鉴权/错误码/入口与视频生成不同），后续可扩展 |
| D2 | 中英双语一次配齐（zh + en 平行） | 对外 API 文档面向下游开发者，英文是硬需求；与站点现有「每页双语」惯例一致 |
| D3 | 内容以代码/ADR 为准，参考文档 v1.0 仅作接口清单骨架 | 下游必须拿到「网关真实接受的行为」，避免重蹈 IK92LQ 文档与网关不符的覆辙 |
| D4 | 归属隔离未覆盖的 `GetAsset`/`DeleteAsset` 在文档中如实注明「当前未校验归属」 | 诚实暴露限制，避免下游误以为已隔离 |
| D5 | 认证/鉴权 401 错误格式以 TokenAuth 实际返回为准，实测一笔 | 已实测（见「实测结论」节）：401 为 OpenAI 风格 `{error:{type:"new_api_error",...}}` |

## 方案

### 页面结构

```
content/docs/zh/material/
├── meta.json        {title:"素材库", icon:"Library", pages:["index","groups","assets","real-person"]}
├── index.mdx        概览
├── groups.mdx       素材组
├── assets.mdx       素材
└── real-person.mdx  真人认证

content/docs/en/material/  （同名平行，标题 "Material Library"）
```

根导航：`zh/meta.json`、`en/meta.json` 的 `pages` 在 `"video"` 后插入 `"material"`。

### 每页内容要点（与代码一一对齐）

| 页 | Action | 要点 |
|---|---|---|
| **index** | — | 统一入口 `POST /api/material/{Action}`；鉴权 `Authorization: Bearer sk-`；Base URL 双域名；**两类响应格式**（CRUD 信封 / 会话类直接）；错误码表（网关主动返回 `InvalidParameter` 400 / `NotFound` 404，其余透传上游归一化 `ResponseError`）；归属隔离规则；`asset://<素材ID>` 引用方式 |
| **groups** | `ListAssetGroups` | **本地查询**（按 API key 归属查本地表，不走上游）；分页参数 `PageNumber/PageSize`，`Filter.GroupType` 过滤 |
| | `GetAssetGroup` | 校验组归属，无归属 404 `NotFound` |
| | `CreateAssetGroup` | `Name`(必)/`Description`(否)/`GroupType`(必, `AIGC`)；Name 含 `<`/`>` 或 >255 rune 报 400 |
| | `UpdateAssetGroup` | 仅可改 `Description`；传不同 `Name` 报 `UnsupportedField` |
| | `DeleteAssetGroup` | 校验归属；同步删组内全部素材 |
| **assets** | `CreateAsset` | `GroupId`(必)+`URL`(必)+`Name`(必)+`AssetType`(必: Image/Video/Audio)；校验目标组归属；URL 公网可访问约束 |
| | `ListAssets` | **`Filter.GroupIds` 必填**（空 400）；`Filter.GroupType/Statuses/Name` 可选；归属校验 |
| | `GetAsset` | 素材详情（含 Status/URL/Error）；**当前未校验归属**（D4） |
| | `UpdateAsset` | `Id`+`Name`；Name 校验 |
| | `DeleteAsset` | **当前未校验归属**（D4） |
| **real-person** | `CreateVisualValidateSession` | 会话类**直接格式**：`Name`(否)/`CallbackURL`(否)/`ProjectName`(否) → `{BytedToken, H5Link, CallbackURL}`；传 `CallbackURL` → 网关替换为内置回调，认证后 302 回跳（带原始 query） |
| | `GetVisualValidateResult` | 会话类**直接格式**：`BytedToken`(必) → `{GroupId}`；**未完成返回 200+空 GroupId**（客户端轮询）；会话不存在/非本人 fail-closed 返回空（不泄露） |

### 内容准确性红线

- 所有请求体字段走火山风格 PascalCase（`GroupId`/`AssetType`/`PageNumber`…），`Content-Type: application/json`。
- 每个 Action 配一个 curl 示例（用 `BASE_URL="https://yunjizi.cn"`，注释注明国外 `https://api.yunjizi.cn`，同 `video/` 风格）。
- 响应示例严格用归一化后的实际格式（CRUD 信封 / 会话类直接）。

## 验证方案

1. **文档站可构建**（`bun run dev` 或 build，无断链/无 meta 错配）。
2. **逐条与代码核对**（已核对；写稿时再对照 `proxy.go`/`ark.go`/ADR 0007 复核一遍）。
3. **实测**（已完成 2026-08-27，测试站网关 `115.190.244.91:3006`）——结果落于「实测结论」节。

## 实测结论（2026-08-27，网关 115.190.244.91:3006）

| 探针 | 期望 | 实测 | 落笔依据 |
|---|---|---|---|
| 401 鉴权（坏 key） | OpenAI 风格 error 信封 | `{"error":{"code":"","message":"Invalid token (request id: ...)","type":"new_api_error"}}` | **401 是 OpenAI 风格** `{error:{type:"new_api_error",message}}`，非素材库 `ResponseError` 信封 |
| `ListAssets` 空 GroupIds | 400 InvalidParameter | `{"ResponseError":{"Code":"InvalidParameter","Message":"invalid parameter"},"ResponseMetadata":{}}` | 字段顺序与代码 writer 一致；空 GroupIds 必填坐实 |
| `GetVisualValidateResult` 随机 token | 200 空 GroupId | `{"GroupId":""}` | 未完成/不存在均 200 + 空 GroupId（fail-closed 坐实） |
| `ListAssetGroups`（好 key） | 200 信封 | `{"ResponseMetadata":{"Action":"ListAssetGroups","Region":"cn-beijing","RequestId":"req_local_23_1",...},"Result":{"TotalCount":0,"Items":[],...}}` | 12 个 Action 盘点坐实；本地查询信封格式坐实 |
| 不存在的 Action | 400 InvalidAction | 404 `{"ResponseError":{"Code":"InvalidActionOrVersion","CodeN":100008,...}}` | 未知 Action 由上游返回 `InvalidActionOrVersion`（非网关定义的 `InvalidAction`），错误码表据此写 |

**落笔修正（相对初版设计）**：
- 401 鉴权失败 → **OpenAI 风格** `{"error":{"type":"new_api_error","message":"Invalid token"}}`（非素材库 `Unauthorized`/`ResponseError` 信封）。概览页需区分「鉴权层错误（OpenAI 风格）」与「素材业务层错误（`ResponseError` 信封）」。
- v1.0 错误码表里的 `InvalidAction` → 实测上游实际返回 `InvalidActionOrVersion`；`Unauthorized` 改为 OpenAI 风格 401。
- 会话类失败（未完成）与「会话不存在/非本人」均返回 `{"GroupId":""}` + 200，客户端轮询即可，无需区分。

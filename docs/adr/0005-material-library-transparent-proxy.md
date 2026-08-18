# 素材库以透明反向代理接入（火山 ARK 兼容接口），内置单租户凭证，记录"用户↔素材组"归属

对接 ai-tokenhub 素材库（Seedance Asset Library，虚拟/真人素材）时，采用**透明反向代理**形态：`/api/material/{Action}` 映射到上游 ARK 兼容接口 `POST /ark?Action={Action}&Version=2024-01-01`，仅做鉴权头替换与 Action 映射两处最小处理。单租户共享，AK/SK 内置在单一 JSON 配置 `material_library` 中，下游客户端不持有也不感知这些凭证。代理静默记录"用户↔素材组"归属，为后续用户隔离与素材管理界面铺路。

**Status:** accepted

## 背景

ai-tokenhub 已作为渠道 61（`relay/channel/task/tokenhub/`）接入，但只实现 Seedance 视频生成的异步任务。本次要新增虚拟（AIGC）/ 真人（LivenessFace）素材库能力。上游提供**火山 ARK 兼容接口**（`docs/` 内"火山 ark 兼容接口-调用说明"）作为素材库的唯一接入面，决定架构的事实：

1. **单一入口、Action 区分操作**——所有接口为 `POST {base_url}/ark?Action=<Action>&Version=2024-01-01`，请求体 JSON 为火山风格 PascalCase 字段，响应统一 `ResponseMetadata/Result` 信封。不再有按资源划分的 REST 路径。
2. **租户隐含在 AK/SK 中**——接口内不需要传租户 ID；凭证所属租户即调用租户。故代理无需注入 tenantId。
3. **上传素材是 URL 直传**——`CreateAsset` 传公网 URL（非 multipart）；有 `ListAssets` 列表接口（不落库素材仍可选）。
4. **素材库凭证与生成凭证不同**——素材库走 AK/SK（`Authorization: Bearer pak_xxx:psk_xxx`），视频生成走单 API Key（Bearer token）。两者必须同属一个账号/租户，`asset://<id>` 引用才能解析到正确租户。

## 决策

1. **透明反向代理**：new-api 挂一条 catch-all 路由 `/api/material/*`，把路径段映射为上游 `Action`：`/api/material/{Action}` → `{base_url}/ark?Action={Action}&Version=2024-01-01`。path/query/body/上游错误格式原样透传，不定义 DTO、不做参数映射、不重写错误信封。客户端看到的 API 面 = 上游 ARK 接口面（以 Action 为路径段）。
2. **仅两处非透传**：
   - 鉴权头替换：收 new-api `TokenAuth`（API key，`Authorization: Bearer sk-xxx`，供 API 客户端调用，与生成任务同口径）→ 剥掉 → 注入 `Authorization: Bearer {ak}:{sk}`（取自配置）。归属记录的归属用户 = API key 所属用户（TokenAuth 从上下文注入）。
   - Action 映射：路径段 → query 参数 `Action`，并固定注入 `Version=2024-01-01`。
3. **单租户共享 + 内置凭证**：新增单一 JSON 配置项 `material_library`（`setting/material_setting/`，镜像 `video_tier_pricing` 的 `config.GlobalConfig.Register` 模式）：
   ```json
   {
     "enabled": true,
     "ak": "pak_xxx",
     "sk": "psk_xxx",
     "base_url": "https://ai-tokenhub.ai"
   }
   ```
   下游客户端不持有 AK/SK，全部由网关内置注入。
4. **记录"用户↔素材组"归属，不镜像素材**：新增轻量归属表 `user_material_groups`（user_id + seedance_asset_group_id 唯一 + 组名/类型镜像；上游素材组无状态字段，故不镜像状态）。代理在 `CreateAssetGroup`（解析响应 `Result.Id`，组类型取请求体）与 `DeleteAssetGroup`（请求体 `Id`）时**静默旁路记录**（`ModifyResponse` 观察层），对客户端完全透明。归属用户 = 调用 API key 的所属用户。素材组内的素材不落库，需要时透传上游查询。
5. **真人 + 虚拟素材均支持**：虚拟组同步创建（`CreateAssetGroup`，`GroupType: AIGC`）；真人组走 `CreateVisualValidateSession`（返回 H5Link/BytedToken，`CallbackURL` 由调用方自持，无平台回调确认）。真人认证必然有一次真人介入；且该接口不返回素材组 id，故真人组在建组时无法记录归属（仅 AIGC 组可记录），素材中心"真人素材"tab 预留待上游开放组 id。
6. **管理界面（素材中心）基于归属表实现，与 API 代理分开**：新增 UserAuth 管理端点 `/api/materials/*`（与 TokenAuth 的 `/api/material/*` 代理字面区分、互不冲突），前端"素材中心"页面（真人/AIGC tab、左侧组列表、右侧素材表格）。
   - **数据隔离**：组列表按 `user_id` 过滤（admin 看全部）；**写操作（删组/上传/改名/删素材/列素材）做归属校验**——非 admin 用户只能操作自己拥有的组，无权返回统一提示。
   - **enabled 守卫**：管理端点路由组挂 `RequireMaterialLibraryEnabled`，`material_library.enabled=false` 时禁用即禁用。
7. **生成侧零改动**：现有 tokenhub 适配器原样透传 `image_url.url` / `video_url.url` / `audio_url.url`，客户端直接传 `asset://<assetId>` 即可引用素材，无需改适配器，仅需文档说明。
8. **无计费**：素材库操作非模型调用，不扣配额、不进计费路径。

## 理由

- **纯透传换最少代码与最高保真**：素材库本质是资源目录管理，套 `TaskAdaptor`（异步生成）或做语义包装端点（DTO/映射/错误重写）都是无谓复杂度。透明代理一条路由解决全部 Action + 错误透传。
- **单租户共享符合实际业务模型**：平台统一维护一套素材（含真人授权），对外提供引用，而非每个 new-api 用户自持 tokenhub 账号。
- **透明代理牺牲了 API 抽象层**：客户端耦合 ai-tokenhub 的 Action 契约，上游改契约客户端需跟随。对"接口包装"定位，此代价可接受。
- **归属只记录不生效**：为隔离/管理界面铺路但不改变客户端行为，最小改动换取后续能力的数据基础。

## 影响

- 新增配置包 `setting/material_setting/material_library.go`（`Register("material_library", &cfg)`），提供 `GetConfig()` 读取。
- 新增归属表 `model/user_material_group.go`（`user_material_groups`），注册进 `AutoMigrate`。
- 代理加响应观察层（`service/material_library`）：`RecordContext` 提取登录用户 + 建组/删组信息（读请求体后原样装回），`ModifyResponse` 在成功时 upsert/删除归属记录；非建组/删组 Action 零开销。对客户端行为无任何改动。
- 新增 ARK 直连客户端 `service/material_library/ark.go`（ListAssets/CreateAssetGroup/DeleteAssetGroup/CreateAsset/UpdateAsset/DeleteAsset）。
- 新增代理路由（`/api/material/*`，TokenAuth）与管理端点（`/api/materials/*`，UserAuth）两组。
- 后端落点：
  - `setting/material_setting/` — JSON 配置注册与读取（单一键 `material_library.config`）。
  - `service/material_library/` — 反向代理（Action 映射 + 鉴权头替换）、归属观察层、ARK 直连客户端。
  - `controller/{material_library,material_console}.go` — 代理守卫 + 管理端点（归属校验、enabled 守卫）。
  - `model/user_material_group.go` — 归属表（upsert 不覆盖既有归属，归属不可转手）。
  - `router/api-router.go` — 两组路由注册。
- 前端：`web/default/src/features/materials/` 素材中心页面 + 菜单「素材中心」+ 路由 + 7 语言 i18n。
- **凭证一致性守卫**：`material_library` 的 AK/SK 必须与 TokenHub 渠道 61 的 API Key 同属一个账号/租户，否则 `asset://` 引用解析到错误租户。做法：配置保存时可选"测试连接"探针 + 文档红字说明。

## 已知限制与延后项

以下为代码评审与实测发现的延后项，已拆工单跟踪：
- **素材中心菜单未按配置显隐**（IK9RGI）：未配置素材库时菜单仍可见。
- **代理 Action 暴露面未收窄**（IK9RGN）：catch-all 透传全部 ARK Action，考虑黑名单收窄。
- **上传失败体验**（IK9RGO）：瞬时 500 无自动重试、Failed 素材未展示上游错误。
- **配置「测试连接」探针**（IK9RGQ）：AK/SK 与生成渠道凭证一致性未在校验时探测。

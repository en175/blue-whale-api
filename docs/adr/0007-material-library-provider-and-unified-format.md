# 素材库 Provider 化 + 对外接口统一 tokenhub 格式 + 安全加固

ADR 0005（透明反向代理接入）后的演进：上游 Provider 化、真人认证闭环、对外接口统一返回格式、归属隔离与输入安全加固。

**Status:** accepted

## 背景

ADR 0005 以透明反向代理接入 ai-tokenhub 素材库（火山 ARK 兼容接口）。随后按需扩展：

1. 新增火山引擎官方素材库作为第二上游（管理员 2 选 1）。
2. 真人素材（LivenessFace）需要 H5 活体认证闭环（管理端 + 对外 API）。
3. 对外接口返回格式因渠道而异（tokenhub 会话类直接、其余 CRUD 与火山渠道均为信封），接口文档无法统一给下游。
4. 验收测试发现对象级越权（IDOR）、XSS 存储、无效 URL 挂起、未认证查询 502 等缺陷。

## 决策

### 1. Provider 抽象（tokenhub / volcengine 分发）
`service/material_library` 定义 `Provider.DoAction(ctx, action, query, body) (*Response, error)`，按配置 `provider` 分发：
- **tokenhubProvider**：Bearer `{ak}:{sk}` 直连 `{base_url}/ark?Action=...&Version=...`
- **volcengineProvider**：火山管控面 SigV4 签名（复用已有 `volc-sdk-golang` 的 `base.Credentials.Sign`，`Service=ark`，零新依赖）
- 代理与管理端直连统一走 provider；`currentProvider()` 按配置返回；回调轮询用**会话记录的 provider**（不受全局配置切换影响）

### 2. 对外接口统一 tokenhub 实际格式（火山渠道兼容 tokenhub）
对外 `/api/material/*` 响应统一成 tokenhub **实际返回**的格式（火山渠道经 `normalizeUpstreamResponse` 归一化对齐），接口文档可统一给下游：
- **CRUD 成功**（建组/素材 9 个动作）：**保留 `ResponseMetadata/Result` 信封**——tokenhub 对 CRUD 本身返回信封（ARK 兼容），两渠道一致（如 `{"ResponseMetadata":{},"Result":{"Id":"group-..."}}`）
- **会话类成功**（CreateVisualValidateSession / GetVisualValidateResult）：直接返回内容（tokenhub 会话类即直接格式，火山信封剥壳对齐），如 `{"BytedToken":...}`、`{"GroupId":...}`
- **错误**：统一 `{"ResponseMetadata":{},"ResponseError":{"Code","Message"}}`（tokenhub 错误格式；火山 `ResponseMetadata.Error` 转 `ResponseError`）
- 实现：`normalizeUpstreamResponse`（动作感知——按 CRUD/会话类区分是否剥壳）在代理响应回写前归一化（`observeOwnership`/`restoreClientCallback` 之后，保证观察层读到原始信封）；网关自实现的 `ListAssetGroups` 构造信封、`GetVisualValidateResult` 构造直接格式
- **Content-Length**：重写响应体后跳过上游 `Content-Length`，交由 Go 按实际长度重算（防截断）
- **更正记录**：初版实现误将 CRUD 也去信封（破坏 tokenhub 自身信封、与已外发接口文档冲突），更正为按 tokenhub 实际格式保留 CRUD 信封

### 3. 归属隔离（防 IDOR）
对外资源操作校验 API key 用户归属（`user_material_groups`），否则 404/400：
- GetAssetGroup / UpdateAssetGroup / DeleteAssetGroup：组 id 归属校验
- ListAssets：`Filter.GroupIds` 全部校验，空 GroupIds 400 拒绝
- CreateAsset：目标组归属校验
- 素材单操作（GetAsset/UpdateAsset/DeleteAsset）：素材不落库，需「素材→组」归属映射，后续处理
- 会话不存在 / 非本人：fail-closed 返回空（不查上游、不泄露）

### 4. 真人素材认证闭环
- **回调中转**：对外客户自定义 CallbackURL 入库（`custom_callback`），发给上游的替换为网关回调；认证完成立即 302 到客户（带原始 query），后台异步轮询记录归属
- **会话表** `user_material_visual_sessions`（byted_token/h5_link/callback_url/custom_callback/group_id/status）
- **查询缓存**：对外 GetVisualValidateResult 本地命中直接返回，未结算查上游并回填（会话 group_id + 归属，幂等）；认证未完成返回 200 + 空 GroupId（客户端轮询）
- 认证未完成：火山 ValidatePending/NotFound、tokenhub 500 InternalError（有本地会话且 InternalError 限定 tokenhub）→ 200 空

### 5. 输入安全与超时
- **输入过滤**：名称统一经 `MaterialNameInvalid` 校验（管理端与代理一致拒绝），拒绝两类：①rune 计数 >255（防镜像表列宽溢出；超长透传上游 tokenhub 会 500 未优雅拒绝）；②HTML 危险字符 `<`/`>`（防 XSS 存储）。覆盖代理层全部 Name 动作（CreateAssetGroup/CreateVisualValidateSession/CreateAsset/UpdateAsset/UpdateAssetGroup）与管理端组/会话/素材处理器。SQL 注入负载不在网关拦截（网关是透明代理，注入防御在上游后端，网关照常透传、错误统一归一化）。`ListAssets` 的 `Filter.Name` 为查询过滤（非存储），暂不校验。代理侧空名放行（Name 可选，上游忽略），管理端空名必填
- **上游超时**：`GetHttpClient` 默认无超时（`RelayTimeout=0`），素材上游调用加 30s 超时兜底（无效 URL 快速失败）

## 理由

- **Provider 抽象**：火山签名无法透传（SigV4），统一 `DoAction` 让代理/管理端/回调共用一处鉴权与地址映射；`providerByName` 按会话 provider 轮询避免全局配置切换污染。
- **统一格式**：下游只需解析一种格式；CRUD 保留信封（tokenhub 自身即信封，与已外发接口文档一致）、会话类直接、错误统一 `ResponseError`（tokenhub 契约）。仅火山渠道的会话类与错误格式归一化对齐，CRUD 信封透传保留。
- **归属隔离**：素材凭证共享（单租户），本地归属表是唯一租户隔离手段；fail-closed 不泄露资源是否存在。
- **回调中转**：客户自有 CallbackURL 会绕开网关，替换为网关回调保证拿到认证结果 + 302 回客户（成功/失败都 302）。
- **输入安全**：XSS 输出侧靠前端转义（React 默认），输入过滤为第一道防线；长度拒绝保证镜像表列宽安全（截断改为拒绝，行为可预期，不留死代码）；30s 超时防上游挂起。

## 影响

- `service/material_library/`：provider.go（Provider/tokenhub/volcengine）、proxy.go（分发 + 归一化 + 归属校验 + XSS）、ark.go（直连封装 + 兼容双格式）、回调/查询缓存
- `model/`：`user_material_groups`（provider 列 + 复合唯一索引）、`user_material_visual_sessions`（真人会话）
- `controller/material_console.go`：管理端点 + 名称校验
- 对外接口：统一 tokenhub 实际格式（CRUD 保留信封、会话类直接、错误 ResponseError），客户端按已外发接口文档适配

## 已知限制与延后项

- **素材单操作归属校验**（GetAsset/UpdateAsset/DeleteAsset）：需「素材→组」归属映射，后续处理
- **熔断/降级**：上游故障快速失败但无熔断短路，本地读链路不受影响，可接受
- **建组幂等**：重复调用重复建组，待产品确认
- 第三方上游（tokenhub）故障（如 CreateAsset 500）需上游排查，网关侧无清理机制

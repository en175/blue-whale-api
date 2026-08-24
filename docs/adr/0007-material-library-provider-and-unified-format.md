# 素材库 Provider 化 + 对外接口统一 tokenhub 格式 + 安全加固

ADR 0005（透明反向代理接入）后的演进：上游 Provider 化、真人认证闭环、对外接口统一返回格式、归属隔离与输入安全加固。

**Status:** accepted

## 背景

ADR 0005 以透明反向代理接入 ai-tokenhub 素材库（火山 ARK 兼容接口）。随后按需扩展：

1. 新增火山引擎官方素材库作为第二上游（管理员 2 选 1）。
2. 真人素材（LivenessFace）需要 H5 活体认证闭环（管理端 + 对外 API）。
3. 对外接口返回格式因渠道而异（火山信封 / tokenhub 直接），接口文档无法统一给下游。
4. 验收测试发现对象级越权（IDOR）、XSS 存储、无效 URL 挂起、未认证查询 502 等缺陷。

## 决策

### 1. Provider 抽象（tokenhub / volcengine 分发）
`service/material_library` 定义 `Provider.DoAction(ctx, action, query, body) (*Response, error)`，按配置 `provider` 分发：
- **tokenhubProvider**：Bearer `{ak}:{sk}` 直连 `{base_url}/ark?Action=...&Version=...`
- **volcengineProvider**：火山管控面 SigV4 签名（复用已有 `volc-sdk-golang` 的 `base.Credentials.Sign`，`Service=ark`，零新依赖）
- 代理与管理端直连统一走 provider；`currentProvider()` 按配置返回；回调轮询用**会话记录的 provider**（不受全局配置切换影响）

### 2. 对外接口统一 tokenhub 直接格式
对外 `/api/material/*` 响应**统一 tokenhub 直接格式**（去火山 `ResponseMetadata/Result` 信封），接口文档可统一给下游：
- 成功：直接返回上游 `Result` 内容（如 `{"Id":"group-..."}`、`{"GroupId":"..."}`、`{TotalCount,Items,...}`）
- 错误：统一 `{"ResponseError":{"Code","Message"}}`（tokenhub 错误格式）
- 实现：`normalizeUpstreamResponse` 在代理响应回写前归一化（`observeOwnership`/`restoreClientCallback` 之后，保证观察层读到原始信封）；网关自实现的 `ListAssetGroups`/`GetVisualValidateResult` 直接构造直接格式
- **Content-Length**：重写响应体后跳过上游 `Content-Length`，交由 Go 按实际长度重算（防截断）

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
- **XSS 输入过滤**：组名/真人会话名含 `<`/`>` 拒绝（管理端 + 代理 CreateAssetGroup/CreateVisualValidateSession）；名称长度 rune 计数 ≤255，管理端与代理**一致拒绝**（不再落库截断）——双渠道在请求层拦截后，`user_material_groups` / 会话记录不会收到超长名，镜像表侧无需再截断（曾有的 255 rune 截断为不可达代码，已移除）。代理侧空名放行（Name 可选，上游忽略），管理端空名必填
- **上游超时**：`GetHttpClient` 默认无超时（`RelayTimeout=0`），素材上游调用加 30s 超时兜底（无效 URL 快速失败）

## 理由

- **Provider 抽象**：火山签名无法透传（SigV4），统一 `DoAction` 让代理/管理端/回调共用一处鉴权与地址映射；`providerByName` 按会话 provider 轮询避免全局配置切换污染。
- **统一格式**：下游只需解析一种格式；错误统一 `ResponseError`（tokenhub 契约）。透传契约牺牲（客户端不再见上游原样信封），换接口文档统一。
- **归属隔离**：素材凭证共享（单租户），本地归属表是唯一租户隔离手段；fail-closed 不泄露资源是否存在。
- **回调中转**：客户自有 CallbackURL 会绕开网关，替换为网关回调保证拿到认证结果 + 302 回客户（成功/失败都 302）。
- **输入安全**：XSS 输出侧靠前端转义（React 默认），输入过滤为第一道防线；长度拒绝保证镜像表列宽安全（截断改为拒绝，行为可预期，不留死代码）；30s 超时防上游挂起。

## 影响

- `service/material_library/`：provider.go（Provider/tokenhub/volcengine）、proxy.go（分发 + 归一化 + 归属校验 + XSS）、ark.go（直连封装 + 兼容双格式）、回调/查询缓存
- `model/`：`user_material_groups`（provider 列 + 复合唯一索引）、`user_material_visual_sessions`（真人会话）
- `controller/material_console.go`：管理端点 + 名称校验
- 对外接口：统一 tokenhub 直接格式（破坏上游信封透传，客户端按新文档适配）

## 已知限制与延后项

- **素材单操作归属校验**（GetAsset/UpdateAsset/DeleteAsset）：需「素材→组」归属映射，后续处理
- **熔断/降级**：上游故障快速失败但无熔断短路，本地读链路不受影响，可接受
- **建组幂等**：重复调用重复建组，待产品确认
- 第三方上游（tokenhub）故障（如 CreateAsset 500）需上游排查，网关侧无清理机制

# 新开渠道 72 运河启源，对客旧视频接口、上游与原渠道 71 相同

渠道 71 只对客开放火山原生协议（`/api/v3/contents/generations/tasks`），但存量下游仍按网关旧视频接口接入。本次新开渠道类型 **72（AistoreVolcLegacy）**，对客只暴露旧视频接口，上游协议、端点、鉴权与渠道 71 完全相同。

**Status:** proposed

## 背景

- 渠道 71 已按 ADR 0025 上线：上游 `https://aistore.gsiig.cn/v1`，请求/响应按火山原文透传。
- 旧视频接口是网关既有的对客协议：`/v1/video/generations` 提交与查询、`/v1/videos/{id}/finalize` 成片。存量下游的 SDK 与调用代码都按这套形状编写。
- 渠道类型号落库，任务归属按渠道类型选适配器。让 71 同时兼容两套协议会把「路径决定响应形状」的分支塞进同一个渠道，也会让 71 的路径隔离失效（ADR 0024 / 0025 已记录同类理由）。
- 渠道 70 已经用同一个适配器兼容两套入口，但它单渠道同时服务两条路径；本次要求 71 保持原生专用，因此需要新的路径边界而不是复用 70 的做法。

## 决策

1. **新开渠道类型 72 `AistoreVolcLegacy`，不改 71。**
   - `ChannelBaseURLs[72] = "https://aistore.gsiig.cn/v1"`，与 71 相同。
   - `GetTaskAdaptor` 复用 `taskaistore.TaskAdaptor`；适配器按 `info.ChannelType` 选择展示名（`aistore-volc` / `aistore-volc-legacy`）。
   - 不新增 APIType：任务专用渠道，模型注册循环自动跳过。

2. **路径隔离按渠道类型互斥。**
   - 71：只服务 `NativeArkVideoTaskPath`。
   - 72：只服务 `/v1/video/generations`（提交与查询）与 `/v1/videos/{id}/finalize`。
   - 两者都不服务 `/v1/videos` 创建、remix 与 chat。`constant.ChannelServesRequestPath` 仍是唯一判定口径，亲和、内存缓存与数据库能力表共用。

3. **抽取火山协议层 `relay/channel/task/arknative`。**
   - `aistore` 与 `tokenhubvolc` 都依赖它，彼此不互相 import。
   - 内容：原生请求模型与解析、`ArkRequest`（原生上下文优先，旧接口按内部请求体还原）、成片参数校验与取值、`InferAction`。
   - `MappedModelName` 下沉到 `taskcommon`。

4. **旧接口入口复用同一套校验与上游构造。**
   - 提交：`prompt` 补成 `content[]` 文本项，`metadata.content` 的媒体项原样保留，`seconds` 变顶层 `duration`；`group` 等网关键不发给上游。
   - 样片：2.5 模型 `metadata.draft=true` 强制 `draft=true` + `480p`；非 2.5 开样片、样片非 480p 拒绝。
   - 成片：只接受 `output_format` / `watermark` / `return_last_frame` 等输出规格参数，分辨率固定 `1080p`；`content` 只能是 `draft_task` 引用。
   - 提交入口拒绝 `draft_task` 引用，避免绕过成片归属校验与 1080p 档计费。

5. **响应形状按入口区分。**
   - 原生入口（71）：上游原文，`id` 换网关公开 ID，`upstreamTaskId` 带出 cgt 号。
   - 旧接口入口（72）：`dto.NewOpenAIVideo()`，与渠道 61/66/67/70 一致。

6. **计费与 71 完全一致。** 复用 `EstimateVideoTierBilling` 与档位价表；样片按 480p 基准档，成片按 1080p / 1080p_video（继承样片的视频输入）。

## 理由

1. **对客契约是渠道的一部分。** 同一上游可以因对客协议不同拆成不同渠道；把两套形状塞进 71 会让任务日志、限制与灰度都无法按对客协议区分。
2. **路径隔离必须保持单值。** 71 原生专用是已交付的行为；扩 71 的路径集合会让既有下游的随机选路结果变化。
3. **共享的是协议，不是渠道。** 火山协议层被 70 / 71 / 72 共同使用，抽取后三者是兄弟关系，谁也不会继承另一家的上游差异。
4. **旧接口是既有契约，不是临时兼容。** 下游 SDK 已按 `/v1/video/generations` 落地，新建渠道比要求下游迁移更安全。

## 后果

- **71 的行为逐位不变。** 新路径判定只多一个 `case`，`ChannelServesRequestPath` 对 71 的返回值不变。
- **72 与 71 需要分别配置模型与限额。** 同一上游两条渠道各自启用，管理员可按对客协议独立放量。
- **旧接口入口的样片成片必须两步。** 第一步提交（`draft=true`）拿公开任务 ID，第二步 `POST /v1/videos/{id}/finalize`；两步是两条独立计费任务。
- **上游字段透传范围与 71 不同。** 旧接口只保留 `metadata` 里的火山参数；顶层未知字段（例如自定义扩展）不会原样转发，原生入口才有全文透传。
- **`arknative` 成为共享层。** 修改协议行为会同时影响 70 / 71 / 72，需按三个渠道的入口形状分别回归。

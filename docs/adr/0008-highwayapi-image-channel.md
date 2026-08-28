# 新开 HighwayAPI 同步图像渠道，走普通 Adaptor 而非异步任务系统

上游 HighwayAPI（jiekou.ai 接口AI 平台）的文生图接口是**同步** HTTP（`POST https://api.highwayapi.ai/v3/{model}`，Bearer 鉴权），一次请求直接返回图片数组（`{"images": [...]}`）；模型名不在请求体，而是拼接在 URL 路径上。

**Status:** accepted

## 背景

本项目有两套接入外部生成类上游的路径：

1. 异步任务系统（`TaskAdaptor`，`relay/channel/task/`）：提交任务 → 轮询状态 → 结算，服务于「提交后长时间运行」的上游（XFAI、Kling、Vidu 等）。
2. 普通同步适配器（`Adaptor`，`relay/channel/`）：一次往返，服务于 OpenAI `/v1/images/generations` 这类同步端点（jimeng、xai、volcengine seedream 等）。

HighwayAPI 请求立即返回结果，没有任务 ID、没有轮询端点。

## 决策

新开独立同步渠道 `ChannelTypeHighwayAPI`（type 63，包 `relay/channel/highwayapi/`），实现普通 `Adaptor` 接口，注册进 `RelayFormatOpenAIImage` → `ImageHelper` 链路；**不进**异步任务系统。遵循 ADR-0003 的渠道边界原则：边界由上游协议与端点决定。

- URL 构建：`{baseURL}/{UpstreamModelName}`，默认 `baseURL = https://api.highwayapi.ai/v3`，`/v3` 及代理前缀等可变部分全部留在渠道 base_url 配置里，适配器只拼模型段。
- 请求侧透传：`prompt`/`n`/`size`/`quality`/`background`/`moderation`/`output_format`/`output_compression` 按 `dto.ImageRequest` 已解析字段透传，客户端未知字段经 `Extra` 原样透传（上游后续加参数无需改代码），也支持 `extra_fields` 对象合并；`model` 不进请求体。上游只返回图片 URL，`response_format=b64_json` 显式拒绝。
- 响应侧转换：`images`（gpt-image 系列）或 `image_urls`（部分模型变体）统一转为 OpenAI `data[].url`；非 `http(s)://` 值按 `b64_json` 兜底。
- 计费：复用 `ImageHelper` 既有链路（imageN × 模型倍率，`PostTextConsumeQuota`），无自定义计费。`n` 的上限由 `GetAndValidOpenAIImageRequest` 统一按 `dto.MaxImageN` 拦截。

## 理由

异步任务系统的价值在于「提交后长时间运行」的生命周期管理（预扣全额、轮询、按终态差额结算）。HighwayAPI 同步返回，套任务系统只会引入一张空转的 task 记录、一次无意义的轮询和 ForcePreConsume 锁额，反而让计费与重试语义变复杂。同步 Adaptor 路径上失败即报错、失败不扣费，天然满足容器工单（IKBO4U）的通用验收项。

透传未知字段而非白名单，是因为上游（聚合平台）参数面会随模型演进扩张；白名单每加一个参数就要发版，`Extra` 透传让网关保持中立。

## 影响

- 新增渠道类型 63 / `APITypeHighwayAPI`，注册进渠道常量（含 `ChannelBaseURLs` 第 63 项）、`ChannelType2APIType` 映射与 `GetAdaptor`；渠道测试标记为不支持（避免真实出图扣费）。
- `/v1/images/generations` 可直接路由该渠道；`/v1/images/edits` 在转换阶段显式拒绝（上游为纯文生图）。
- 前端渠道类型 63 注册进类型列表、图标映射与类型配置（默认 base url 自动填充）。
- 默认模型 `gpt-image-2-text-to-image`、`gpt-image-2-light-text-to-image`；上游 `/v3` 下其他模型可由管理员在渠道模型列表自行添加。

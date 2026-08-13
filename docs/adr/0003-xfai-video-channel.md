# 新开 XFAI 视频渠道，不复用 doubao 渠道

上游「XFAI」是聚合平台，重卖火山 Seedance 2.0 视频生成；其请求参数、响应结构、状态枚举与 doubao 渠道（火山直连）完全一致，仅端点路径不同（`POST {base}/create` + `GET {base}/tasks/{id}`，Bearer 鉴权，模型 `doubao-seedance-2-0-c3`）。

**Status:** accepted

## 背景

对接 XFAI 时发现其视频生成协议与 doubao 渠道字节一致（`content[]` 多模态 + `ratio`/`resolution`/`duration`/`frames`/`generate_audio`/`watermark` 等），最省事的做法是把 `doubao-seedance-2-0-c3` 加进 doubao 渠道的 `ModelList`。但 XFAI 是独立上游：不同 baseURL、不同鉴权、不同可用性与对账。复用 doubao 会让一个渠道同时绑定两个上游，任一方端点变动都牵连另一方。

## 决策

新开独立 task 渠道 `ChannelTypeXFAI`（包 `relay/channel/task/xfai/`），复制 doubao 的 converter 与响应结构，仅改 URL 动词；可变命名空间（`/api/v1/seedance_c3`）放进 channel `base_url` 配置，代码只写死 `/create` 与 `/tasks/{id}` 两个稳定动词。

## 理由

渠道边界由**上游协议与端点**决定，不由模型厂商决定。参数一致 ≠ 同一上游。独立渠道换来独立生命周期（baseURL、鉴权、可用性、对账），代价是一份 converter 的少量复制，可接受。

## 影响

- 计费复用现有「视频档位价格表」机制，`doubao-seedance-2-0-c3` 档位价与内置 `doubao-seedance-2-0-260128` 一致，仅在价格表新增一个模型键。
- 对外仍走统一 OpenAI 视频格式 `/v1/video/generations`，下游无感知。

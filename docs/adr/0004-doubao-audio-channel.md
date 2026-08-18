# 新开 DoubaoAudio 音频生成渠道，不复用火山 WS TTS 渠道

上游「豆包语音·音频生成大模型 seed-audio-1.0」是同步 HTTP 接口（`POST /api/v3/tts/create`，`X-Api-Key` 单头鉴权），一次返回完整音频（`{url, duration, original_duration}`）；火山引擎(45) 渠道是 WebSocket 流式语音合成（逐句下发人声朗读）。两者协议、端点、鉴权、计费口径完全不同。

**Status:** accepted

## 背景

接入 seed-audio 时最省事的做法是把 `seed-audio-1.0` 加进火山引擎(45) 渠道的 `ModelList`。但 45 是 WS 流式 TTS 适配器（`relay/channel/volcengine/`），只支持人声朗读的流式合成；seed-audio 是同步 HTTP、非流式，生成任意音频（配乐/音效/人声），返回结构（`audio`/`url` 二选一）与鉴权（`X-Api-Key` vs WS 签名）都不同。复用 45 需要在同一适配器里塞进两套协议。

## 决策

新开独立同步渠道 `ChannelTypeDoubaoAudio`（type 62，包 `relay/channel/doubaoaudio/`），遵循 ADR-0003 的渠道边界原则：**边界由上游协议与端点决定，不由模型厂商决定**。对外走统一 OpenAI 音频格式 `/v1/audio/speech`。计费按输出时长：1 token = 1 秒，`ceil(original_duration)`，ModelRatio 配置为每秒价格。

## 理由

WS 语音合成与 HTTP 音频生成是两个上游协议，塞进一个渠道会让同一渠道绑定两套协议，任一方接口变动都牵连另一方；且计费口径不同（TTS 按文本/字，seed-audio 按输出时长），复用会破坏计费模型。

## 影响

- 新增渠道类型 62 / `APITypeDoubaoAudio`，注册进渠道常量、`ChannelType2APIType` 映射与 `GetAdaptor`；渠道测试标记为不支持（无余额查询等）。
- 计费按输出时长（`original_duration`），转换走饱和助手 `QuotaRoundChecked`，超界 clamp 落入审计字段。
- 对外暴露 OpenAI 兼容 `/v1/audio/speech`，下游无感知。
- 与火山引擎(45) WS TTS 渠道并存，互不干扰。
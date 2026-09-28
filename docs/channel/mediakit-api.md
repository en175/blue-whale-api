# AI MediaKit 工具接口文档

火山引擎 **AI MediaKit** 是视频云的智能多媒体工具集：图像的增强/抠图/擦除/翻译、视频的增强/转码/字幕/抠像/换脸、音频的分离/转码/抽音轨/截取/混合/调速等。本网关共开放 **69 个工具**——官方三大集合下的 64 个，外加集合外的 5 个：`extract-audio`（视频抽音轨）、`trim-audio`（音频截取）、`mix-audio`（音频混合）、`mux-audio-video`（音视频合成）、`adjust-audio-speed`（音频调速）。

本网关把它整体接进来，**请求体与上游完全一致、零改写**——你按 MediaKit 原生协议写请求，网关只做三件事：**鉴权、按工具路由、计费**。所以本页的每个示例都可以直接复制运行。

- **渠道类型：** MediaKit（内部编号 65）
- **上游协议：** `mediakit.cn-beijing.volces.com`，`Authorization: Bearer <apiKey>`
- **本文定位：** 对客调用说明。渠道自身的设计与取舍见 [`mediakit.md`](./mediakit.md)，决策记录见 [ADR-0018](../adr/0018-mediakit-channel-and-tool-registry.md)。

---

## 1. 基础信息

| 项 | 值 |
|---|---|
| 接口域名 | `https://<网关域名>`（部署时确定，下文示例统一写作 `<网关域名>`） |
| 鉴权 | `Authorization: Bearer sk-<你的网关令牌>` |
| 请求体 | `application/json` |
| 响应体 | `application/json` |

**鉴权头**：与网关其它接口一致，用你在网关申请的令牌。

```bash
-H 'Authorization: Bearer sk-<你的网关令牌>'
-H 'Content-Type: application/json'
```

> 鉴权失败返回 `401`，形如 `{"success": false, "message": "..."}`。

**工具名在路径里，不在请求体里**。MediaKit 原生请求体没有 `model` 字段——调哪个工具由 URL 路径决定。网关拿路径上的工具名去查内置注册表，查不到直接 `400`，不会转发给上游。

---

## 2. 两种调用模式

MediaKit 的工具分**同步**与**异步**两类，调用方式不同：

| | 同步工具 | 异步工具 |
|---|---|---|
| 覆盖 | 图像类，共 **27** 个 | 视频 **33** 个 + 音频 **4** 个，共 **37** 个 |
| 提交 | `POST /v1/mediakit/sync/{tool}` | `POST /v1/mediakit/{tool}` |
| 拿结果 | 响应体里**直接**有 `result` | 先拿 `task_id`，再轮询查询接口 |
| 查询 | 不需要 | `GET /v1/mediakit/tasks/{task_id}` |

### 2.1 同步调用（图像工具）

一次往返，响应里的 `result` 就是产物。

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/remove-image-background' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/photo.jpg"
  }'
```

### 2.2 异步调用（视频 / 音频工具）

分两步：**提交拿 ID** → **轮询取结果**。

**第 1 步：提交**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/enhance-video-generative' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/input.mp4",
    "resolution": "1080p"
  }'
```

返回的是**网关的公开任务 ID**（`task_xxx`），不是上游任务号：

```json
{
  "success": true,
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "message": ""
}
```

**第 2 步：轮询查询**

```bash
curl -X GET 'https://<网关域名>/v1/mediakit/tasks/task_xxxxxxxxxxxxxxxx' \
  -H 'Authorization: Bearer sk-<你的网关令牌>'
```

```json
{
  "success": true,
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "status": "completed",
  "message": "",
  "result": {
    "video_url": "https://example.volcvideo.com/out.mp4?auth_key=..."
  },
  "url": "https://example.volcvideo.com/out.mp4?auth_key=..."
}
```

`status` 取值：

| status | 含义 | 建议轮询间隔 |
|---|---|---|
| `pending` / `queued` | 已受理，排队中 | 5 秒 |
| `processing` | 处理中 | 5–10 秒 |
| `completed` | **成功**，产物在 `result` 里 | 停止 |
| `failed` | **失败**，原因在 `message` 里 | 停止 |

> `url` 是网关从 `result` 里按该工具的产物位置提取出来的便捷字段，等价于你手动取 `result.<产物字段>`（每个工具的产物字段见各小节）。**不是所有工具都有 `url`**——分析类工具（如画质评分、语音转字幕）只产出数据、不产出文件，这类工具的 `url` 不返回，结果结构看 `result`。失败的任务不返回 `url`。

**查询只能查自己的任务**。网关的查询接口只读本地任务表，按令牌所属用户过滤，拿别人的 `task_id` 查不到。

---

## 3. 通用约定

### 3.1 输入素材必须是公网可访问 URL

`video_url` / `image_url` / `audio_url` 都需要是**公网可直接下载**的地址（或先上传到 TOS / 视频点播 VOD 再取地址）。网关不做素材中转，地址直接透传给上游。

### 3.2 产物 URL 有效期 24 小时

上游产出的文件地址**默认 24 小时失效**。需要长期保存的，请在拿到结果后自行转存到你自己的存储。

### 3.3 异步任务幂等

上游对异步提交默认幂等：按账号 + 核心参数生成唯一标识，**24 小时内**相同标识返回同一个 `task_id`，不重复建任务、不重复计费。

需要主动控制时，在请求体里加 `client_token`（≤64 字符）：

- **重试**：传与上次**相同**的 `client_token` → 命中幂等，拿回同一个任务
- **强制重跑**：传一个**新的唯一值** → 建新任务

> 同步工具（图像类）**不支持**幂等，每次调用都是一次新处理。

### 3.4 并发与限流

- **异步任务**：账号维度全局限流，主账号及其全部子账号合计 **40 QPS**
- **同步任务**：按工具独立限流，各工具互不影响

超限会返回限流错误，需要降频或重试。

### 3.5 失败不计费

任务处理失败**不扣费**：异步任务失败时网关会**全额退回**提交时的预扣额度；同步工具上游返回 `success: false` 时同样不计费。

### 3.6 计费口径

三类工具按不同维度计费，**具体单价由管理员在网关的模型定价页配置**，本页只说明计费维度：

| 计费维度 | 覆盖 | 说明 |
|---|---|---|
| **按次** | 17 个（图像 14 + 视频 2 + 音频 1） | 每次调用一个固定价 |
| **按时长** | 39 个（视频 32 + 音频 7，含集合外 5 个的全部） | 按处理时长（元/分钟）计。提交时按预估时长**预扣**，任务完成后用上游回包的真实时长**结算差额**（多退少补） |
| **按量 GiB** | 13 个（图像基础编辑族） | 按处理量计 |

**3 个元信息获取工具的定价说明**：`probe-image-metadata`、`probe-video-metadata`、`probe-audio-metadata` 只读素材的容器/流信息、不产出文件，网关按上游口径给它们单独定价：

| 工具 | 上游计费口径 | 网关单价 | 说明 |
|---|---|---|---|
| `probe-video-metadata` | 按成功处理次数 0.1 元/千次 | **0.0001 元/次** | 与上游一致 |
| `probe-audio-metadata` | 按成功处理次数 0.1 元/千次 | **0.0001 元/次** | 与上游一致 |
| `probe-image-metadata` | 按处理量 GiB，0.025 元/GiB | **0.00024 元/次** | 近似值，见下 |

> `probe-image-metadata` 的上游口径是按图片体积计费，而**网关的模型定价只有「元/次」一档，不测量图片体积**。因此按单图 10 MiB 上限折算：`10/1024 GiB × 0.025 元/GiB = 0.00024 元/次`。大图不亏、小图略多收，差额在 1e-4 元量级。

### 3.7 多产物参数：请一律传 1（重要）

有三个工具支持「一次调用产出多条」，**本网关不支持**：

| 工具 | 参数 | 网关当前行为 |
|---|---|---|
| `generate-product-scene-image`（同步） | `batch_count` | 传 > 1 被网关**直接拒绝**（`400`） |
| `drama-recap`（异步） | `batch_count` | 上游会真的产出多条，但网关只按**一次**计费 |
| `drama-recap-vertical`（异步） | `max_count` | 同上。**该参数默认值为 3** —— 不传也会产出多条 |

原因：这些参数让上游按条数产出，而网关按单次计价。

**请务必显式传 `1`**，尤其是 `drama-recap-vertical`——它的 `max_count` 默认就是 3，不传会拿到多条产物却只被计费一次。

> 同步工具的 `batch_count > 1` 由网关在计费前拒绝；两个异步工具的同类参数**目前不会被网关拦截**，需调用方自行约束（异步侧拦截为待补项）。

---

## 4. 工具总览

**图像工具（同步，27 个）**

| # | 工具 | 端点 |
|---|---|---|
| 1 | 图像画质增强 | `POST /v1/mediakit/sync/enhance-image` |
| 2 | 图像画质评估 | `POST /v1/mediakit/sync/evaluate-image-quality` |
| 3 | 图像人脸打码 | `POST /v1/mediakit/sync/face-blur-image` |
| 4 | 图像擦除修复 | `POST /v1/mediakit/sync/erase-image` |
| 5 | 图像背景移除（智能抠图） | `POST /v1/mediakit/sync/remove-image-background` |
| 6 | 图像文字识别（OCR） | `POST /v1/mediakit/sync/image-ocr` |
| 7 | 图像智能裁剪 | `POST /v1/mediakit/sync/smart-crop-image` |
| 8 | 智能扩图 | `POST /v1/mediakit/sync/expand-image-canvas` |
| 9 | 集智瘦身 | `POST /v1/mediakit/sync/slim-image` |
| 10 | 图像翻译 | `POST /v1/mediakit/sync/translate-image-text` |
| 11 | 电商牛皮癣擦除 | `POST /v1/mediakit/sync/remove-image-elements` |
| 12 | 电商万创（商品场景图生成） | `POST /v1/mediakit/sync/generate-product-scene-image` |
| 13 | 图像暗水印 - 添加 | `POST /v1/mediakit/sync/add-image-invisible-watermark` |
| 14 | 图像暗水印 - 提取 | `POST /v1/mediakit/sync/extract-image-invisible-watermark` |
| 15 | 图像圆角 | `POST /v1/mediakit/sync/round-corner-image` |
| 16 | 图像翻转 | `POST /v1/mediakit/sync/flip-image` |
| 17 | 图像旋转 | `POST /v1/mediakit/sync/rotate-image` |
| 18 | 图像锐化 | `POST /v1/mediakit/sync/sharpen-image` |
| 19 | 图像高斯模糊 | `POST /v1/mediakit/sync/gaussian-blur-image` |
| 20 | 图像打码 | `POST /v1/mediakit/sync/mosaic-image` |
| 21 | 图像添加图文水印 | `POST /v1/mediakit/sync/add-image-watermark` |
| 22 | 图像调整 | `POST /v1/mediakit/sync/adjust-image-color` |
| 23 | 图像缩放 | `POST /v1/mediakit/sync/resize-image` |
| 24 | 图像负片 | `POST /v1/mediakit/sync/invert-image` |
| 25 | 图像裁剪 | `POST /v1/mediakit/sync/crop-image` |
| 26 | 图像压缩 | `POST /v1/mediakit/sync/compress-image` |
| 27 | 图像元信息获取 | `POST /v1/mediakit/sync/probe-image-metadata` |

**视频工具（异步，34 个）**

| # | 工具 | 端点 |
|---|---|---|
| 1 | 画质增强（标准版/专业版） | `POST /v1/mediakit/enhance-video` |
| 2 | 画质增强（大模型版） | `POST /v1/mediakit/enhance-video-generative` |
| 3 | 画质增强（极速版） | `POST /v1/mediakit/enhance-video-fast` |
| 4 | 字幕擦除（精细化版） | `POST /v1/mediakit/erase-video-subtitle-pro` |
| 5 | 视频抠图（人像） | `POST /v1/mediakit/matte-portrait-video` |
| 6 | 视频抠图（绿幕） | `POST /v1/mediakit/matte-greenscreen-video` |
| 7 | 语音转字幕（ASR） | `POST /v1/mediakit/asr-subtitles` |
| 8 | 视频识别字幕（OCR） | `POST /v1/mediakit/video-ocr` |
| 9 | 场景切分 | `POST /v1/mediakit/segment-scenes` |
| 10 | 智能语义切片（基础版） | `POST /v1/mediakit/semantic-segment` |
| 11 | 高光片段提取 | `POST /v1/mediakit/analyze-video-highlights` |
| 12 | 剧情故事线分析 | `POST /v1/mediakit/analyze-video-storyline` |
| 13 | 高光智剪 - 短剧 | `POST /v1/mediakit/generate-highlights-microdrama` |
| 14 | 高光智剪 - 小游戏 | `POST /v1/mediakit/generate-highlights-minigame` |
| 15 | 高光智剪 - 影视拆条 | `POST /v1/mediakit/generate-highlights-movie` |
| 16 | 剧本还原 | `POST /v1/mediakit/drama-script` |
| 17 | 解说视频生成 | `POST /v1/mediakit/drama-recap` |
| 18 | 解说视频生成（短剧行业模型） | `POST /v1/mediakit/drama-recap-vertical` |
| 19 | 视频抽帧 | `POST /v1/mediakit/extract-frames` |
| 20 | 视频转码 | `POST /v1/mediakit/transcode-video` |
| 21 | 极智超清 | `POST /v1/mediakit/martencode-video` |
| 22 | 视频转封装 | `POST /v1/mediakit/remux-video` |
| 23 | 视频暗水印 - 添加 | `POST /v1/mediakit/add-video-invisible-watermark` |
| 24 | 视频暗水印 - 提取 | `POST /v1/mediakit/extract-video-invisible-watermark` |
| 25 | 视频画质检测 VQScore | `POST /v1/mediakit/assess-video-quality` |
| 26 | 视频人脸打码 | `POST /v1/mediakit/face-blur-video` |
| 27 | 视频人脸融合（换脸） | `POST /v1/mediakit/face-swap-video` |
| 28 | 视频插帧 | `POST /v1/mediakit/video-frame-interpolation` |
| 29 | 视频口型对齐 | `POST /v1/mediakit/lip-sync` |
| 30 | 视频横转竖 | `POST /v1/mediakit/reframe-video-to-vertical` |
| 31 | 字幕擦除（标准版） | `POST /v1/mediakit/erase-video-subtitle` |
| 32 | 剧本还原（重绘场景） | `POST /v1/mediakit/drama-script-vertical` |
| 33 | 视频元信息获取 | `POST /v1/mediakit/probe-video-metadata` |
| 34 | 音视频合成（加 BGM / 换配音） | `POST /v1/mediakit/mux-audio-video` |

**音频工具（异步，8 个）**

| # | 工具 | 端点 |
|---|---|---|
| 1 | 人声背景音分离 | `POST /v1/mediakit/separate-voice` |
| 2 | 音频转码 | `POST /v1/mediakit/transcode-audio` |
| 3 | 视频抽音轨 | `POST /v1/mediakit/extract-audio` |
| 4 | 音频截取 | `POST /v1/mediakit/trim-audio` |
| 5 | 语音端点识别 | `POST /v1/mediakit/detect-voice-activity` |
| 6 | 音频元信息获取 | `POST /v1/mediakit/probe-audio-metadata` |
| 7 | 音频混合（多轨混音） | `POST /v1/mediakit/mix-audio` |
| 8 | 音频调速（快放 / 慢放） | `POST /v1/mediakit/adjust-audio-speed` |

> **本页覆盖全部 69 个工具**：官方三个集合下的 64 个与火山登记的数量一一对应（视频 33 / 图像 27 / 音频 4）——没有遗漏；在此之上另开放 5 个集合外的工具——视频侧 `mux-audio-video`（第 34 行，详见 [§6.34](#634-音视频合成-mux-audio-video)），音频侧 `extract-audio`（第 3 行，详见 [§7.3](#73-视频抽音轨-extract-audio)）、`trim-audio`（第 4 行，详见 [§7.4](#74-音频截取-trim-audio)）、`mix-audio`（第 7 行，详见 [§7.7](#77-音频混合-mix-audio)）、`adjust-audio-speed`（第 8 行，详见 [§7.8](#78-音频调速-adjust-audio-speed)）。它们不在官方那三个集合里，分别应下游「音视频合成」「音频提取」「音频截取」「音频混合」「音频调速」需求额外接入。

>
> 另有「剪辑工具 API」（27 个）与「大模型处理工具 API」（3 个）两个集合整体未接入：前者官方未给出逐工具的独立标识，后者依赖火山方舟按 token 计费。详见 [`mediakit.md`](./mediakit.md) 第 1 节。

---

## 5. 图像工具（同步，27 个）

### 5.1 图像画质增强 `enhance-image`

**作用：** 输入一张模糊、低分辨率或有噪点/伪影的图片，输出放大、去噪、去模糊、锐化、人像与文字增强后的高清图。适合 AIGC 生图后处理、老照片修复、OCR 与大模型参考图预处理、商品图/封面图优化等场景。

- **端点：** `POST /v1/mediakit/sync/enhance-image`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待增强图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `tool_version` | string | 否 | 模型版本：`standard`（标准版，默认）、`professional`（专业版）、`max`（大模型增强版） |
| `multiple` | number | 否 | 倍率放大，指定输出相对原图的放大倍数（2.0 表示长宽均放大 2 倍）。`standard` 取值 [1, 8]，`professional`/`max` 取值 [1, 30]。不传且未指定目标尺寸时默认 2 |
| `target_width` | int | 否 | 指定输出目标宽度（px）。仅指定一边时另一边按原图宽高比等比缩放；与 `target_height` 同传时按能完整容纳于目标框内的最大尺寸输出并保持原比例。`standard` 取值 [原图宽, 6144] 且放大倍数不超过 8，`professional`/`max` 取值 [64, 10240]。优先级：`multiple` > (`target_width`, `target_height`) |
| `target_height` | int | 否 | 指定输出目标高度（px），规则与 `target_width` 相同 |
| `generative_enhance_mode` | string | 否 | 生成式增强策略，仅在 `tool_version` 为 `professional` 或 `max` 时生效：`fidelity_first`（保真度优先，保留既有细节）、`generative_first`（生成度优先，智能生成更丰富的纹理细节） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/enhance-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/source.jpg",
    "tool_version": "professional",
    "generative_enhance_mode": "fidelity_first",
    "multiple": 2
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-enhance-image-1234567",
  "task_type": "enhance-image",
  "request_id": "20260415160000F6DE4C24A6A0D94B7FF2",
  "result": {
    "image_url": "https://...",
    "image_size": 875321,
    "image_format": "jpeg",
    "image_width": 2048,
    "image_height": 1536
  },
  "expires_at": 1778573984
}
```

**产物字段：** `result.image_url`

---

### 5.2 图像画质评估 `evaluate-image-quality`

**作用：** 输入一张图片，输出多维度的客观画质（清晰度/模糊、噪声、对比度、亮度等）与主观美学评分，用分数判断这张图能不能用。适合 AIGC 生图质量筛选、内容审核低质图打回、AI 训练数据清洗、图库与相册推荐排序等场景。

- **端点：** `POST /v1/mediakit/sync/evaluate-image-quality`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待评估图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `tool_version` | string | 否 | 模型版本：`standard`（标准版，默认，提供 15 种基础画质评估维度）、`professional`（专业版，基于大模型直接返回一组固定的综合性评分，不支持自定义评估维度） |
| `standard_evaluate_items` | array[string] | 否 | 指定评估维度，仅标准版（`standard`）生效。不传则默认返回 `["vqscore", "noise", "aesthetic", "blur"]` 四个维度；传了则只返回所列维度，可减少响应体大小 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/evaluate-image-quality' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/your_image.jpg",
    "tool_version": "standard",
    "standard_evaluate_items": [
      "vqscore",
      "noise"
    ]
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-evaluate-image-quality-12345",
  "request_id": "021778401180935fdbd0ff1ce00652c087a",
  "result": {
    "tool_version": "standard",
    "vqscore": 85.5,
    "noise": 10.2
  }
}
```

当 `standard_evaluate_items` 传入其他维度时，`result` 下按维度名平铺对应分值，例如：

```json
{
  "success": true,
  "result": {
    "tool_version": "standard",
    "contrast": 85.2,
    "brightness": 128.5
  }
}
```

**产物字段：** 无文件产物，结果结构见上方响应示例

---

### 5.3 图像人脸打码 `face-blur-image`

**作用：** 自动检测图片中的人脸区域并逐张打上马赛克，输出脱敏后的图片与每张人脸的位置坐标。适合社交平台内容审核、街景/监控画面脱敏、新闻与媒体素材处理、AI 训练数据集隐私清洗等需要批量保护人脸隐私的场景。

- **端点：** `POST /v1/mediakit/sync/face-blur-image`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `blur_shape` | string | 否 | 人脸模糊区域形状：`circle`（默认，圆形）、`rectangle`（矩形） |
| `mosaic_step` | int | 否 | 马赛克像素格大小（px 的正整数），值越大颗粒越大、脱敏强度越高。建议范围 [5, 100]，默认 12 |
| `face_detect_thresh` | float | 否 | 人脸检测置信度阈值，取值范围 (0, 1)，默认 0.9。调高更严格、减少非人脸误判但可能漏检；调低更宽松、可召回模糊或遮挡人脸但误判增多 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/face-blur-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/street.jpeg",
    "blur_shape": "rectangle",
    "mosaic_step": 20,
    "face_detect_thresh": 0.8
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-face-blur-image-123456789",
  "task_type": "face-blur-image",
  "request_id": "20260617202100EXAMPLE123456789",
  "result": {
    "image_url": "https://...",
    "image_size": 111174,
    "image_format": "webp",
    "image_width": 2732,
    "image_height": 1534,
    "face_count": 4,
    "face_location": [
      {
        "top_left_x": 2017,
        "top_left_y": 637,
        "bottom_right_x": 2215,
        "bottom_right_y": 894,
        "confidence": 0.9995
      }
    ]
  },
  "expires_at": 1781785262
}
```

**产物字段：** `result.image_url`

---

### 5.4 图像擦除修复 `erase-image`

**作用：** 自动检测并移除图片中的文字、水印、Logo/台标等图标，或用坐标框、遮罩图指定任意区域后擦除，并用智能填充把擦除区域与背景自然融合。适合内容清洗与分发（去他人平台水印）、素材二次创作（去路人杂物）、AIGC 生图瑕疵修正、商品图去杂乱背景等场景。

- **端点：** `POST /v1/mediakit/sync/erase-image`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `standard_scene` | string | 否 | 擦除场景：`full_screen_text_erase`（全屏文字擦除，默认，不传 `standard_scene` 时即执行此场景）、`full_screen_icon_erase`（擦除图片中所有常见图标，如 Logo、台标）、`selected_area_erase`（擦除指定区域，用于杂物、路人等非文字/图标目标） |
| `standard_erase_text` | string | 否 | 指定要擦除的文字内容，系统精确查找并擦除匹配字样；仅当 `standard_scene` 为 `full_screen_text_erase` 时生效。不传该参数时擦除全屏所有文字 |
| `selected_area` | object | 否 | 矩形擦除区域，仅 `standard_scene` 为 `selected_area_erase` 时使用。坐标以图片左上角为原点 (0, 0)、右下角为 (1, 1) 的归一化坐标 |
| `selected_area.top_left_x` | float | 否 | 矩形左上角 x 归一化坐标 |
| `selected_area.top_left_y` | float | 否 | 矩形左上角 y 归一化坐标 |
| `selected_area.bottom_right_x` | float | 否 | 矩形右下角 x 归一化坐标 |
| `selected_area.bottom_right_y` | float | 否 | 矩形右下角 y 归一化坐标 |
| `mask_url` | string | 否 | 遮罩图 URL，仅 `standard_scene` 为 `selected_area_erase` 时使用。图为与原图等大的黑白图，白色区域擦除、黑色区域保留，可定义任意形状；需与输入图分辨率一致。与 `selected_area` 可单独使用或组合使用，组合时擦除两者区域的并集 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/erase-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/confidential.jpg",
    "standard_scene": "full_screen_text_erase",
    "standard_erase_text": "内部资料"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-erase-image-123456",
  "request_id": "20240520203000ABCDEFG12345",
  "expires_at": 1779183261,
  "result": {
    "image_url": "https://...",
    "image_size": 51200,
    "image_format": "webp",
    "image_width": 1920,
    "image_height": 1080
  }
}
```

**产物字段：** `result.image_url`

---

### 5.5 图像背景移除（智能抠图） `remove-image-background`

**作用：** 把一张图片里的主体（人物、商品、物体等）从背景中分离出来，输出透明底 PNG 图。适合电商主图去底、证件照换底色、营销海报素材合成等需要"只要主体、不要背景"的场景。

- **端点：** `POST /v1/mediakit/sync/remove-image-background`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待抠图的图片地址。支持公网 HTTP/HTTPS URL、本地上传（`mediakit://`）、火山引擎对象存储（`tos://`）三种输入协议；支持 .png/.jpg/.jpeg/.webp/.tiff/.bmp/.heic，单张不超过 30 MB |
| `scene` | string | 否 | 抠图场景：`general`（通用）、`human`（人像）、`product`（商品）。分辨率上限随场景不同：`general` 长边 ≤ 10240 px / 短边 ≤ 6000 px / 总像素 ≤ 40 MP，`human`、`product` 长边 ≤ 7680 px、短边 ≤ 4320 px（必填性与默认值推断） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/remove-image-background' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/your_object_image.jpg",
    "scene": "general"
  }'
```

**响应**

```json
{
    "success": true,
    "task_id": "amk-tool-remove-bg-1716289900",
    "request_id": "20240521183140AABBCCDDEEFF778899",
    "result": {
        "image_url": "https://...",
        "image_size": 45875,
        "image_format": "png",
        "image_width": 800,
        "image_height": 600
    }
}
```

**产物字段：** `result.image_url`

---

### 5.6 图像文字识别（OCR） `image-ocr`

**作用：** 从图片中提取印刷体文字，返回每个文本块的内容、位置坐标与置信度；加强版还支持按指定关键词从票据、证照等图片中检索固定字段。适合文档扫描件/截图/海报的文字电子化，以及营业执照、商标注册证等资质的自动审查录入。

- **端点：** `POST /v1/mediakit/sync/image-ocr`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待识别图片地址。支持公网 HTTP/HTTPS URL、本地上传（`mediakit://`）、火山引擎对象存储（`tos://`）三种输入协议；支持 .png/.jpg/.jpeg/.webp/.tiff/.bmp/.heic，单张不超过 10 MB，短边 ≤ 2160 px、长边 ≤ 3840 px |
| `tool_version` | string | 否 | 识别版本：`standard`（标准版）、`max`（加强版）。标准版支持简体中文+英文印刷体通用识别并返回 `confidence`；加强版精度更高，支持印刷体与手写体、100+ 语种，且支持关键词匹配识别（默认值与标准版取值按文档"版本"表推断） |
| `task_type` | string | 否 | 识别任务类型：设为 `keyword` 时启用关键词匹配识别，结果在 `result.keyword_result` 中返回；不传时为通用文字识别，结果在 `result.ocr_result[]` 中返回（推断，关键词匹配需配合 `tool_version: "max"`） |
| `max_keywords` | array\<string\> | 否 | 待检索的关键词列表，如 `["公司名称", "法定代表人"]`；仅在 `task_type` 为 `keyword` 时生效（推断） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/image-ocr' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/screenshot_with_text.jpg"
  }'
```

**响应**

```json
{
    "success": true,
    "task_id": "amk-tool-image-ocr-1716290500",
    "request_id": "20240521184140AABBCCDDEEFF112233",
    "result": {
        "ocr_result": [
            {
                "content": "AI MediaKit",
                "confidence": 0.998,
                "top_left_x": 100,
                "top_left_y": 50,
                "bottom_right_x": 350,
                "bottom_right_y": 100
            },
            {
                "content": "图像文字识别",
                "confidence": 0.985,
                "top_left_x": 102,
                "top_left_y": 120,
                "bottom_right_x": 348,
                "bottom_right_y": 170
            }
        ]
    }
}
```

**产物字段：** 无文件产物，识别结果见上方响应示例的 `result.ocr_result[]`

---

### 5.7 图像智能裁剪 `smart-crop-image`

**作用：** 自动识别人脸主体区域，按你指定的目标尺寸把图片裁剪成构图合理的成品图（普通人脸与动漫人脸两种模型）；未检测到人脸时按降级策略（居中裁剪、毛玻璃填充）兜底输出。适合批量制作头像、封面图，以及把社交媒体配图统一到固定尺寸。

- **端点：** `POST /v1/mediakit/sync/smart-crop-image`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待裁剪图片地址。支持公网 HTTP/HTTPS URL、本地上传（`mediakit://`）、火山引擎对象存储（`tos://`）三种输入协议；支持 .png/.jpg/.jpeg/.webp/.tiff/.bmp/.heic，单张不超过 10 MB，宽高均不超过 6000 px |
| `target_width` | integer | 是 | 裁剪后图片的目标宽度（像素），如 `200` |
| `target_height` | integer | 是 | 裁剪后图片的目标高度（像素），如 `200` |
| `scene` | string | 否 | 裁剪场景：`person_face`（普通人脸）、`cartoon_face`（动漫/卡通人脸）。取值按文档示例推断，默认值推断为 `person_face` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/smart-crop-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/human-image.png",
    "target_width": 200,
    "target_height": 200,
    "scene": "person_face"
  }'
```

**响应**

```json
{
    "success": true,
    "task_id": "amk-tool-smart-crop-image-1234567",
    "result": {
        "image_url": "https://...",
        "image_size": 45023,
        "image_format": "jpeg",
        "image_width": 200,
        "image_height": 200
    },
    "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f"
}
```

**产物字段：** `result.image_url`

---

### 5.8 智能扩图 `expand-image-canvas`

**作用：** 用 AI 对上、下、左、右四个方向按比例补画内容，扩展区域在纹理与边缘过渡上与原图自然衔接。适合竖屏转横屏、横屏转竖屏，以及同一张广告素材适配多平台画幅比例的场景。

- **端点：** `POST /v1/mediakit/sync/expand-image-canvas`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待扩图图片地址。支持公网 HTTP/HTTPS URL、本地上传（`mediakit://`）、火山引擎对象存储（`tos://`）三种输入协议；支持 .png/.jpg/.jpeg/.webp/.tiff/.bmp，单张不超过 10 MB，长边不超过 4160 px |
| `expand_left` | number | 否 | 向左扩展的比例，取值范围 `[0, 0.4]`，`0` 表示该方向不扩展。比例相对原图**短边**计算，如 1920×1080 的图设 `0.2` 即向左扩 1080 × 0.2 = 216 px（默认值推断为 0） |
| `expand_right` | number | 否 | 向右扩展的比例，取值范围 `[0, 0.4]`，含义与计算方式同 `expand_left`（默认值推断为 0） |
| `expand_top` | number | 否 | 向上扩展的比例，取值范围 `[0, 0.4]`，含义与计算方式同 `expand_left`（默认值推断为 0） |
| `expand_bottom` | number | 否 | 向下扩展的比例，取值范围 `[0, 0.4]`，含义与计算方式同 `expand_left`（默认值推断为 0） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/expand-image-canvas' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/portrait_image.jpg",
    "expand_left": 0.2,
    "expand_right": 0.2,
    "expand_top": 0,
    "expand_bottom": 0
  }'
```

**响应**

```json
{
    "success": true,
    "task_id": "amk-tool-expand-image-canvas-1234567",
    "result": {
        "image_url": "https://...",
        "image_size": 120456,
        "image_format": "jpeg",
        "image_width": 1792,
        "image_height": 1024
    },
    "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f"
}
```

**产物字段：** `result.image_url`

---

### 5.9 集智瘦身 `slim-image`

**作用：** 在明显压缩图片体积的同时，用 AI 修复压缩产生的毛刺、彩噪、块效应并增强边缘纹理细节，输出比原图更小且更清晰的图片。适合批量降低图片存储成本、加快页面加载、改善终端图显体验。

- **端点：** `POST /v1/mediakit/sync/slim-image`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待瘦身图片地址。支持公网 HTTP/HTTPS URL、本地上传（`mediakit://`）、火山引擎对象存储（`tos://`）三种输入协议；支持 jpeg/jpg/png/heic/avif/webp（暂不支持动图），单张建议不超过 50 MB，宽高均不超过 10000 px（avif 输入建议宽高乘积不超过 100000） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/slim-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://vod-ai-test.tos-cn-beijing.volces.com/demo/large-image.jpg"
  }'
```

**响应**

```json
{
    "success": true,
    "task_id": "amk-tool-slim-image-12345678",
    "result": {
        "image_url": "https://...",
        "image_size": 158240,
        "image_format": "png",
        "image_width": 1920,
        "image_height": 1080
    },
    "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
    "expires_at": 1775271661
}
```

**产物字段：** `result.image_url`

---

### 5.10 图像翻译 `translate-image-text`

**作用：** 自动识别图片中的文字，擦除原文并在原位排版译文（或按所选模型直接叠加译文），输出一张保留原图布局与视觉风格、文字已换成目标语言的新图。适合跨境电商商品图、漫画、游戏、行业文献、路牌菜单等出海多语言本地化场景。

- **端点：** `POST /v1/mediakit/sync/translate-image-text`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待翻译图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `tool_version` | string | 否 | 翻译模型版本：`seed-translation`（默认，大模型版擦除模型，先擦除再放置译文，效果最佳）、`erase`（擦除翻译区域）、`overlay-translation`（直接叠加译文、不擦除原文，适合强调译文叠加效果）、`dense-text-translation`（密集文本非擦除，适合漫画）、`logo-retain-erase-translation`（带品牌识别的擦除模型，保持品牌 Logo/名称不被翻译） |
| `source_lang` | string | 否 | 源语言编号（如 `en`、`ja`、`zh`、`zh_hant`）。不传时系统自动检测源语言。不同 `tool_version` 支持的语种集合不同 |
| `target_lang` | string | 否 | 目标语言编号（如 `zh`、`en`、`ja`）。不同 `tool_version` 支持的语种集合不同 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/translate-image-text' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/image_in_english.jpg",
    "tool_version": "seed-translation",
    "source_lang": "en",
    "target_lang": "zh"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-translate-image-text-1234567",
  "task_type": "translate-image-text",
  "request_id": "021778401180935fdbd0ff1ce00652c087",
  "result": {
    "image_url": "https://...",
    "image_size": 150234,
    "image_format": "jpeg",
    "image_width": 1920,
    "image_height": 1080
  },
  "expires_at": 1778573984
}
```

**产物字段：** `result.image_url`

---

### 5.11 电商牛皮癣擦除 `remove-image-elements`

**作用：** 无需人工框选，自动检测并擦除商品图中的平台水印、品牌 Logo、促销贴纸、价格标签和干扰性文字，输出干净、可二次流转与再创作的商品图。适合电商搬家（去原平台水印）、商品图净化、素材二次创作等场景。

- **端点：** `POST /v1/mediakit/sync/remove-image-elements`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待擦除图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议。单图建议不超过 10 MB；分辨率需满足 128 px ≤ 高 ≤ 1440 px、128 px ≤ 宽 ≤ 2560 px |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/remove-image-elements' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/product_with_watermark.jpg"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-remove-image-elements-1234567",
  "task_type": "remove-image-elements",
  "result": {
    "image_format": "png",
    "image_height": 1200,
    "image_size": 980123,
    "image_url": "https://...",
    "image_width": 1200
  },
  "expires_at": 1778574188,
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f"
}
```

**产物字段：** `result.image_url`

---

### 5.12 电商万创（商品场景图生成） `generate-product-scene-image`

**作用：** 输入一张背景干净、主体明确的商品图，AI 自动生成全新的高质量背景与营销场景，输出可直接用于电商投放的商品场景图。三种玩法：标准版选预设场景模板一键出图；专业版用自然语言 Prompt + 风格参考图自定义创作；行业版生成汽车棚拍风格场景图。

- **端点：** `POST /v1/mediakit/sync/generate-product-scene-image`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 商品图地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议。单图建议不超过 10 MB；短边 ≥ 512 px、长边 ≤ 1024 px |
| `tool_version` | string | 否 | 模型版本：`standard`（标准版，使用 `standard_scene` 预设模板一键生成，成本较低）、`professional`（专业版，用 `prompt` + 参考图高度自定义，成本较高）、`industry`（行业版，汽车棚拍场景图，成本较高） |
| `standard_scene` | string | 否 | 标准版预设场景，仅 `tool_version` 为 `standard` 时生效。默认 `general`（通用，取该值时须同时配置用于生图的提示词）。常用值：`exhibit_home`（家居桌面）、`exhibit_simple`（极简展台）、`natural_pasture`（乡村牧场）、`water_reflect`（水面光影）、`exhibit_luxury`（奢华展台）等 |
| `batch_count` | int | 否 | 期望生成的图片张数（文档示例为 4）。**本网关 v1 不支持大于 1，见下方说明** |
| `output_width` | int | 否 | 输出图片宽度（px），取值范围 [512, 1024] |
| `output_height` | int | 否 | 输出图片高度（px），取值范围 [512, 1024] |
| `product_ratio` | float | 否 | 商品主体在画面中的占比，文档示例为 0.85（推断） |
| `prompt` | string | 否 | 创意描述（自然语言），支持中文或英文。`tool_version` 为 `professional` 时**必填**；为 `industry` 时选填；标准版仅在 `standard_scene` 为 `general` 时支持英文 |
| `professional_reference_image_url` | string | 否 | 背景风格参考图 URL，模型将模仿其风格创作。`tool_version` 为 `professional` 时**必填** |
| `professional_reference_image_adapt_scale` | float | 否 | 参考图适配强度，取值范围 [0, 1]，默认 0.9。数值越高，生成结果越接近参考图 |
| `industry_scene_image_url` | string | 否 | 参考棚拍场景图 URL，仅 `tool_version` 为 `industry` 时生效（推断） |
| `industry_detail_image_url` | string | 否 | 车标/车牌等细节参考图 URL，仅 `tool_version` 为 `industry` 时生效（推断） |

> **重要限制：本网关 v1 不支持 `batch_count > 1`。**
> 该参数会让上游按张数出图，而网关对图像工具是**按次计费**，无法对多张产物正确计价。因此网关会在**计费发生之前**直接拒绝 `batch_count` 大于 1 的请求并返回 **400**，不会产生任何扣费。请只传 `1` 或不传该参数（不传时按 1 张处理）。需要多张图时，请分多次调用。

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/generate-product-scene-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/product.png",
    "tool_version": "standard",
    "standard_scene": "exhibit_home",
    "batch_count": 1,
    "output_width": 600,
    "output_height": 600
  }'
```

专业版（`tool_version` 为 `professional`）示例：

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/generate-product-scene-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/product.png",
    "tool_version": "professional",
    "prompt": "将商品放置在浅色木质桌面上，背景为简洁明亮的室内家居场景，整体暖色调，突出商品主体",
    "professional_reference_image_url": "https://example.com/reference-image.jpeg",
    "professional_reference_image_adapt_scale": 0.8
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-generate-product-scene-image-11796531458",
  "task_type": "generate-product-scene-image",
  "result": {
    "images": [
      {
        "image_format": "png",
        "image_height": 600,
        "image_width": 600,
        "image_size": 411259,
        "image_url": "https://..."
      }
    ]
  },
  "expires_at": 1781697340,
  "request_id": "20260616195534BD8669B6807C72600EE2"
}
```

`result.images` 是数组，每个元素对应一张生成图。受上述 `batch_count` 限制，经本网关调用时数组中只会有一项。

**产物字段：** `result.images[].image_url`

---

### 5.13 图像暗水印-添加 `add-image-invisible-watermark`

**作用：** 把一段自定义文本（如版权归属、渠道号、用户 ID）以肉眼不可见的方式嵌入图片像素中，输出画质基本无损但已携带隐藏信息的新图，且水印在被缩放、压缩、转码后仍有较大概率可被提取。适合版权保护、AIGC 生图合规标识、传播链路追踪、搬运取证等场景。

- **端点：** `POST /v1/mediakit/sync/add-image-invisible-watermark`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待嵌入水印的图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 等协议，格式支持 `.png`、`.jpg`、`.jpeg`、`.webp` |
| `tool_version` | string | 是 | 暗水印模型版本，须显式指定，文档示例为 `adapt`、`adapt_resize`。**请记录该值，提取水印时必须传入相同版本** |
| `watermark_content` | string | 是 | 自定义暗水印文本内容。最长 128 字符；若全为中文，建议不超过 42 个字 |
| `strength_level` | string | 否 | 暗水印强度：`low`（低，适合纯色图或对画质要求高的场景）、`medium`（默认，画质与抗攻击性平衡）、`strong`（高，适合纹理丰富的图，抗攻击性最强） |
| `output_format` | string | 否 | 输出图片格式，文档示例为 `webp`（推断） |
| `output_quality` | int | 否 | 输出图片质量，文档示例为 85（推断） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/add-image-invisible-watermark' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/source_image.jpg",
    "tool_version": "adapt",
    "watermark_content": "copyright-2026",
    "strength_level": "medium",
    "output_format": "webp",
    "output_quality": 85
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-add-image-invisible-watermark-1703200",
  "task_type": "add-image-invisible-watermark",
  "request_id": "20260415150000F6DE4C24A6A0D94B7FF2",
  "result": {
    "image_url": "https://...",
    "image_size": 875321,
    "image_format": "png",
    "image_width": 2048,
    "image_height": 1536
  },
  "expires_at": 1776274800
}
```

**产物字段：** `result.image_url`

---

### 5.14 图像暗水印-提取 `extract-image-invisible-watermark`

**作用：** 输入一张疑似被泄露或搬运的图片，解析其中由 MediaKit 暗水印添加接口嵌入的隐藏文本，把当初写入的内容（版权方、渠道号、用户 ID 等）原样还原出来，用于溯源比对与维权取证。仅能提取本服务添加的水印。

- **端点：** `POST /v1/mediakit/sync/extract-image-invisible-watermark`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待提取水印的图片地址，通常是调用 1.13 添加接口后返回的 `result.image_url`。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 等协议 |
| `tool_version` | string | 是 | 暗水印模型版本，**必须与添加水印时（1.13）使用的版本完全一致**（如 `adapt`、`adapt_resize`），否则可能无法正确提取 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/extract-image-invisible-watermark' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/watermarked_output.png",
    "tool_version": "adapt"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-extract-image-invisible-watermark-1703201",
  "task_type": "extract-image-invisible-watermark",
  "request_id": "20260415150030F6DE4C24A6A0D94B7FF2",
  "result": {
    "watermark_content": "copyright-2026"
  },
  "expires_at": 1776274830
}
```

**产物字段：** 无文件产物，水印内容见上方响应示例的 `result.watermark_content`

---

### 5.15 图像圆角 `round-corner-image`

**作用：** 把矩形图片的四个角切成圆角，输出圆角图：`corner_type` 为正圆时四角是四分之一圆，为椭圆时四角是椭圆弧；角外区域按输出格式做透明或白色填充。适合头像/卡片圆角化、电商主图与 Banner 的视觉统一、UI 素材批量切圆角。

- **端点：** `POST /v1/mediakit/sync/round-corner-image`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `corner_type` | string | 否 | 圆角类型：`circle`（默认，正圆圆角，用 `circle_radius` 配半径）、`ellipse`（椭圆圆角，用 `ellipse_radius_x` / `ellipse_radius_y` 配半径） |
| `circle_radius` | int | 否 | 正圆半径（px）。`corner_type` 为 `circle` 时必填。取值 [0, 原图最小边/2]，超出按最大内切圆半径处理 |
| `ellipse_radius_x` | int | 否 | 椭圆圆角 X 轴（水平）半径（px）。`corner_type` 为 `ellipse` 时必填，取值 ≥ 0 |
| `ellipse_radius_y` | int | 否 | 椭圆圆角 Y 轴（垂直）半径（px）。`corner_type` 为 `ellipse` 时必填，取值 ≥ 0 |
| `output_format` | string | 否 | 输出格式，默认 `webp`：`original`（保持原图格式）、`png` / `webp`（圆角外区域透明填充）、`jpeg`（圆角外区域白色填充） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/round-corner-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/input_image.png",
    "corner_type": "circle",
    "circle_radius": 50,
    "output_format": "png"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-round-corner-image-1234567",
  "task_type": "round-corner-image",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 987654,
    "image_format": "png",
    "image_width": 1920,
    "image_height": 1080
  }
}
```

**产物字段：** `result.image_url`

---

### 5.16 图像翻转 `flip-image`

**作用：** 把整张图片做镜像翻转：`horizontal` 左右镜像（等价于照镜子），`vertical` 上下颠倒；像素内容不变、只有方向改变。适合修正手机前置摄像头自拍照的镜像脸、素材镜像加工、扫描件/排版方向纠偏。

- **端点：** `POST /v1/mediakit/sync/flip-image`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `flip_type` | string | 是 | 翻转类型：`horizontal`（水平翻转，左右镜像）、`vertical`（竖直翻转，上下翻转） |
| `output_format` | string | 否 | 输出格式，默认 `original`（保持原图格式），可选 `png` / `jpeg` / `webp` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/flip-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/input_image.jpg",
    "flip_type": "horizontal",
    "output_format": "jpeg"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-flip-image-1234567",
  "task_type": "flip-image",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 890123,
    "image_format": "jpeg",
    "image_width": 1920,
    "image_height": 1080
  }
}
```

**产物字段：** `result.image_url`

---

### 5.17 图像旋转 `rotate-image`

**作用：** 按指定角度把图片**逆时针**旋转，非 90 度整数倍旋转时四周出现的空白区域由 `fill_color` 决定填充黑、白还是透明。适合校正扫描件与照片的摆放方向（90/180/270 度）、把歪掉的版面转正、制作倾斜构图的创意素材。

- **端点：** `POST /v1/mediakit/sync/rotate-image`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `rotate_angle` | int | 是 | 逆时针旋转角度，取值 (0, 360) |
| `fill_color` | string | 否 | 旋转后空白区域的填充色，默认 `Black`：`Black`（黑）、`White`（白）、`Transparent`（透明，建议配合 `png` 输出） |
| `output_format` | string | 否 | 输出格式，默认 `webp`：`original`（保持原图格式）、`png` / `jpeg` / `webp` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/rotate-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/input_image.jpg",
    "rotate_angle": 90,
    "fill_color": "White",
    "output_format": "png"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-rotate-image-1234567",
  "task_type": "rotate-image",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 1024588,
    "image_format": "png",
    "image_width": 1080,
    "image_height": 1920
  }
}
```

**产物字段：** `result.image_url`

---

### 5.18 图像锐化 `sharpen-image`

**作用：** 增强图片的边缘细节，让画面更清晰锐利，提供轻/中/重三档强度。适合压缩过度或翻拍发虚的图片回锐、商品图与封面图突出材质细节、缩图后补偿清晰度；强度过高会产生白边与噪点，按素材试档。

- **端点：** `POST /v1/mediakit/sync/sharpen-image`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `sharpen_level` | string | 否 | 锐化强度档位，默认 `low`：`low`（轻度）、`medium`（中度）、`high`（重度） |
| `output_format` | string | 否 | 输出格式，默认 `original`（保持原图格式），可选 `png` / `jpeg` / `webp` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/sharpen-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/input_image.jpg",
    "sharpen_level": "medium",
    "output_format": "jpeg"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-sharpen-image-1234567",
  "task_type": "sharpen-image",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 890123,
    "image_format": "jpeg",
    "image_width": 1920,
    "image_height": 1080
  }
}
```

**产物字段：** `result.image_url`

---

### 5.19 图像高斯模糊 `gaussian-blur-image`

**作用：** 对整张图片施加高斯模糊，`blur_strength` 数值越大越糊。适合人像背景虚化、截图里的敏感信息弱化、给封面图留出压字区域做底、需要「整体柔化」的创意效果；模糊是全图生效，只想去掉局部信息请改用图像打码或图像擦除修复。

- **端点：** `POST /v1/mediakit/sync/gaussian-blur-image`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `blur_strength` | int | 否 | 高斯模糊强度，默认 `10`，取值 [1, 100]。数值越大越模糊，推荐 10（轻度）/ 30（中度）/ 100（重度） |
| `output_format` | string | 否 | 输出格式，默认 `original`（保持原图格式），可选 `png` / `jpeg` / `webp` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/gaussian-blur-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/input_image.jpg",
    "blur_strength": 30,
    "output_format": "jpeg"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-gaussian-blur-image-1234567",
  "task_type": "gaussian-blur-image",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 987654,
    "image_format": "jpeg",
    "image_width": 1920,
    "image_height": 1080
  }
}
```

**产物字段：** `result.image_url`

---

### 5.20 图像打码（马赛克）`mosaic-image`

**作用：** 给图片打马赛克：可以整图打码，也可以只对最多 3 个指定矩形区域打码，像素格可选圆形（柔和）或矩形（规整）。适合证件号/手机号/名片等隐私信息脱敏、评论与聊天截图脱敏、需要保留画面其余部分的局部遮挡场景。

- **端点：** `POST /v1/mediakit/sync/mosaic-image`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `mosaic_type` | string | 否 | 打码范围：`full-image`（默认，整图打码）、`specify-region`（仅对 `mosaic_regions` 指定区域打码） |
| `mosaic_regions` | array[object] | 否 | 打码矩形坐标列表，`mosaic_type` 为 `specify-region` 时必填，最多 3 个区域 |
| `mosaic_regions[].top_left_x` | int | 是 | 矩形左上角 X 坐标（px），原点为图像左上角，≥ 0 |
| `mosaic_regions[].top_left_y` | int | 是 | 矩形左上角 Y 坐标（px），原点为图像左上角，≥ 0 |
| `mosaic_regions[].bottom_right_x` | int | 是 | 矩形右下角 X 坐标（px），原点为图像左上角，≥ 0 |
| `mosaic_regions[].bottom_right_y` | int | 是 | 矩形右下角 Y 坐标（px），原点为图像左上角，≥ 0 |
| `mosaic_shape` | string | 否 | 像素格形状，默认 `circle`：`circle`（圆形/椭圆像素格，视觉更柔和）、`rectangle`（矩形像素格，遮挡更规整） |
| `mosaic_step_x` | int | 否 | 像素格宽度（px），默认 `12`，需 ≥ 1。数值越大颗粒感越强 |
| `mosaic_step_y` | int | 否 | 像素格高度（px），默认 `12`，需 ≥ 1。数值越大颗粒感越强 |
| `output_format` | string | 否 | 输出格式，默认 `original`（保持原图格式），可选 `png` / `jpeg` / `webp` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/mosaic-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/privacy_image.jpg",
    "mosaic_type": "specify-region",
    "mosaic_regions": [
      {
        "top_left_x": 100,
        "top_left_y": 150,
        "bottom_right_x": 300,
        "bottom_right_y": 250
      }
    ],
    "mosaic_shape": "rectangle",
    "mosaic_step_x": 16,
    "mosaic_step_y": 16,
    "output_format": "jpeg"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-mosaic-image-1234567",
  "task_type": "mosaic-image",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 887654,
    "image_format": "jpeg",
    "image_width": 1920,
    "image_height": 1080
  }
}
```

**产物字段：** `result.image_url`

---

### 5.21 图像添加图文水印 `add-image-watermark`

**作用：** 在图片的任意位置叠加文字水印或图片（Logo）水印，文字水印可指定内容、字体、字号、颜色与不透明度，图片水印可指定 Logo 地址与不透明度，两者都支持九宫格定位、偏移微调以及整图平铺。适合版权署名、品牌 Logo 铺底、素材防盗与内部资料标识。

- **端点：** `POST /v1/mediakit/sync/add-image-watermark`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `watermark_type` | string | 否 | 水印类型，默认 `text`：`text`（文字水印）、`image`（图片水印） |
| `watermark_text` | string | 否 | 水印文字内容，`watermark_type` 为 `text` 时必填，不超过 64 个字符 |
| `watermark_text_font` | string | 否 | 文字字体，默认 `SourceHanSans-Regular.ttf`（思源黑体）。支持思源黑体、思源宋体、站酷、方正兰亭黑等系列，取值为字体文件名 |
| `watermark_text_font_size` | int | 否 | 文字字号（px），默认 `30`，取值 [1, 200] |
| `watermark_text_color` | string | 否 | 文字颜色，支持十六进制（如 `#FF0000`）与 RGB 格式，默认 `#FFFFFF`（白色） |
| `watermark_text_opacity` | int | 否 | 文字水印不透明度，取值 [0, 100]，默认 `30`，值越小越透明 |
| `watermark_image_url` | string | 否 | 图片水印地址，`watermark_type` 为 `image` 时必填。必须是公网可访问的 HTTP/HTTPS URL，建议 ≤ 5 MB，支持 `.jpg` / `.jpeg` / `.webp` 等格式 |
| `watermark_image_opacity` | int | 否 | 图片水印不透明度，取值 [0, 100]，默认 100，值越小越透明 |
| `enable_tile` | bool | 否 | 是否平铺水印，默认 `false`。开启后水印按固定间距铺满整图；文字水印会额外逆时针旋转 30 度，图片水印只平铺不旋转 |
| `watermark_position` | string | 否 | 九宫格位置，默认 `bottom_right`：`top_left` / `top_center` / `top_right` / `left_center` / `center` / `right_center` / `bottom_left` / `bottom_center` / `bottom_right` |
| `watermark_position_offset_x` | int | 否 | 水平偏移量（px），默认 `0`，需 ≥ 0。取值含 `center` 的位置（如 `top_center`、`center`）时该参数不生效 |
| `watermark_position_offset_y` | int | 否 | 垂直偏移量（px），默认 `0`，需 ≥ 0。取值含 `center` 的位置时该参数不生效 |
| `output_format` | string | 否 | 输出格式，默认 `original`（保持原图格式），可选 `png` / `jpeg` / `webp` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/add-image-watermark' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/background.jpg",
    "watermark_type": "text",
    "watermark_text": "© MyBrand",
    "watermark_text_font_size": 24,
    "watermark_text_color": "#FF0000",
    "watermark_text_opacity": 50,
    "watermark_position": "bottom_right",
    "watermark_position_offset_x": 20,
    "watermark_position_offset_y": 20
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-add-image-watermark-1234567",
  "task_type": "add-image-watermark",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 310450,
    "image_format": "jpeg",
    "image_width": 1920,
    "image_height": 1080
  }
}
```

**产物字段：** `result.image_url`

---

### 5.22 图像调整（亮度/饱和度等）`adjust-image-color`

**作用：** 一次调用给图片做一种色彩调整：调亮或调暗、增强或减弱对比度、增强或减弱饱和度，由 `adjust_type` 单选取值。适合修复曝光过暗/过亮的废片、让弱对比的素材更通透、给商品图做饱和度标准化，或把一批素材的色彩风格拉齐。

- **端点：** `POST /v1/mediakit/sync/adjust-image-color`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `adjust_type` | string | 是 | 调整类型，一次调用只能选一种：`increase_brightness`（调亮）、`decrease_brightness`（调暗）、`increase_contrast`（增强对比度）、`decrease_contrast`（减弱对比度）、`increase_saturation`（增强饱和度）、`decrease_saturation`（减弱饱和度） |
| `output_format` | string | 否 | 输出格式，默认 `original`（保持原图格式），可选 `png` / `jpeg` / `webp` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/adjust-image-color' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/input_image.png",
    "adjust_type": "increase_contrast",
    "output_format": "jpeg"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-adjust-image-color-1234567",
  "task_type": "adjust-image-color",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 245310,
    "image_format": "jpeg",
    "image_width": 1280,
    "image_height": 720
  }
}
```

**产物字段：** `result.image_url`

---

### 5.23 图像缩放 `resize-image`

**作用：** 按长边/短边目标尺寸等比缩放图片，或强制缩放到指定宽高；还能限定只放大或只缩小。适合批量生成缩略图与多规格尺寸、App/网页自适应出图、内容合规检查前把超大图压到可处理尺寸。

- **端点：** `POST /v1/mediakit/sync/resize-image`
- **计费：** 按量 GiB

> 上游该工具的提交路径为 `resize-image`（火山引擎文档里写作 `scale-image`，该路径上游已返回 NotFound），本网关按 `resize-image` 路由。

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `resize_long` | int | 否 | 目标长边尺寸（px）。只设此参数（或将 `resize_short` 设为 0）时按原比例缩放至长边等于该值。与 `resize_short` 至少要有一个大于 0 |
| `resize_short` | int | 否 | 目标短边尺寸（px）。只设此参数（或将 `resize_long` 设为 0）时按原比例缩放至短边等于该值。与 `resize_long` 至少要有一个大于 0 |
| `resize_mode` | string | 否 | 同时指定 `resize_long` 与 `resize_short` 时的缩放行为，默认 `contain`：`exact`（强制缩放到目标宽高，可能拉伸变形）、`contain`（等比缩放并完整装入目标框，宽高均不超过目标值）、`cover`（等比缩放并填满目标框，超出部分居中裁剪） |
| `resize_adaptive` | array[string] | 否 | 缩放生效条件，默认 `["enlarge", "shrink"]`（总是缩放）：`enlarge`（仅当原图小于目标尺寸时放大）、`shrink`（仅当原图大于目标尺寸时缩小） |
| `output_format` | string | 否 | 输出格式，默认 `original`（保持原图格式），可选 `png` / `jpeg` / `webp` |
| `width` | int | 否 | 只指定宽度（px）时按原图比例等比缩放（推断：文档快速体验示例中的简写写法，官方 API 参考的正式字段为 `resize_long` / `resize_short`） |
| `height` | int | 否 | 只指定高度（px）时按原图比例等比缩放（推断，同上） |
| `scale` | number | 否 | 直接给出缩放倍数，如 `2` 表示长宽均放大 2 倍（推断，同上） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/resize-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/input_image.jpg",
    "resize_long": 200
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-resize-image-1234567",
  "task_type": "resize-image",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 25432,
    "image_format": "jpeg",
    "image_width": 200,
    "image_height": 150
  }
}
```

**产物字段：** `result.image_url`

---

### 5.24 图像负片（反色）`invert-image`

**作用：** 把图片的每个像素颜色取反（如 R 通道 30 变 225），一键生成胶片底片式的反色效果。适合做艺术化滤镜、创意视觉与内容二次创作；这是纯色彩反相，不改几何形状，写实素材慎用。

- **端点：** `POST /v1/mediakit/sync/invert-image`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `output_format` | string | 否 | 输出格式，默认 `original`（保持原图格式），可选 `png` / `jpeg` / `webp` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/invert-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/input_image.jpg"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-invert-image-1234567",
  "task_type": "invert-image",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 910234,
    "image_format": "jpeg",
    "image_width": 1920,
    "image_height": 1080
  }
}
```

**产物字段：** `result.image_url`

---

### 5.25 图像裁剪 `crop-image`

**作用：** 按四种模式裁掉图片的多余部分：方向裁剪（给定目标宽高 + 九宫格方位）、定向裁剪（锚点 + 横纵偏移 + 目标宽高）、自定义裁剪（左上角/右下角坐标）、内切圆裁剪（自动裁出以短边为直径的最大内切圆）。适合生成固定尺寸的缩略图/横幅、商品图去边、主体保留式截取、圆形头像与贴纸素材。

- **端点：** `POST /v1/mediakit/sync/crop-image`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `crop_mode` | string | 否 | 裁剪模式，默认 `directional`。`directional`（方向裁剪：按目标宽高 + `crop_position` 裁剪）、`origin`（定向裁剪：锚点 + 偏移 + 目标宽高）、`custom`（自定义裁剪：按左上/右下坐标裁剪）、`circle`（最大内切圆裁剪，无需其他参数；该模式下原图最短边不得超过 2048 px） |
| `crop_width` | int | 否 | 裁剪后目标宽度（px）。`directional` 模式下与 `crop_height` 至少一个不为 0，设为 0 时该边按另一边比例自适应；`origin` 模式下必填 |
| `crop_height` | int | 否 | 裁剪后目标高度（px）。`directional` 模式下与 `crop_width` 至少一个不为 0，设为 0 时该边按另一边比例自适应；`origin` 模式下必填 |
| `crop_position` | string | 否 | 裁剪基准方位，仅 `directional` 模式使用，默认 `center`：`center`（居中，如裁 600x600 方形缩略图）、`up`（顶部，保留顶部内容做横幅）、`down`（底部）、`left`（左侧）、`right`（右侧） |
| `origin_gravity` | string | 否 | 裁剪锚点，仅 `origin` 模式使用，默认 `northwest`：`northwest` / `north` / `northeast` / `west` / `center` / `east` / `southwest` / `south` / `southeast` |
| `origin_x` | int | 否 | 相对锚点的水平偏移量（px），仅 `origin` 模式使用且必填。正值向右、负值向左 |
| `origin_y` | int | 否 | 相对锚点的垂直偏移量（px），仅 `origin` 模式使用且必填。正值向下、负值向上 |
| `custom_x1` | int | 否 | 裁剪区域左上角 X 坐标（px），仅 `custom` 模式使用且必填 |
| `custom_y1` | int | 否 | 裁剪区域左上角 Y 坐标（px），仅 `custom` 模式使用且必填 |
| `custom_x2` | int | 否 | 裁剪区域右下角 X 坐标（px），仅 `custom` 模式使用且必填 |
| `custom_y2` | int | 否 | 裁剪区域右下角 Y 坐标（px），仅 `custom` 模式使用且必填 |
| `output_format` | string | 否 | 输出格式，默认 `original`（保持原图格式），可选 `png` / `jpeg` / `webp`。内切圆裁剪建议用 `png` 或 `webp` 以保证圆外透明，输出 `jpeg` 时圆外会填充白色 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/crop-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/source_image.jpg",
    "crop_mode": "custom",
    "custom_x1": 100,
    "custom_y1": 150,
    "custom_x2": 400,
    "custom_y2": 350,
    "output_format": "png"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-crop-image-1234567",
  "task_type": "crop-image",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 188080,
    "image_format": "png",
    "image_width": 300,
    "image_height": 200
  }
}
```

**产物字段：** `result.image_url`

---

### 5.26 图像压缩 `compress-image`

**作用：** 在尽量保住观感的前提下把图片体积压下来：可指定压缩质量、输出体积上限，或转成 WebP / AVIF / HEIC 等更省的格式，也可对 PNG 启用有损瘦身。适合网站与 App 的图片分发加载提速、CDN 流量成本优化、用户上传大图的入库治理。

- **端点：** `POST /v1/mediakit/sync/compress-image`
- **计费：** 按量 GiB

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待处理图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议 |
| `output_format` | string | 否 | 输出格式，默认 `webp`，可选 `png` / `jpeg` / `webp` / `avif` / `heic`。**不传即转成 WebP**，想保持原图格式需显式传入原格式 |
| `quality` | int | 否 | 压缩质量，取值 [1, 100]，默认 `75`。数值越小压缩率越高、画质损失越大。设置了 `max_size` 时该参数被忽略 |
| `max_size` | int | 否 | 输出文件的体积上限（字节），例如 `204800` 表示 200 KB，推荐 `10485760`（10 MiB）。设置后系统自动调整压缩参数以尽量满足该上限，此时 `quality` 被忽略。仅当 `output_format` 为 `jpeg` 或 `webp` 时生效 |
| `png_lossy` | bool | 否 | 是否对 PNG 启用有损压缩（PNG 瘦身）以获得更高压缩率，默认 `false`。仅当 `output_format` 为 `png` 时生效 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/compress-image' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/large_image.png",
    "output_format": "webp",
    "quality": 80
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-compress-image-1234567",
  "task_type": "compress-image",
  "request_id": "021778401180935fdbd0ff1ce00652c087a478b3f62d63b5dbf0f",
  "expires_at": 1778573984,
  "result": {
    "image_url": "https://...",
    "image_size": 98765,
    "image_format": "webp",
    "image_width": 1920,
    "image_height": 1080
  }
}
```

**产物字段：** `result.image_url`

### 5.27 图像元信息获取 `probe-image-metadata`

**作用：** 读取一张图片的格式与基础元信息——容器格式、宽高、文件大小等，**不修改图片、不产出新文件**。默认返回基础元数据；也可切换成提取主题色、分析 Alpha 透明通道、生成 BlurHash 占位串。适合上传前的格式与尺寸校验、按体积分流处理策略、取主题色做前端占位配色、以及给图片生成轻量模糊占位图等场景。

- **端点：** `POST /v1/mediakit/sync/probe-image-metadata`
- **计费：** 按次，0.00024 元/次（近似上游 0.025 元/GiB 口径，见 [3.6](#36-计费口径)）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `image_url` | string | 是 | 待探测的图片地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种协议；支持 png/jpg/jpeg/webp 等主流格式（含动图）；宽和高均不得超过 10000 像素；建议单张不超过 35 MB |
| `info_type` | string | 否 | 查询类型，默认 `metadata`。可选：`metadata`（基础元信息）、`avghue`（十六进制主题色）、`alpha`（Alpha 透明通道信息）、`blurhash`（BlurHash 字符串） |
| `queue_id` | string | 否 | 任务提交的目标队列 ID。不传则用系统自动创建的队列 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/sync/probe-image-metadata' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "image_url": "https://example.com/your_image.png"
  }'
```

**响应**

同步工具，结果直接在本接口响应里返回：

```json
{
  "success": true,
  "task_id": "amk-tool-probe-image-metadata-12345",
  "request_id": "021778401180935fdbd0ff1ce00652c087a",
  "result": {
    "format": "png",
    "width": 1920,
    "height": 1080
  }
}
```

`result` 的形状随 `info_type` 变化：`avghue` 返回十六进制主题色，`alpha` 返回透明通道信息，`blurhash` 返回一段 BlurHash 字符串。

**产物字段：** 无文件产物，结果结构见上方响应示例


---

## 6. 视频工具（异步，34 个）

全部为异步工具，端点为 `POST /v1/mediakit/{tool}`。提交后拿 `task_id`，轮询 `GET /v1/mediakit/tasks/{task_id}` 取结果，流程见 [2.2](#22-异步调用视频--音频工具)。

除「视频抽帧」「视频元信息获取」按次计费外，其余均按处理时长计费。

### 6.1 画质增强（标准版/专业版） `enhance-video`

**作用：** 把低分辨率、有噪点/压缩损伤或老旧模糊的视频，通过超分、插帧、降噪、色彩增强等算子组合提升到 1080p / 2K / 4K 甚至 8K 的清晰画面，并能顺带提升帧率。适合 AIGC 生成视频（如 Seedance 直出的 720p）出片后处理、短剧与 UGC 短视频画质修复、老片修复、大屏投放与商业交付等场景。标准版性价比优先，专业版算力密度更高、支持 10/12/16-bit 高位深输出。

- **端点：** `POST /v1/mediakit/enhance-video`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待增强视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/wmv/mkv 等主流格式，单文件建议不超过 10 GB；输入分辨率最高 2K（短边 [360,1440]，长边 [360,2560]） |
| `tool_version` | string | 否 | 增强版本：`standard`（标准版，默认，10+ 常用算法，速度与画质平衡）、`professional`（专业版，30+ 深度 AI 算法，镜头级画质，耗时为标准版的数倍） |
| `scene` | string | 否 | 业务场景预设模板，**仅标准版（`tool_version: standard`）生效**：`aigc`（大模型生成视频超分到 2K/4K 并重绘细节）、`short_series`（短剧人像增强）、`ugc`（修复多次压缩导致的模糊/块效应/色带）、`old_film`（老片时域去噪、划痕去除、消闪烁、修色偏） |
| `enhance_style` | string | 否 | 增强风格，标准版与专业版均支持：`hd`（默认，高清，增强强度适中、锐利清晰）、`natural`（自然，降低强度、锐化痕迹更少） |
| `resolution` | string | 否 | 输出分辨率预设规格（如 `720p`、`1080p`、`4k`、`8k`，最高支持 8k），系统自动对齐到该规格。与 `resolution_limit` **互斥**，不可同时配置 |
| `resolution_limit` | int | 否 | 锁定输出短边像素值，保持原宽高比等比缩放（例：640x480 输入设 720，输出短边 720、宽 960）。取值范围 [128, 4320]。与 `resolution` **互斥**，不可同时配置 |
| `fps` | int | 否 | 输出帧率，最高支持 120 fps。高于原帧率时会自动触发 AI 智能插帧；建议不超过原帧率的 4 倍。不传则保持与片源一致 |
| `bitrate_level` | string | 否 | 码率档位：`low`（低）、`medium`（中，推荐，默认）、`high`（高）。实际码率结合目标分辨率与帧率自动换算。与 `bitrate` 同时指定时仅 `bitrate` 生效 |
| `bitrate` | int | 否 | 精确指定目标平均码率，单位 kbps，取值范围 [10, 150000]。不填时按 `bitrate_level` 结合分辨率与帧率自动换算 |
| `bit_depth` | int | 否 | 输出色深，**仅专业版生效**：`8`（默认，MP4）、`10`（MP4）、`12`（MP4）、`16`（H.265，输出 MOV，输入视频需不超过 40 秒，此时 `bitrate`/`bitrate_level` 均不生效）。12/16-bit 最终会降维到 10-bit 输出 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/enhance-video' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/source_720p.mp4",
    "tool_version": "professional",
    "resolution": "1080p",
    "enhance_style": "natural",
    "fps": 60,
    "bitrate": 8000,
    "bit_depth": 10
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-enhance-video-1755136",
  "task_type": "enhance-video",
  "status": "completed",
  "result": {
    "duration": 5.967,
    "fps": 30.168,
    "resolution": "1080p",
    "tool_version": "standard",
    "video_url": "https://..."
  },
  "expires_at": 1775960452,
  "created_at": 1775787510,
  "finished_at": 1775787652,
  "request_id": "20260410102611A2F6BCCB40A5B534A302"
}
```

**产物字段：** `result.video_url`

---

### 6.2 画质增强（大模型版） `enhance-video-generative`

**作用：** 用 Diffusion 扩散大模型对视频做生成式增强修复：不只修复压缩/老化损失的像素，还能基于语义理解补全和重绘缺失细节（人脸五官、织物纹理等），把 480p 低清视频还原到接近 1080p 直出质感。适合 AIGC 视频降本工作流（低清生成 + 后期增强，仅对最终成片增强）、老片与压缩损伤视频的细节重建等场景。

- **端点：** `POST /v1/mediakit/enhance-video-generative`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待增强视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；单文件建议不超过 10 GB；**输入分辨率最高 1080p**（短边 [360,1080]，长边 [360,1920]）；**仅支持 SDR 视频** |
| `resolution` | string | 否 | 输出分辨率档位：`720p`（默认，标准高清、性价比最高）、`1080p`（全高清）、`2k`（超高清，适合大屏与高要求修复） |
| `fps` | int | 否 | 输出帧率，最高支持 120 fps。高于原帧率时自动触发 AI 智能插帧，建议不超过原帧率的 4 倍。不传则保持与片源一致 |
| `bitrate_level` | string | 否 | 码率档位：`low`（低）、`medium`（中，推荐，默认）、`high`（高）。实际码率结合目标分辨率与帧率自动换算。与 `bitrate` 同时指定时仅 `bitrate` 生效 |
| `bitrate` | int | 否 | 精确指定目标平均码率，单位 kbps，取值范围 [10, 150000]。不填时按 `bitrate_level` 结合分辨率与帧率自动换算 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/enhance-video-generative' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/source_480p.mp4",
    "resolution": "1080p",
    "fps": 60,
    "bitrate": 8000
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-enhance-video-generative-987654",
  "request_id": "20240522110000ABCDEFG67890",
  "status": "completed",
  "result": {
    "video_url": "https://...",
    "duration": 65.5,
    "resolution": "1080p",
    "fps": 30
  },
  "expires_at": 1777446399,
  "created_at": 1777358413,
  "finished_at": 1777359999
}
```

**产物字段：** `result.video_url`

---

### 6.3 画质增强（极速版） `enhance-video-fast`

**作用：** 用轻量级超分 + 智能画质增强算法，以速度优先策略快速把低清晰度视频提升到 720p/1080p/4K，在画质有效改善的同时大幅缩短等待时间。适合直播、短视频批量处理等对时延敏感、且不需要极致修复效果的场景。

- **端点：** `POST /v1/mediakit/enhance-video-fast`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待增强视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/wmv/mkv 等主流格式，单文件建议不超过 10 GB；输入分辨率最高 2K（短边 [360,1440]，长边 [360,2560]） |
| `resolution` | string | 否 | 输出分辨率预设规格（如 `720p`、`1080p`、`4k`，**最高支持 4k**），系统自动对齐到该规格。与 `resolution_limit` **互斥**，不可同时配置 |
| `resolution_limit` | int | 否 | 锁定输出短边像素值，保持原宽高比等比缩放（例：640x480 输入设 720，输出短边 720、宽 960）。取值范围 [128, 2160]。与 `resolution` **互斥**，不可同时配置 |
| `fps` | int | 否 | 目标帧率，取值范围 [15, 120]。不指定则尽量保持与原片一致；建议不超过原视频帧率的 4 倍 |
| `bitrate_level` | string | 否 | 码率档位：`low`（低）、`medium`（中，推荐，默认）、`high`（高）。实际码率结合目标分辨率与帧率自动换算。与 `bitrate` 同时指定时仅 `bitrate` 生效 |
| `bitrate` | int | 否 | 精确指定目标平均码率，单位 kbps，取值范围 [10, 150000]。不填时按 `bitrate_level` 结合分辨率与帧率自动换算 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/enhance-video-fast' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/source_video.mp4",
    "resolution": "1080p",
    "fps": 30,
    "bitrate": 8000
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-enhance-video-fast-987654",
  "request_id": "20240523110000ABCDEFG67890",
  "status": "completed",
  "result": {
    "video_url": "https://...",
    "duration": 65.5,
    "resolution": "720p",
    "fps": 30
  },
  "expires_at": 1777531298,
  "created_at": 1777358413,
  "finished_at": 1777359999
}
```

**产物字段：** `result.video_url`

---

### 6.4 字幕擦除（精细化版） `erase-video-subtitle-pro`

**作用：** 自动检测视频画面中内嵌的硬字幕（画面下方、居中、白色、高度占画面 1%～10% 的文字）并擦除，再用 AIGC 修复算法按帧重建被擦区域的纹理，实现无痕清除；也支持用归一化矩形框手动指定擦除区域。适合短剧出海/内容翻译前擦掉原字幕、混剪二创去字幕、AIGC 生成视频的意外文字修正、以及擦除固定位置的水印/台标/人名条等场景。

- **端点：** `POST /v1/mediakit/erase-video-subtitle-pro`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待处理视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/wmv/mkv 等主流格式；输入分辨率最高 2K；OCR 目前仅针对中英文优化 |
| `mode` | string | 否 | 擦除模式：`Subtitle`（字幕模式，OCR 自动检测并擦除画面下半部分、横向居中、白色的字幕）、`Text`（文本模式，擦除检测到的所有类型文本，范围不受「画面下半部分」限制，适合擦水印/台标/人名条等固定元素） |
| `model_version` | string | 否 | 擦除算法版本：`v4`（默认）、`v5`（最新，优化了字幕闪烁、带阴影字幕与误擦问题，处理更快） |
| `erase_ratio_location` | array[object] | 否 | 手动指定擦除框数组，可追加多个框。坐标为相对画面宽高的归一化比例，原点 (0,0) 为画面左上角、(1,1) 为右下角，取值范围 0.0～1.0。与 `mode` 联动：`Text` 模式下擦除框即最终擦除区域（可置于任意位置）；`Subtitle` 模式下实际擦除区域为擦除框与画面下半部分（Y ≥ 0.5）的重叠部分 |
| `erase_ratio_location[].top_left_x` | float | 否 | 擦除框左上角 X 归一化坐标 |
| `erase_ratio_location[].top_left_y` | float | 否 | 擦除框左上角 Y 归一化坐标 |
| `erase_ratio_location[].bottom_right_x` | float | 否 | 擦除框右下角 X 归一化坐标，需大于 `top_left_x` |
| `erase_ratio_location[].bottom_right_y` | float | 否 | 擦除框右下角 Y 归一化坐标，需大于 `top_left_y` |
| `time_segment_filter` | object | 否 | 时间段过滤，用于只擦正片、保留片头片尾字幕 |
| `time_segment_filter.mode` | string | 否 | `selected`（仅对列出时间段执行擦除）、`skip`（跳过列出时间段，擦除其余部分） |
| `time_segment_filter.segments` | array[object] | 否 | 时间段数组，每段由 `start_time` / `end_time` 定义 |
| `time_segment_filter.segments[].start_time` | number | 否 | 时间段起始时间，单位：秒 |
| `time_segment_filter.segments[].end_time` | number | 否 | 时间段结束时间，单位：秒，需大于 `start_time` |
| `subtitle_filter` | object | 否 | 字幕特征过滤，仅在 `Subtitle` 模式下生效，用于微调字幕判定规则 |
| `subtitle_filter.min_text_height_ratio` | float | 否 | 文字高度最小比例（相对视频高度），默认 0.01；低于此高度的文本不判定为字幕 |
| `subtitle_filter.max_text_height_ratio` | float | 否 | 文字高度最大比例（相对视频高度）；高于此高度的文本不判定为字幕。不传时默认值与算法版本有关：v4 为 0.2，v5 为 0.1 |
| `subtitle_filter.center_offset_ratio` | float | 否 | 文字区域中心相对视频宽度中心的最大偏离比例，默认 0.08；超过该偏离比例的文本不判定为字幕 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/erase-video-subtitle-pro' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/source_video.mp4",
    "mode": "Text",
    "model_version": "v5",
    "erase_ratio_location": [
      {
        "top_left_x": 0.62,
        "top_left_y": 0.86,
        "bottom_right_x": 0.97,
        "bottom_right_y": 0.96
      }
    ],
    "time_segment_filter": {
      "mode": "selected",
      "segments": [
        { "start_time": 10, "end_time": 60 }
      ]
    },
    "subtitle_filter": {
      "min_text_height_ratio": 0.01,
      "max_text_height_ratio": 0.15,
      "center_offset_ratio": 0.08
    }
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-erase-video-subtitle-pro-123456789",
  "task_type": "erase-video-subtitle-pro",
  "status": "completed",
  "result": {
    "video_url": "https://...",
    "duration": 180.5
  },
  "expires_at": 1775047666,
  "created_at": 1774874853,
  "finished_at": 1774874866,
  "request_id": "1742012dcc1938455d84dbe56beb7acc"
}
```

**产物字段：** `result.video_url`

---

### 6.5 视频抠图（人像） `matte-portrait-video`

**作用：** 自动识别视频中的人物主体，把人物从原始背景里抠出来，输出背景透明（WebM/MOV 带 Alpha 通道）或纯色背景（MP4）的视频素材，用于后续换背景、叠加到课件/特效上做二次合成。适合直播与视频会议的虚拟背景、影视与短视频创意合成、电商商品视频换背景、教学画中画等场景。如果素材本身是绿幕或纯色背景拍摄，改用绿幕抠图效果更精细。

- **端点：** `POST /v1/mediakit/matte-portrait-video`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待抠图视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/mkv/wmv 等主流格式，单文件建议不超过 10 GB |
| `format` | string | 否 | 输出格式：`WEBM`（默认，透明背景，兼容好、体积小）、`MOV`（透明背景，无损 Alpha 通道，体积大）、`MP4`（纯色背景，不透明） |
| `background_color` | string | 否 | 背景颜色，**仅 `format` 为 `MP4` 时生效**：`black`（黑）、`white`（白）、`green`（绿）。人像抠图在 `format: MP4` 且未传本参数时默认 `green` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/matte-portrait-video' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_portrait_video.mp4",
    "format": "MP4",
    "background_color": "white"
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-matte-portrait-video-123456789",
  "status": "completed",
  "result": {
    "video_url": "https://...",
    "resolution": "1080p",
    "duration": 15.5
  }
}
```

**产物字段：** `result.video_url`

---

### 6.6 视频抠图（绿幕） `matte-greenscreen-video`

**作用：** 针对在绿幕或纯色背景下拍摄的视频，自动识别并保留主体（人物、物品、动物等），移除绿幕/纯色背景，输出背景透明或纯色背景的视频素材。相比人像抠图，它利用纯色背景先验，在绿幕/影棚素材上边缘处理更精细，适合虚拟演播室、直播带货、影视特效合成、电商商品换背景等场景。

- **端点：** `POST /v1/mediakit/matte-greenscreen-video`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待抠图视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/mkv/wmv 等主流格式，单文件建议不超过 10 GB |
| `format` | string | 否 | 输出格式：`WEBM`（默认，透明背景，兼容好、体积小）、`MOV`（透明背景，无损 Alpha 通道，体积大）、`MP4`（纯色背景，不透明） |
| `background_color` | string | 否 | 背景颜色，**仅 `format` 为 `MP4` 时生效**：`black`（黑）、`white`（白）、`green`（绿）。绿幕抠图在 `format: MP4` 且未传本参数时默认 `black` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/matte-greenscreen-video' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_greenscreen_video.mp4",
    "format": "MP4",
    "background_color": "black"
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-matte-greenscreen-video-123456789",
  "status": "completed",
  "result": {
    "video_url": "https://...",
    "resolution": "1080p",
    "duration": 15.5
  }
}
```

**产物字段：** `result.video_url`

---

### 6.7 语音转字幕（ASR） `asr-subtitles`

**作用：** 听声音——用自动语音识别（ASR）把视频/音频里的**人声对话**转写成带时间戳的结构化字幕数据（每句的起止时间 + 文本），可选区分不同说话人。适合给短视频/长视频/直播回放自动配字幕、会议与访谈录音转写归档、播客与有声书内容提取检索等场景。与 2.8 视频识别字幕（OCR）的区别：本工具识别的是**语音（音轨里的人声）**，OCR 识别的是**画面里固化显示的文字**，两者输入相同但数据来源完全不同。

- **端点：** `POST /v1/mediakit/asr-subtitles`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 否 | 待识别视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议。与 `audio_url` 二选一，纯视频输入时传本字段 |
| `audio_url` | string | 否 | 待识别纯音频文件地址，协议支持同上。与 `video_url` 二选一，用于处理 mp3 / m4a / wav 等纯音频输入 |
| `language` | string | 否 | 指定识别语种。不传由算法自动判断；示例取值为 `eng-US`（美式英语）。当前版本仅支持识别简体中文与英语（推断） |
| `enable_speaker_info` | boolean | 否 | 是否开启说话人识别（说话人日志）。设为 `true` 后，返回的每条字幕会额外带 `speaker` 字段区分说话人。默认 `false` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/asr-subtitles' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_video.mp4",
    "language": "eng-US",
    "enable_speaker_info": true
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-asr-subtitles-123456789",
  "status": "completed",
  "result": {
    "duration": 8.0,
    "subtitles": [
      {
        "start_time": 0.5,
        "end_time": 2.8,
        "subtitle_text": "Hello everyone, and welcome to today's session.",
        "speaker": "speaker_0"
      },
      {
        "start_time": 6.2,
        "end_time": 8.0,
        "subtitle_text": "Indeed, this is a very interesting topic.",
        "speaker": "speaker_1"
      }
    ]
  }
}
```

**产物字段：** 无文件产物，字幕见上方响应示例的 `result.subtitles`

---

### 6.8 视频识别字幕（OCR） `video-ocr`

**作用：** 看画面——用视频光学字符识别（OCR）把**固化在视频画面里的硬字幕**（烧录字、无法作为文本轨道访问的字）按帧识别出来，输出带时间戳的结构化文本。适合从无字幕轨的视频里提取字幕再生成 SRT/WebVTT、建立视频内容关键词索引、敏感词与版权信息等合规审核、视觉文本数据化分析等场景。与 2.7 语音转字幕（ASR）的区别：本工具读的是**画面像素上的文字**，听不到声音，因此对无对白但有大量屏幕文字的素材同样有效。

- **端点：** `POST /v1/mediakit/video-ocr`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待识别视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议 |
| `mode` | string | 否 | 识别模式：`Subtitle`（默认，仅识别画面常规字幕位置的文本）、`Detailed`（详细模式，识别画面中所有可见文本，含标题、注释、水印、台标、贴片等非字幕内容） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/video-ocr' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/scene_with_text.mp4",
    "mode": "Detailed"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-video-ocr-123456789",
  "request_id": "20240521120000ABCDEFG12345",
  "status": "completed",
  "result": {
    "duration": 55.2,
    "subtitles": [
      {
        "start_time": 1.2,
        "end_time": 3.5,
        "subtitle_text": "这是一个示例文本"
      },
      {
        "start_time": 4.0,
        "end_time": 6.8,
        "subtitle_text": "识别视频中的硬字幕"
      }
    ]
  },
  "expire_timestamp": 1774504841
}
```

`mode` 为 `Detailed` 时，`subtitles` 数组中的每个对象会额外返回两个字段：`text_label`（文本类别：`Subtitle` 字幕 / `Others` 画面其他文字）和 `text_location`（文本在画面中的像素坐标矩形，含 `top_left_x` / `top_left_y` / `bottom_right_x` / `bottom_right_y`）。

**产物字段：** 无文件产物，字幕见上方响应示例的 `result.subtitles`

---

### 6.9 场景切分 `segment-scenes`

**作用：** 按镜头语言切——依据镜头切换、转场效果和画面内容变化，把一个长视频自动切分成多个有意义的场景片段，每个片段输出起止时间轴**并单独产出一个可直接下载的切片视频文件**。适合视频拆条、集锦/混剪素材准备、按场景定位关键帧等下游应用。

- **端点：** `POST /v1/mediakit/segment-scenes`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待切分视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议 |
| `segment_threshold` | number | 否 | 场景切分敏感度阈值，范围 [0, 100]，推荐经验值 10。值越低切得越细碎（对画面变化更敏感），值越高切得越粗略（忽略不显著的场景变化） |
| `min_duration` | number | 否 | 单个切片的最小时长（秒）。小于此时长的片段会被自动合并到相邻片段。示例取值 2.5 |
| `max_duration` | number | 否 | 单个切片的最大时长（秒）。大于此时长的片段会被强制切分。示例取值 30 |
| `enable_clip_fade` | boolean | 否 | 是否将检测到的淡入/淡出片段作为独立切片输出。默认 `false` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/segment-scenes' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_long_video.mp4",
    "segment_threshold": 15,
    "min_duration": 2.5,
    "max_duration": 30,
    "enable_clip_fade": true
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-segment-scenes-123456789",
  "status": "completed",
  "result": {
    "duration": 185.5,
    "segments": [
      {
        "start_time": 0.0,
        "end_time": 15.2,
        "segment_video_url": "https://..."
      },
      {
        "start_time": 15.2,
        "end_time": 45.8,
        "segment_video_url": "https://..."
      },
      {
        "start_time": 45.8,
        "end_time": 185.5,
        "segment_video_url": "https://..."
      }
    ]
  }
}
```

**产物字段：** `result.segments[<n>].segment_video_url`（数组，每个片段一个 URL，取 `result.segments[<n>].segment_video_url`，n 从 0 开始）

---

### 6.10 智能语义切片（基础版） `semantic-segment`

**作用：** 按语义切——综合分析画面、语音与叙事结构，把一个长视频切成多个「可独立表达一段完整语义」的片段（例如不会从一句话中间切断），输出每个片段的起止时间轴。与 2.9 场景切分按转场/画面差异切不同，本工具按**语义完整性**切，更适合短视频拆条、剪辑素材召回、多模态数据标注等场景。

- **端点：** `POST /v1/mediakit/semantic-segment`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待切片视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议 |
| `min_duration` | number | 否 | 切片的目标最小时长（秒），默认 3，最小 1。小于此值的片段会与相邻切片合并（前提是合并后不超过 `max_duration`），避免产生过短碎片 |
| `max_duration` | number | 否 | 切片的目标最大时长（秒），默认 30，最小 1。超过此值将触发强制切分，保证片段不过长 |
| `max_shift_tolerance` | number | 否 | 切点偏移容忍度（秒），默认 0，最小 0。设为大于 0 时，算法会在切点前后该范围内寻找更贴近语义的位置（如句末、停顿），让切点尽量落在自然停顿处而非句子中间；值越大切点越贴合语义，但实际时长相对目标时长的抖动也越大 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/semantic-segment' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_long_video.mp4",
    "min_duration": 5,
    "max_duration": 20,
    "max_shift_tolerance": 2
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-semantic-segment-123456789",
  "status": "completed",
  "result": {
    "duration": 185.5,
    "segment_count": 12,
    "result_url": "https://.../semantic_segments.json.gz?auth_key=..."
  }
}
```

`result_url` 指向一份 gzip 压缩的 JSON 文件，下载解压后为完整切片结果，结构示意：

```json
{
  "duration_ms": 185500,
  "duration": 185.5,
  "has_audio": true,
  "segment_count": 12,
  "segments": [
    { "index": 0, "start_ms": 0, "end_ms": 12350, "start": "00:00:00.000", "end": "00:00:12.350" },
    { "index": 1, "start_ms": 12350, "end_ms": 27800, "start": "00:00:12.350", "end": "00:00:27.800" }
  ]
}
```

**产物字段：** `result.result_url`

---

### 6.11 高光片段提取 `analyze-video-highlights`

**作用：** 只分析、不出片——作为智能分析引擎深度理解视频内容，捕捉「情绪高潮」「关键动作」「高能瞬间」，输出结构化 JSON 元数据（精确时间戳、高光分数、OCR 文本、画面描述）。它**不产生任何成品视频文件**，产出的是喂给后续剪辑流程的分析结果：先按 `score` 挑出高分片段，再用裁剪/拼接工具（见 2.9 / 2.10 等）自行裁出并合成高光集锦。适合短剧、小游戏等需要批量筛选精彩片段的自动化剪辑流水线。

- **端点：** `POST /v1/mediakit/analyze-video-highlights`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_urls` | array[string] | 是 | 待分析的视频地址列表。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议 |
| `model` | string | 否 | 分析模型，按场景选择：`Miniseries`（短剧模型）、`Game`（小游戏模型）。短剧模型单次最多 30 个文件、累计时长建议不超过 45 分钟，要求画面下半部分（垂直 0.5–1.0）有清晰居中中文字幕且音轨含可识别中文对白；小游戏模型单次仅支持 1 个文件且时长不超过 10 分钟 |
| `mode` | string | 否 | 分析模式。示例取值为 `StorylineCuts`（按剧情切分高光）（推断） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/analyze-video-highlights' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_urls": [
      "https://example.com/miniseries_part1.mp4",
      "https://example.com/miniseries_part2.mp4",
      "https://example.com/miniseries_part3.mp4"
    ],
    "model": "Miniseries",
    "mode": "StorylineCuts"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-analyze-video-highlights-102762921730",
  "request_id": "20260507145242FCF48EE1F61E7DB775F1",
  "status": "completed",
  "result": {
    "duration": 342.38,
    "highlight_info": [
      {
        "source_video_index": 0,
        "start_time": 81.12,
        "end_time": 95.84,
        "score": 3.5,
        "ocr": "所谓的股票老师...背后就是个空壳公司，妥妥的垃圾，买了就是送钱。",
        "description": "主角揭露“内部消息”股票实为空壳公司的骗局。"
      },
      {
        "source_video_index": 2,
        "start_time": 34.68,
        "end_time": 48.6,
        "score": 4.0,
        "ocr": "这是我跟你哥所有的积蓄，要有什么损失，我跟你没完！",
        "description": "因投资问题，家庭内部爆发激烈争吵，戏剧冲突达到高潮。"
      }
    ]
  },
  "expires_at": 1778310069,
  "created_at": 1778136763,
  "finished_at": 1778137269
}
```

**产物字段：** 无文件产物，高光片段信息见响应示例的 `result.highlight_info`

---

### 6.12 剧情故事线分析 `analyze-video-storyline`

**作用：** 只分析、不出片——把单个或多个长视频（如电影、电视剧，累计可达 3.5 小时）智能解析为可检索、可运营的结构化剧情故事线 JSON：按时间顺序的剧情片段（标题/摘要/对白/高光打分/可选关键帧快照）以及归纳出的高光故事线主题。适合媒资检索、智能预告片生成、播放器章节导航、基于剧情内容的高级搜索、「只看 TA」/按角色主题连播等场景。

- **端点：** `POST /v1/mediakit/analyze-video-storyline`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_urls` | array[string] | 是 | 待分析的视频地址列表，单次任务最多 30 个文件。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议 |
| `enable_snapshot` | boolean | 否 | 是否为每个剧情片段生成关键帧快照。设为 `true` 时，`result.storyline_clips[].clip_snapshot_url` 返回快照图片地址 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/analyze-video-storyline' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_urls": [
      "https://example.com/movie_part1.mp4",
      "https://example.com/movie_part2.mp4"
    ],
    "enable_snapshot": true
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-analyze-video-storyline-47695264770",
  "status": "completed",
  "result": {
    "duration": 342.36,
    "source_video_info": [
      {
        "source_video_index": 0,
        "source_video_url": "https://...",
        "source_video_title": "少年识破荐股骗局，支开舅舅操作炒股放大收益",
        "source_video_summary": "男子接到荐股老师电话，被怂恿投二百万买垃圾股，对话被身旁少年全程听到...",
        "source_video_tag": ["荐股骗局", "股票投资", "机智应对", "亲情守护", "金融理财"]
      }
    ],
    "storyline_clips": [
      {
        "clip_index": 0,
        "source_video_index": 0,
        "clip_start_time": 9.12,
        "clip_end_time": 43.28,
        "clip_title": "男子咨询购股建议，被怂恿投入二百万购买八三零六股票",
        "clip_summary": "故事发生在居家室内场景，男子坐在书桌前打电话向吴老师确认是否购入八三零六股票...",
        "clip_dialogue": "吴老师。  我按您所说的，那个，买八三零六这支股票，对吧？  你听我的，把所有的存款全投进去...",
        "clip_score": 3.5,
        "clip_snapshot_url": "https://..."
      }
    ],
    "storyline_highlights": [
      {
        "highlight_index": 0,
        "highlight_clips_index": [0, 1, 2],
        "highlight_title": "男子轻信荐股要投全部积蓄，少年得知隐患打算帮忙",
        "highlight_summary": "男子接到荐股老师电话，对方怂恿他把二百万存款全买股票8306..."
      }
    ]
  },
  "created_at": 1778834568,
  "finished_at": 1778835452,
  "expires_at": 1778921851
}
```

**产物字段：** 无文件产物，剧情故事线数据见响应示例的 `result.storyline_clips`（剧情片段）、`result.storyline_highlights`（高光故事线）与 `result.source_video_info`（源视频信息）

---

### 6.13 高光智剪-短剧 `generate-highlights-microdrama`

**作用：** 输入多集真人短剧视频，让模型读懂角色关系与故事脉络后，自动挑出冲突/反转/情绪爆发等高光片段并重新编排，产出一条或多条带剧名、角标、提示语等视觉包装的营销混剪（高光混剪、单集预告等）。适合短剧投流买量素材的批量生产、剧集推广短视频的自动化剪辑。

- **端点：** `POST /v1/mediakit/generate-highlights-microdrama`
- **计费：** 按时长（元/分钟，输入视频总时长与输出视频总时长混合累计）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_urls` | array[string] | 是 | 输入短剧视频地址列表。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/wmv/mkv 等主流格式。单次最多 30 个文件，累计总时长不超过 45 分钟，输入分辨率最高 1080p |
| `mode` | string | 否 | 剪辑模式，示例均为 `StorylineCuts`（推断） |
| `edit_param` | object | 否 | 成片视觉包装配置（推断） |
| `edit_param.mode` | string | 否 | 包装模式，示例为 `TemplateEdit`（按模板包装）（推断） |
| `edit_param.template_edit.template` | string | 否 | 三要素视觉模板名，可选 `热门短剧1`～`热门短剧5`（推断） |
| `edit_param.template_edit.title` | string | 否 | 成片中展示的剧名（推断） |
| `edit_param.template_edit.hint` | string | 否 | 画面左右两侧的提示语/免责声明（推断） |
| `highlight_cuts_param` | object | 否 | 高光片段选取与剪辑控制（推断） |
| `highlight_cuts_param.cut_mode` | string | 否 | 剪辑模式：`Mixed`（混剪，默认，按精彩程度重排）、`Sequential`（顺剪，保持原片时间顺序，适合分集回顾/剧情概述） |
| `highlight_cuts_param.min_duration` | number | 否 | 生成的高光集锦总时长下限，单位秒。示例配置：1-3 分钟素材用 60，3-5 分钟用 180，5-7 分钟用 300（推断属于该对象） |
| `highlight_cuts_param.max_duration` | number | 否 | 生成的高光集锦总时长上限，单位秒。与 `min_duration` 配套：60/180、180/300、300/420（推断属于该对象） |
| `highlight_cuts_param.highlight_segment_prompt` | string | 否 | 自然语言指令，定义全部高光片段的整体筛选标准（建议包含「优先选择」「内容要求」「节奏要求」等关键词）。仅混剪模式生效 |
| `highlight_cuts_param.highlight_start_prompt` | string | 否 | 自然语言指令，定义混剪开场（Hook）片段应具备的特征。仅混剪模式生效 |
| `highlight_cuts_param.highlight_ending_prompt` | string | 否 | 自然语言指令，定义混剪结尾片段应具备的特征。仅混剪模式生效 |
| `highlight_cuts_param.user_preferred_segments` | array[object] | 否 | 指定优先/强制选用的片段，可多个 |
| `highlight_cuts_param.user_preferred_segments[].episode` | int | 否 | 集数索引，从 0 开始（示例中第 4 集传 3，第 2 集传 1） |
| `highlight_cuts_param.user_preferred_segments[].start_time` | number | 否 | 该集内起始时间，单位秒。不传表示整集优先（推断） |
| `highlight_cuts_param.user_preferred_segments[].end_time` | number | 否 | 该集内结束时间，单位秒。不传表示整集优先（推断） |
| `highlight_cuts_param.enable_storyboard` | bool | 否 | 设为 `true` 时，响应结果的 `result.storyboard_info` 会返回详细分镜列表（来源视频索引、原片起止时间、高光分数、OCR 字幕文本等）。默认不返回 |
| `opening_hook_param` | object | 否 | 智能精彩前置（开场钩子）配置，该功能默认开启（推断） |
| `opening_hook_param.enable_opening_hook` | bool | 否 | 是否启用智能精彩前置，默认 `true`；设为 `false` 则直接从正片高光片段开始 |
| `opening_hook_param.min_duration` | number | 否 | 开场钩子片段的总时长下限，单位秒，默认 5 |
| `opening_hook_param.max_duration` | number | 否 | 开场钩子片段的总时长上限，单位秒，默认 15 |
| `opening_hook_param.min_clip_duration` | number | 否 | 构成钩子的单个高光片段的最小持续时长，单位秒，默认 5 |
| `opening_hook_param.min_score` | number | 否 | 构成钩子的单个高光片段所需达到的最低高光分，范围 [1, 5]，默认 3 |
| `opening_hook_param.opening_hook_prompt` | string | 否 | 自然语言描述偏好的钩子筛选标准 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/generate-highlights-microdrama' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_urls": [
      "https://example.com/drama_1.mp4",
      "https://example.com/drama_2.mp4",
      "https://example.com/drama_3.mp4"
    ],
    "mode": "StorylineCuts",
    "edit_param": {
      "mode": "TemplateEdit",
      "template_edit": {
        "template": "热门短剧1",
        "title": "《我的霸总男友》",
        "hint": "点击下方看完整版"
      }
    },
    "highlight_cuts_param": {
      "cut_mode": "Mixed",
      "min_duration": 60,
      "max_duration": 180,
      "highlight_segment_prompt": "- 优先选择：剧情关键节点与情绪爆发。 - 内容要求：能独立承载叙事。 - 规避：无关支线和低信息量闲聊。 - 合规：不要选择包含“未完待续”提示的片段。"
    },
    "opening_hook_param": {
      "enable_opening_hook": true,
      "max_duration": 10
    }
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-generate-highlights-microdrama-123456789",
  "request_id": "20240521150000ABCDEFG12345",
  "status": "completed",
  "result": {
    "duration": 2450.5,
    "video_urls": [
      "https://example.volcvideo.com/highlight_output_1.mp4?auth_key=..."
    ],
    "mixvideo_info": [
      {
        "mixvideo_index": 0,
        "video_url": "https://example.volcvideo.com/highlight_output_1.mp4?auth_key=...",
        "clips": [
          {
            "clip_type": "OpeningHook",
            "score": 4.8,
            "source_video_index": 1,
            "source_start_time": 305.2,
            "source_end_time": 310.7,
            "cut_start_time": 0.0,
            "cut_end_time": 5.5
          },
          {
            "clip_type": "HighlightClip",
            "score": 4.5,
            "source_video_index": 0,
            "source_start_time": 120.1,
            "source_end_time": 128.3,
            "cut_start_time": 5.5,
            "cut_end_time": 13.7
          }
        ]
      }
    ]
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** `result.video_urls[<n>]` — 取数组下标第 n 条，一次任务可能剪出多条成品（经注册表实测确认）

---

### 6.14 高光智剪-小游戏 `generate-highlights-minigame`

**作用：** 输入一段游戏录屏，自动识别消除、闯关、竞技等玩法中的「全屏连击、BOSS 通关、极限操作」等高光瞬间，剪成一条紧凑的高光集锦。适合买量推广素材、社区/社群分享视频的快速产出；可通过补充游戏名称、玩法与高光定义提升识别精度。

- **端点：** `POST /v1/mediakit/generate-highlights-minigame`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_urls` | array[string] | 是 | 游戏录屏视频地址列表。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/wmv/mkv 等主流格式。单次任务仅支持 1 个文件，时长不超过 10 分钟，分辨率最高 1080p |
| `mode` | string | 否 | 剪辑模式，示例为 `HighlightExtract`（高光提取）（推断） |
| `minigame_info` | object | 否 | 游戏描述信息，用于提升高光识别精度（推断） |
| `minigame_info.name` | string | 否 | 游戏名称 |
| `minigame_info.play_definition` | string | 否 | 游戏玩法、规则或核心特点描述 |
| `minigame_info.highlight_definition` | string | 否 | 高光时刻的定义描述，例如「一次消除多个方块」「躲避所有障碍物通关」「触发隐藏彩蛋」 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/generate-highlights-minigame' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_urls": [
      "https://example.com/parkour_gameplay.mp4"
    ],
    "mode": "HighlightExtract",
    "minigame_info": {
      "name": "极限跑酷达人",
      "play_definition": "玩家通过左右滑动控制角色，躲避障碍物并收集金币，最终到达终点。",
      "highlight_definition": "精彩瞬间包括：连续躲避多个快速移动的障碍物、吃到特殊加速道具、以第一名冲过终点线。"
    }
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-generate-highlights-minigame-123456789",
  "request_id": "20240521160000ABCDEFG12345",
  "status": "completed",
  "result": {
    "duration": 350.8,
    "video_urls": [
      "https://example.volcvideo.com/game_highlight_1.mp4?auth_key=..."
    ],
    "mixvideo_info": [
      {
        "mixvideo_index": 0,
        "video_url": "https://example.volcvideo.com/game_highlight_1.mp4?auth_key=...",
        "clips": [
          {
            "clip_type": "HighlightClip",
            "score": 4.9,
            "source_video_index": 0,
            "source_start_time": 45.5,
            "source_end_time": 52.1,
            "cut_start_time": 0.0,
            "cut_end_time": 6.6
          }
        ]
      }
    ]
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** `result.video_urls[<n>]` — 取数组下标第 n 条，一次任务可能剪出多条成品（经注册表实测确认）

---

### 6.15 高光智剪-影视拆条 `generate-highlights-movie`

**作用：** 输入一部电影、电视剧或纪录片等长视频，自动识别剧情转折点与故事线索，把整片拆成多条各自包含完整故事弧线的 2-3 分钟独立短片，并给每条自动加上高光前置开场与结尾悬念，把长视频「短剧化」。适合长视频二次分发到短视频平台、批量生产可投流的拆条素材。

- **端点：** `POST /v1/mediakit/generate-highlights-movie`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待拆条的单个长视频地址（注意是单个字符串，非数组）。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/wmv/mkv 等主流格式。单文件时长不超过 180 分钟，分辨率最高 1080p，必须同时含视频流与音频流 |
| `highlight_cuts_param` | object | 否 | 拆条片段控制（推断） |
| `highlight_cuts_param.min_duration` | number | 否 | 单个拆条片段的最短时长，单位秒，默认 90 |
| `highlight_cuts_param.max_duration` | number | 否 | 单个拆条片段的最大时长，单位秒，默认 180 |
| `highlight_cuts_param.enable_detailed_info` | bool | 否 | 设为 `true` 时，响应结果的 `result_cuts_info` 数组会增加 `clips` 字段，列出构成该拆条的每个分镜片段的类型（钩子/主体）、评分、在原片与拆条视频中的起止时间 |
| `opening_hook_param` | object | 否 | 高光前置开场（钩子）配置，默认开启（推断） |
| `opening_hook_param.is_enabled` | bool | 否 | 设为 `false` 关闭高光前置，所有拆条严格按原始剧情顺序播放 |
| `opening_hook_param.min_duration` | number | 否 | 「钩子」片段本身的时长下限，单位秒（推断） |
| `opening_hook_param.max_duration` | number | 否 | 「钩子」片段本身的时长上限，单位秒（推断） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/generate-highlights-movie' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_movie.mp4",
    "highlight_cuts_param": {
      "min_duration": 120,
      "max_duration": 180
    },
    "opening_hook_param": {
      "is_enabled": true
    }
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-generate-highlights-movie-123456789",
  "request_id": "20240522150000ABCDEFG12345",
  "status": "completed",
  "result": {
    "input_duration": 7250.5,
    "video_urls": [
      "https://example.volcvideo.com/movie_cut_1.mp4?auth_key=...",
      "https://example.volcvideo.com/movie_cut_2.mp4?auth_key=..."
    ],
    "result_cuts_info": [
      {
        "title": "主角智斗反派，夺回重要文件",
        "duration": 175.8,
        "size": 180555123,
        "video_url": "https://example.volcvideo.com/movie_cut_1.mp4?auth_key=..."
      },
      {
        "title": "雨夜追车，上演生死时速",
        "duration": 160.2,
        "size": 170223456,
        "video_url": "https://example.volcvideo.com/movie_cut_2.mp4?auth_key=..."
      }
    ]
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** `result.video_urls[<n>]` — 取数组下标第 n 条，一次任务会拆出多条成品（经注册表实测确认）

---

### 6.16 剧本还原 `drama-script`

**作用：** 把真人实拍短剧/长剧视频解析成结构化剧本文本——逐场景切分并提取场景描述、人物角色（含外观特征）、逐句对白（含时间码、说话人、情绪、音色描述）、运镜与视觉风格参数，输出一份 Gzip 压缩的 JSON 分析包。它是后续 `drama-recap`（解说视频生成）的**前置分析产物**，本身不产出视频，仅做视频到剧本的还原。

- **端点：** `POST /v1/mediakit/drama-script`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_urls` | array[string] | 是 | 待分析的剧集视频地址列表。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/mov/mkv 等主流格式，暂不支持 HLS (M3U8)。单次支持 1-100 个文件，单个文件不超过 120 分钟、累计不超过 300 分钟；多个视频的分辨率（宽高）必须保持一致；视频须包含清晰内嵌硬字幕 |
| `return_pkg` | bool | 否 | 产物打包方式。`false`（默认）：只返回核心剧本数据，下载得到 Gzip 压缩的 `result.json.gz`；`true`：返回包含全部解析资产的 `.tar.gz` 压缩包，解压后含 `result.json`（核心剧本）、`faces/`（按识别到的人物命名文件夹的人物聚类图片）、`scene_frames/`（按场景划分的视频帧截图） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/drama-script' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_urls": [
      "https://example.com/drama_ep01.mp4",
      "https://example.com/drama_ep02.mp4"
    ],
    "return_pkg": false
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-drama-script-123456789",
  "status": "completed",
  "result": {
    "result_url": "https://example.volcvideo.com/result.json.gz?auth_key=...",
    "duration": 245.5,
    "drama_script_task_id": "v03cd6g10067d8aotcchmjljad3qra7g"
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** `result.result_url` — 剧本分析包下载地址（`.json.gz` 或 `.tar.gz`，有效期 24 小时），下载解压后是结构化剧本文本，**不是视频**

查询响应 `result` 中的 `drama_script_task_id` 需保存好，它是下一步调用 `drama-recap` 的必填入参。

---

### 6.17 解说视频生成 `drama-recap`

**作用：** 基于已有的剧本还原结果，把原片高光片段重新剪辑，配上 AI 配音（或屏幕文字）与全新字幕，产出解说类二创视频；解说词可让 AI 自动创作，也可用自己写的文案。适合短剧投流、影视解说号批量出片、原片硬字幕需擦除后重新包装的二次创作场景。

- **端点：** `POST /v1/mediakit/drama-recap`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `drama_script_task_id` | string | 是 | 前置「剧本还原」任务 ID（以 `v` 开头）。**本接口不自带剧本分析环节，必须先跑 `drama-script` 再用其任务 ID 调用** |
| `recap_text` | string | 否 | 自定义解说文案。当 `drama_recap_config.auto_generate_recap` 为 `false` 或不传时使用；长度不超过 5000 字符 |
| `batch_count` | int | 否 | 单任务批量生成多个版本的解说视频，生成 N 个时结果在 `result.video_urls` 中返回 N 条地址。**本网关请传 `1`**：上游会真的产出多条，但网关只按**一次**计费，且异步侧尚未拦截该参数（见 [3.7](#37-多产物参数请一律传-1重要)） |
| `erase_subtitle` | bool | 否 | 设为 `true` 开启原视频硬字幕擦除，避免与新生解说字幕重叠 |
| `erase_mode` | string | 否 | 擦除模式，目前仅支持 `standard`（标准版，效果与效率平衡） |
| `drama_recap_config` | object | 否 | 解说词与配音节奏配置（推断） |
| `drama_recap_config.auto_generate_recap` | bool | 否 | `true`：由 AI 自动创作解说词；`false` 或不传：使用 `recap_text` 的自定义文案。**该参数只决定解说词由谁写，不能替代 `drama-script` 前置步骤** |
| `drama_recap_config.style` | string | 否 | AI 创作解说词的风格要求，如「悬疑风格，层层递进，结尾留下巨大悬念」「风格轻松搞笑，多用网络热梗」 |
| `drama_recap_config.text_length` | int | 否 | AI 生成解说词的期望长度（字符数），不超过 5000 |
| `drama_recap_config.text_speed` | number | 否 | 配音语速，范围 [0.5, 2.0]，越大越快，默认 1.0；短视频快节奏场景推荐 1.1-1.2 |
| `drama_recap_config.pause_time` | int | 否 | 配音句间停顿时长，单位毫秒，默认 120；值越大节奏越舒缓 |
| `speaker_config` | object | 否 | AI 配音配置（推断） |
| `speaker_config.voice_type` | string | 否 | 配音音色代码。男声：`Yunfeng`(云枫，自信激昂)、`Yunjian`(云健，深沉休闲)、`Yunze`(云泽，沉稳正式)、`Yunye`(云野，成熟放松)、`Yunxi`(云希，活泼阳光)、`Yunyi`(云逸，温柔亲切)、`Yunjie`(云杰，自信干练)；女声：`Xiaoxiao`(晓晓，活泼温暖)、`Xiaochen`(晓辰，自然真实)、`Xiaohan`(晓涵，温暖甜美)、`Xiaomo`(晓墨，清晰放松) |
| `subtitle_config` | object | 否 | 新生解说字幕样式配置（推断） |
| `subtitle_config.disable_subtitle` | bool | 否 | 是否关闭字幕（推断） |
| `subtitle_config.font_size` | int | 否 | 字号，如 40（推断） |
| `subtitle_config.font_color` | string | 否 | 字体颜色，格式 `#RRGGBBAA`，如 `#FFFF00FF`（推断） |
| `subtitle_config.border_color` | string | 否 | 描边颜色，格式 `#RRGGBBAA`，如 `#000000FF`（推断） |
| `subtitle_config.border_width` | int | 否 | 描边宽度（推断） |
| `subtitle_config.align_type` | int | 否 | 字幕对齐方式，示例值 1（推断） |
| `miniseries_edit` | object | 否 | 竖屏视频的「短剧三要素」视觉包装（推断） |
| `miniseries_edit.template` | string | 否 | 视觉模板名，可选 `热门短剧1`～`热门短剧5`（推断） |
| `miniseries_edit.title` | string | 否 | 剧名（推断） |
| `miniseries_edit.hint` | string | 否 | 提示语（推断） |

> **调用顺序硬依赖（务必先读）：** 本接口**不能单独调用**。必须先调用 `POST /v1/mediakit/drama-script` 完成剧本还原，轮询其任务至 `completed` 后，从查询响应 `result.drama_script_task_id` 取出剧本还原任务 ID，再把它作为本接口的必填参数 `drama_script_task_id` 传入。`drama_recap_config.auto_generate_recap` 只控制解说词由 AI 自动生成还是用 `recap_text` 自定义文案，**它不能跳过剧本还原这一步**。若希望「一次调用」直接出解说视频，请改用 2.18 `drama-recap-vertical`（短剧行业模型，已把剧本分析与视频生成合并为一个任务）。

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/drama-recap' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "drama_script_task_id": "v03cd6g10067d8aotcchmjljad3qra7g",
    "erase_subtitle": true,
    "erase_mode": "standard",
    "drama_recap_config": {
      "auto_generate_recap": true,
      "style": "悬疑风格，层层递进，结尾留下巨大悬念",
      "text_speed": 1.2
    },
    "speaker_config": {
      "voice_type": "Yunfeng"
    }
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "request_id": "2026052711093289E685E06E529500826C",
  "task_id": "amk-tool-drama-recap-121654832642",
  "task_type": "drama-recap",
  "status": "completed",
  "result": {
    "video_url": "https://example.volcvideo.com/output.mp4?auth_key=...",
    "video_urls": [
      "https://example.volcvideo.com/output.mp4?auth_key=..."
    ],
    "duration": 29.314,
    "total_count": 1,
    "success_count": 1,
    "failed_count": 0,
    "error": {}
  },
  "created_at": 1779851373,
  "finished_at": 1779852072,
  "expires_at": 1779938471
}
```

**产物字段：** `result.video_url` — 单产物（设置了 `batch_count` 批量生成多个版本时，用 `result.video_urls[<n>]` 按数组下标取第 n 条）

---

### 6.18 解说视频生成（短剧行业模型） `drama-recap-vertical`

**作用：** 一次调用即完成「剧本分析 + 高光混剪 + 解说合成」，直接输**竖版**解说视频：支持 `text`（原片高光混剪 + 屏幕文字解说）与 `narrate`（原片高光混剪 + AI 语音解说 + 自定义 BGM）两种模式，可控制产出条数与单条时长、添加「短剧三要素」包装。适合短剧投流、宣发种草、二创剪辑等竖屏分发场景——相比 2.17 免去了先跑剧本还原的两步流程。

- **端点：** `POST /v1/mediakit/drama-recap-vertical`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_urls` | array[string] | 是 | 输入短剧视频地址列表。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/wmv/mkv 等主流格式。单次支持 1-30 个文件，累计时长不超过 120 分钟，仅支持 1080p 且各文件分辨率须一致；视频须同时含视频流与音频流，音频轨须有清晰可识别的中文对白 |
| `mode` | string | 是 | 解说模式：`text`（文字解说，AI 文案以文字形式呈现在画面上）、`narrate`（旁白解说，AI 语音解说） |
| `max_count` | int | 否 | 期望生成的视频数量上限，范围 [1, 100]，默认 3。**本网关请显式传 `1`**：默认值 3 意味着不传也会产出多条，而网关只按**一次**计费，且异步侧尚未拦截该参数（见 [3.7](#37-多产物参数请一律传-1重要)） |
| `min_duration` | int | 否 | 单个视频的最小时长，单位秒，默认 30 |
| `max_duration` | int | 否 | 单个视频的最大时长，单位秒，默认 180（输出时长整体限制在 1-7200 秒） |
| `opening_hook` | string | 否 | 精彩片段前置：`auto`（默认，智能判断是否开启）、`force`（强制开启）、`disable`（关闭） |
| `narrate_options` | object | 否 | 旁白解说配置，`mode` 为 `narrate` 时生效（推断） |
| `narrate_options.narrate_ratio` | number | 否 | 解说浓度，即旁白时长占生成视频时长的比例，范围 [0, 1]，越大解说越密集，建议 0.3-0.5 |
| `narrate_options.enable_narrate_bgm` | bool | 否 | 是否添加背景音乐，默认 `true` |
| `narrate_bgm_url` | string | 否 | 自定义背景音乐地址，当 `narrate_options.enable_narrate_bgm` 为 `true` 时生效 |
| `text_options` | object | 否 | 文字解说样式配置，`mode` 为 `text` 时生效（推断） |
| `text_options.font_type` | string | 否 | 字体，如 `SY_Bold`（思源粗体） |
| `text_options.font_size` | int | 否 | 字号，如 45 |
| `text_options.font_color` | string | 否 | 字体颜色，格式 `#RRGGBBAA`，如 `#FFFF00FF` |
| `text_options.border_width` | int | 否 | 描边宽度 |
| `text_options.border_color` | string | 否 | 描边颜色，格式 `#RRGGBBAA`，如 `#000000FF` |
| `text_options.shadow_color` | string | 否 | 阴影颜色 |
| `text_options.is_bold` | bool | 否 | 是否加粗 |
| `text_options.is_italic` | bool | 否 | 是否斜体 |
| `edit_param` | object | 否 | 「短剧三要素」视觉包装配置（推断） |
| `edit_param.mode` | string | 否 | 包装模式，示例为 `TemplateEdit`（按模板包装）（推断） |
| `edit_param.template_edit.template` | string | 否 | 视觉模板名，可选 `热门短剧1`～`热门短剧5`（推断） |
| `edit_param.template_edit.title` | string | 否 | 剧名（推断） |
| `edit_param.template_edit.hint` | string | 否 | 提示语（推断） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/drama-recap-vertical' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_urls": [
      "https://example.com/drama_1.mp4",
      "https://example.com/drama_2.mp4",
      "https://example.com/drama_3.mp4"
    ],
    "mode": "text",
    "max_count": 1,
    "max_duration": 60,
    "opening_hook": "force",
    "text_options": {
      "font_type": "SY_Bold",
      "font_size": 45,
      "font_color": "#FFFF00FF",
      "border_width": 2,
      "border_color": "#000000FF",
      "is_italic": false
    }
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-drama-recap-vertical-12345",
  "request_id": "20240522180000ABCDEFG12345",
  "status": "completed",
  "result": {
    "input_duration": 245.5,
    "output_duration": 58.2,
    "mode": "text",
    "video_urls": [
      "https://example.volcvideo.com/narration_output_1.mp4?auth_key=..."
    ],
    "video_infos": [
      {
        "duration": 58.2,
        "size": 48123456,
        "poster_url": "",
        "video_url": "https://example.volcvideo.com/narration_output_1.mp4?auth_key=..."
      }
    ]
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** `result.video_urls[<n>]` — 取数组下标第 n 条，一次任务可生成多条成品（经注册表实测确认）

### 6.19 视频抽帧 `extract-frames`

**作用：** 从一段视频里截取一批静态图片，支持四种策略：按固定时间间隔（`TimeInterval`）、按指定时间点（`SpecifiedTime`）、按指定帧首/尾帧（`SpecifiedFrames`）、按场景切换点自动检测（`SceneChange`），还可把整批截图拼成一张雪碧图。适合生成视频封面/进度条预览图、给下游视频理解任务准备输入帧。

- **端点：** `POST /v1/mediakit/extract-frames`
- **计费：** 按次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待抽帧视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4, mov, mkv, flv, ts, avi, wmv 等主流格式 |
| `snapshot_type` | string | 是 | 抽帧策略：`TimeInterval`（按时间间隔）、`SpecifiedTime`（按指定时间点）、`SpecifiedFrames`（按指定帧）、`SceneChange`（按场景变化）。不同策略需要搭配下面对应的参数 |
| `time_interval` | number | 否 | 抽帧间隔，单位秒。`snapshot_type: TimeInterval` 时使用（示例：每 10 秒一帧填 `10`）。其余策略不生效 |
| `specified_time` | array[number] | 否 | 指定的时间点列表，单位秒（示例：`[5, 10, 15]` 表示在 5s/10s/15s 各截一帧）。`snapshot_type: SpecifiedTime` 时使用，最多支持 1000 个时间点 |
| `specified_frames` | array[int] | 否 | 指定的帧序号列表，`0` 表示第一帧、`-1` 表示最后一帧（示例：`[0, -1]` 同时取首尾帧）。`snapshot_type: SpecifiedFrames` 时使用，常用于生成封面 |
| `scene_change_threshold` | number | 否 | 场景切换检测敏感度，仅 `snapshot_type: SceneChange` 生效，默认 `0.1`。值越小检测越敏感、截出的图越多 |
| `snapshot_limit` | int | 否 | 限制 `TimeInterval` 与 `SceneChange` 策略生成截图的最大张数，上限 1000（推断其类型为整型） |
| `enable_sprite` | bool | 否 | 是否把本批截图拼成一张雪碧图大图，默认 `false`。开启后 `result.snapshots` 里返回的是拼接后的大图地址而不是单帧小图（推断其默认值为 `false`） |
| `sprite_rows` | int | 否 | 雪碧图行数，`enable_sprite: true` 时用于排列小图（推断其类型为整型） |
| `sprite_cols` | int | 否 | 雪碧图列数，与 `sprite_rows` 搭配（推断其类型为整型） |
| `scale_long` | int | 否 | 控制单张截图的长边像素，用于限制雪碧图最终尺寸（推断其类型为整型） |
| `scale_short` | int | 否 | 控制单张截图的短边像素，用于限制雪碧图最终尺寸（推断其类型为整型） |

> **计费口径说明：** 官方对视频抽帧实际是**按截图总张数**计费（费用 = 截图总张数 / 1000 × 单价），不是按次。本网关把该工具登记为**按次固定价**。请管理员定价时留意这个口径差：长视频、或 `time_interval` 设得很小、或 `SceneChange` 策略遇上场景密集的片子，一次任务可能产出上万张截图，官方侧的实际成本会远高于按次固定价。

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/extract-frames' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/source.mp4",
    "snapshot_type": "TimeInterval",
    "time_interval": 10
  }'
```

**响应**

提交后拿到 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询到 `status` 为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-extract-frames-123456789",
  "request_id": "20240523100000ABCDEFG12345",
  "status": "completed",
  "result": {
    "snapshots": [
      {
        "image_url": "https://example.com/snapshot/frame_0s.jpg?sign=..."
      },
      {
        "image_url": "https://example.com/snapshot/frame_10s.jpg?sign=..."
      }
    ],
    "snapshot_count": 2
  },
  "expires_at": 1777531298,
  "created_at": 1777358413,
  "finished_at": 1777359999
}
```

**产物字段：** 每个截图一个 URL，取 `result.snapshots[<n>].image_url`，可能有很多张（另有 `result.snapshot_count` 为总张数）；默认输出 JPG，URL 有效期 24 小时

---

### 6.20 视频转码 `transcode-video`

**作用：** 把一路视频码流转成另一路码流——改变分辨率与画幅比例、换编码参数、换封装格式，以适配不同的播放终端和网络环境。典型用法是把竖屏视频转成 1:1 方形，并通过 `scale_mode` 自动补黑边，在不拉伸、不裁剪画面的前提下套进新分辨率。

- **端点：** `POST /v1/mediakit/transcode-video`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待转码视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议 |
| `video.scale_type` | int | 否 | 缩放方式，示例值 `2` 表示按下面给出的目标宽高进行缩放（其余取值官方文档未展开，推断） |
| `video.scale_width` | int | 否 | 目标宽度（px），示例 `720` |
| `video.scale_height` | int | 否 | 目标高度（px），示例 `720` |
| `video.scale_mode` | int | 否 | 缩放模式，示例值 `2` 表示自动补黑边：保持画面比例，不拉伸、不裁剪地适配目标分辨率 |
| `container_format` | string | 否 | 输出封装格式（如 `MP4`、`FLV`）。官方文档正文提到本工具支持封装格式转换，但未给出该字段的取值表（推断：与同族视频处理工具共用的输出封装格式字段） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/transcode-video' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/source_vertical.mp4",
    "video": {
      "scale_type": 2,
      "scale_width": 720,
      "scale_height": 720,
      "scale_mode": 2
    }
  }'
```

**响应**

提交后拿到 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询到 `status` 为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-transcode-video-112738623234",
  "task_type": "transcode-video",
  "status": "completed",
  "result": {
    "duration": 15.07,
    "resolution": "720p",
    "video_codec": "h264",
    "video_url": "https://output.volcvideo.com/transcoded_video.mp4?auth_key=..."
  },
  "expires_at": 1780472196,
  "created_at": 1780385775,
  "finished_at": 1780385797,
  "request_id": "20260601153614EA4FAB54AB56A39BC53E",
  "queue_id": "default"
}
```

**产物字段：** `result.video_url`（另有 `result.duration` / `result.resolution` / `result.video_codec` 描述产物），URL 有效期 24 小时

---

### 6.21 极智超清 `martencode-video`

**作用：** 在重编码过程中对画面里的场景、动作、内容、纹理做智能分析，选出最优编码参数，用更低的码率输出主观画质不下降的视频，从而降低存储与带宽成本。核心是**编码效率**而不是分辨率：输出分辨率可以与输入保持一致，文件体积和平均码率明显变小。与 2.1–2.3 的「画质增强 `enhance-video`」不同——画质增强做的是超分/插帧/降噪，把观感（乃至分辨率）做上去；极智超清不做 AI 画质修复，只把「同样的观感」压成更小的文件，适合已经满意画面、只想省流省存储的场景。

- **端点：** `POST /v1/mediakit/martencode-video`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待处理视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议 |
| `container_format` | string | 否 | 输出封装格式，示例 `MP4`（推断取值与视频转码一致：`MP4` / `FLV` / `MPEGTS`） |
| `video.codec` | string | 否 | 输出编码格式，示例 `h265`（H.265 压缩效率更高，用于降低文件体积） |
| `video.scale_type` | int | 否 | 缩放方式，`0` 表示不进行任何缩放、保持原始分辨率 |
| `video.bitrate_mode` | string | 否 | 码率控制模式，示例 `crf`（智能省流） |
| `video.bitrate_crf` | int | 否 | CRF 值，示例 `25`。与 `bitrate_mode: crf` 搭配，决定日常场景下的省流力度（推断其类型为整型） |
| `video.bitrate_kbps` | int | 否 | 码率上限（kbps），示例 `2000`。给复杂、高运动场景设一层码率天花板，防止瞬时码率飙升导致播放卡顿 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/martencode-video' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/source_480p.mp4",
    "container_format": "MP4",
    "video": {
      "codec": "h265",
      "scale_type": 0,
      "bitrate_mode": "crf",
      "bitrate_crf": 25,
      "bitrate_kbps": 2000
    }
  }'
```

**响应**

提交后拿到 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询到 `status` 为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-martencode-video-123456789",
  "task_type": "martencode-video",
  "status": "completed",
  "result": {
    "duration": 15.07,
    "resolution": "480p",
    "video_codec": "h265",
    "video_url": "https://output.volcvideo.com/martencode_video.mp4?auth_key=..."
  },
  "expires_at": 1780472196,
  "created_at": 1780385775,
  "finished_at": 1780385797,
  "request_id": "20260601153614EA4FAB54AB56A39BC53E",
  "queue_id": "default"
}
```

**产物字段：** `result.video_url`（另有 `result.duration` / `result.resolution` / `result.video_codec` 描述产物），URL 有效期 24 小时

---

### 6.22 视频转封装 `remux-video`

**作用：** 在不重新编码音视频码流的前提下，只把文件的容器格式换掉（MP4 / FLV / MPEG-TS 之间互转）。因为不做编解码计算，所以处理速度极快、画质与音质零损失、成本极低。适合点播分发格式适配、流媒体切片打包、多端播放兼容等场景。

- **端点：** `POST /v1/mediakit/remux-video`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待转换视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议 |
| `container_format` | string | 否 | 目标封装格式，支持 `MP4`（默认）、`FLV`、`MPEGTS` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/remux-video' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/source_480p.mp4",
    "container_format": "FLV"
  }'
```

**响应**

提交后拿到 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询到 `status` 为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-remux-video-123456789",
  "request_id": "20240522190000ABCDEFG67890",
  "status": "completed",
  "result": {
    "video_url": "https://example.volcvideo.com/remux_output.flv?auth_key=...",
    "duration": 180.5
  },
  "expires_at": 1777468800,
  "created_at": 1777292400,
  "finished_at": 1777292402
}
```

**产物字段：** `result.video_url`（另有 `result.duration`），URL 有效期 24 小时

---

### 6.23 视频暗水印-添加 `add-video-invisible-watermark`

**作用：** 把一串数字信息（用户 ID、内容 ID 等）以肉眼不可见的方式嵌入视频像素中，生成一个看不出差异、但已带隐藏信息的新视频。用途是视频版权保护、内容泄露溯源与文件真实性校验：即使视频经过压缩、转码、裁剪或录屏，水印仍有很大概率能被 2.24 的提取接口读回来。若要抗更强的二次处理，可调高 `watermark_level`，或对同一视频叠加两层不同强度的水印（提取接口能一次性读出全部水印）。

- **端点：** `POST /v1/mediakit/add-video-invisible-watermark`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待嵌入水印的视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4, mov, mkv, flv, ts, avi, wmv 等主流格式，最高支持 4K 分辨率 |
| `watermark_content` | string | 是 | 要嵌入的水印内容。必须是纯数字字符串（如 `"12345678901234567"`），对应数值须在 1 ~ 9223372036854775807（64 位正整数）之间，不允许前导零或任何非数字字符 |
| `watermark_level` | string | 否 | 暗水印强度，决定抗攻击性与画面影响程度的平衡：`normal`（默认，标准强度，画质与抗攻击性较均衡）、`high`（高强度，抗压缩/多次录屏能力最强，对画面的潜在修改也相对更大） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/add-video-invisible-watermark' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/drama_ep01.mp4",
    "watermark_content": "12345678901234567",
    "watermark_level": "normal"
  }'
```

**响应**

提交后拿到 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询到 `status` 为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-add-video-invisible-watermark-1703200",
  "status": "completed",
  "result": {
    "video_url": "https://example.volcvideo.com/watermarked_output.mp4?auth_key=...",
    "duration": 120.5,
    "resolution": "1080p"
  }
}
```

**产物字段：** `result.video_url`（已嵌入暗水印的视频，另有 `result.duration` / `result.resolution`）；把这个 URL 交给 2.24 `extract-video-invisible-watermark` 即可验证水印

---

### 6.24 视频暗水印-提取 `extract-video-invisible-watermark`

**作用：** 从一段视频里把 2.23 嵌入的暗水印数字串读出来，用于校验视频来源。典型场景：拿到一段疑似泄露/盗播的视频，提取水印内容后与当初嵌入的 ID 比对，一致即证明该视频可追溯到某个用户或某批素材；若视频被叠加过两层水印，本接口会一次性返回全部水印。注意只能提取由 AI MediaKit 暗水印添加接口嵌入的水印，无法提取第三方工具添加的暗水印。

- **端点：** `POST /v1/mediakit/extract-video-invisible-watermark`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待提取水印的视频地址，通常填 2.23 添加接口产物中的 `result.video_url`。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；最高支持 4K 分辨率 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/extract-video-invisible-watermark' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.volcvideo.com/watermarked_output.mp4?auth_key=..."
  }'
```

**响应**

提交后拿到 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询到 `status` 为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-extract-video-invisible-watermark-1703201",
  "status": "completed",
  "result": {
    "watermark_contents": [
      {
        "watermark_content": "12345678901234567"
      }
    ],
    "duration": 120.5
  }
}
```

**产物字段：** 无 URL 产物——提取结果在 `result.watermark_contents[<n>].watermark_content`（每个元素是一条提取到的水印数字串，多层水印会有多条），另有 `result.duration`

---

### 6.25 视频画质检测 VQScore `assess-video-quality`

**作用：** 无需原始参考视频，直接对输入视频做无参考质量评价（VQScore），输出 0–100 的客观画质分，模拟人眼主观感受。用于给画质「打分」而不是「修画质」：适合入库/上线前的批量片源筛查、AIGC 生成视频的质量验收、分发链路画质巡检与监控。它不会输出任何文件，若检测出低分需要修复，请另接画质增强类工具。

- **端点：** `POST /v1/mediakit/assess-video-quality`
- **计费：** 按时长

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待检测的视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种输入协议 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/assess-video-quality' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_video.mp4"
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-assess-video-quality-12345",
  "request_id": "20240523110000ABCDEFG98765",
  "status": "completed",
  "result": {
    "vq_score": 88.5,
    "duration": 91.2
  },
  "expires_at": 1777534898,
  "created_at": 1777362013,
  "finished_at": 1777362599
}
```

**产物字段：** 无文件产物，评分见上方响应示例的 `result.vq_score`（0–100，越高越好）

---

### 6.26 视频人脸打码 `face-blur-video`

**作用：** 自动识别视频每一帧中的人脸区域，并对检测到的所有人脸统一打上马赛克或高斯模糊，输出人脸被遮盖的成片。适合短剧/影视内容脱敏、未成年人出镜保护、新闻与纪实素材处理、生成式视频参考素材预处理等需要隐藏人物五官隐私的场景。打码方式、强度、外扩范围与检测灵敏度均可调。

- **端点：** `POST /v1/mediakit/face-blur-video`
- **计费：** 按时长

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待打码的视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种输入协议 |
| `mask_mode` | string | 否 | 打码方式：`mosaic`（马赛克，默认）、`blur`（高斯模糊） |
| `mask_strength` | string | 否 | 打码强度：`low`（低）、`medium`（中，默认）、`high`（高） |
| `face_box_expand` | number | 否 | 在人脸检测框基础上向外扩展的比例，用于更彻底地覆盖人脸轮廓。例如 `0.2` 表示检测框每条边向外扩展 20%（推断） |
| `face_confidence` | number | 否 | 人脸检测置信度阈值，取值 [0.1, 1.0]，默认 0.35。值越高检测越保守（可能漏掉模糊或非正面的脸），值越低越容易把非人脸物体误判为人脸 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/face-blur-video' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_video.mp4",
    "mask_mode": "blur",
    "mask_strength": "high",
    "face_box_expand": 0.2,
    "face_confidence": 0.5
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-face-blur-video-123456789",
  "request_id": "20260415160000ABCDEFG12345",
  "status": "completed",
  "result": {
    "video_url": "https://.../blurred_video.mp4?auth_key=...",
    "duration": 180.5
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** `result.video_url`

---

### 6.27 视频人脸融合（换脸） `face-swap-video`

**作用：** 把一张目标人脸照片替换到视频中人物的脸上，输出换脸后的成片，同时保留原视频的动作、背景、脸型、表情和口型。适合生成式视频脱敏换脸（用虚拟形象替代真人出镜）、虚拟试穿/数字人形象替换等场景。当前仅支持单人换脸：多人视频中会逐帧替换画面里尺寸最大的人脸，因此建议输入以单人为主的视频。

- **端点：** `POST /v1/mediakit/face-swap-video`
- **计费：** 按时长（按输入视频时长计费）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待换脸的源视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种输入协议；当前仅支持 MP4，分辨率不超过 1080P，时长不超过 10 分钟（600 秒） |
| `face_mappings` | array | 是 | 人脸映射列表，每项是一个对象（见下一行）。当前仅支持单人换脸，传入一项即可 |
| `face_mappings[].target_face_url` | string | 是 | 目标人脸照片地址（JPEG/PNG，建议分辨率 256×256～1080×1080，需为清晰无遮挡的正脸，不支持动漫人脸）。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎对象存储 (`tos://`) 三种输入协议 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/face-swap-video' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_video.mp4",
    "face_mappings": [
      {
        "target_face_url": "https://example.com/target_face.png"
      }
    ]
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-face-swap-video-123456789",
  "request_id": "20260629160000ABCDEFG12345",
  "status": "completed",
  "result": {
    "video_url": "https://.../face_swap_result.mp4?auth_key=...",
    "duration": 180.5
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** `result.video_url`

---

### 6.28 视频插帧 `video-frame-interpolation`

**作用：** 基于深度学习的补帧能力，在相邻视频帧之间智能生成过渡帧，把低帧率视频提升到指定目标帧率。用来消除快速运动、大屏播放场景下的卡顿、跳帧与画面不连贯；也适合 AIGC 视频增强，例如把 Seedance 等模型生成的 24fps 视频补到更高帧率，让人物动作、镜头运动和动态细节衔接更顺滑。注意它只提升帧率，不改变分辨率与码率（均跟随原片），若同时需要提清晰度请配合画质增强工具。

- **端点：** `POST /v1/mediakit/video-frame-interpolation`
- **计费：** 按时长

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待插帧的视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种输入协议 |
| `fps` | int | 是 | 目标输出帧率，取值范围 [15, 120]。建议目标帧率不超过原片帧率的 4 倍，以获得更自然的补帧效果（推断为必填） |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/video-frame-interpolation' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/demo_video_24fps.mp4",
    "fps": 60
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-video-frame-interpolation-123456789",
  "request_id": "20240523100000ABCDEFG12345",
  "status": "completed",
  "result": {
    "video_url": "https://.../interpolated.mp4?sign=...",
    "duration": 16.0,
    "resolution": "1080p",
    "fps": 60
  },
  "expires_at": 1777531298,
  "created_at": 1777358413,
  "finished_at": 1777359999
}
```

**产物字段：** `result.video_url`

---

### 6.29 视频口型对齐 `lip-sync`

**作用：** 输入一段人物出镜视频和一段驱动音频，用音频内容驱动视频中人物的面部表情与口型，输出口型与音频同步的口播视频——即给一段现成画面「换上新台词」。适合把录好的口播音频套到已有视频上、虚拟主播/数字人口播生成、多语种配音换口型、教育培训与营销推广视频的批量再配音等场景。音视频时长不一致时，可通过 `enable_video_loop` 决定是截断还是循环画面以对齐音频。

- **端点：** `POST /v1/mediakit/lip-sync`
- **计费：** 按时长（按输出视频时长计费）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 人物视频地址（视频必须包含视频流，仅支持单人真人出镜，卡通人物需五官与真人比例相近；建议正对镜头，水平转动不超过 45 度、俯仰不超过 15 度，无遮挡且面部光线稳定）。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种输入协议；仅支持 MP4，时长不超过 30 分钟，暂不支持 HDR |
| `audio_url` | string | 是 | 驱动音频地址，用于驱动视频中人物的表情与口型。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种输入协议；支持 mp3、aac、wav、m4a、flac |
| `enable_video_loop` | bool | 否 | 音视频时长不一致时的处理策略。`false`（默认）：以较短者为准截断，输出时长取原视频与驱动音频中较短的一方；`true`：以音频时长为准严格对齐，音频长于视频时按「正向 → 反向 → 正向…」镜像循环画面，音频短于视频时截断画面 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/lip-sync' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_portrait_video.mp4",
    "audio_url": "https://example.com/your_driving_audio.mp3",
    "enable_video_loop": true
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-lip-sync-123456789",
  "status": "completed",
  "result": {
    "video_url": "https://.../lip_sync_output.mp4?auth_key=...",
    "duration": 60.5
  }
}
```

**产物字段：** `result.video_url`

---

### 6.30 视频横转竖 `reframe-video-to-vertical`

**作用：** 把横版视频智能重构图成竖版（默认 9:16）成片，由 AI 逐帧做主体检测、发言主体识别与主体跟踪，自动决定裁剪与多人场景的布局，无需人工剪辑即可产出可直接投放抖音/快手/视频号等竖屏平台的版本。若已有字幕轨，可额外传入 `subtitle_url` 把字幕烧录进竖屏成片。

- **端点：** `POST /v1/mediakit/reframe-video-to-vertical`
- **计费：** 按时长（按输出视频时长计费，0.3 元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待转换的横版视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种输入协议；支持 MP4、MOV、MKV、AVI、FLV，画面比例需为横屏（16:9、4:3、21:9），时长不超过 4 小时，单文件不超过 20 GB |
| `subtitle_url` | string | 否 | 需要烧录进竖屏成片的外部字幕文件地址，支持 SRT、ASS 等格式，最多传入一个。该字幕仅用于成片阶段烧录，不参与主体识别、跟踪、裁剪等内部处理。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种输入协议 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/reframe-video-to-vertical' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_landscape_video.mp4",
    "subtitle_url": "https://example.com/your_subtitle.srt"
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-reframe-video-to-vertical-123456789",
  "request_id": "20260908130000ABCDEFG12345",
  "status": "completed",
  "result": {
    "video_url": "https://.../vertical_output.mp4?auth_key=...",
    "duration": 120.5
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777292051
}
```

**产物字段：** `result.video_url`

### 6.31 字幕擦除（标准版） `erase-video-subtitle`

**作用：** 检测视频画面中内嵌的硬字幕并擦除，再按帧重建被擦区域的纹理，输出无字幕的干净视频。适合短剧出海/翻译前擦掉原字幕、混剪二创去字幕等「整片去硬字幕」的主流用法。

> **与 6.4「字幕擦除（精细化版）」的区别：** 两者是**两条独立的上游工具**，名字只差 `-pro` 后缀，不是同一个接口的版本号。
>
> | | 标准版（本节） | 精细化版（[6.4](#64-字幕擦除精细化版-erase-video-subtitle-pro)） |
> |---|---|---|
> | 入参 | 只有 `video_url` | 另支持 `mode`（字幕/文本）、`model_version`、手动框选 `erase_ratio_location`、时间段过滤、字幕特征阈值 |
> | 擦除范围 | 只擦字幕 | 可擦水印、台标、人名条等任意文本 |
> | 输出分辨率 | 最高 1080P | 最高 1080P |
>
> **需要指定擦除区域、只擦某个时间段、或调整字幕判定阈值时，用精细化版。**

- **端点：** `POST /v1/mediakit/erase-video-subtitle`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待擦除字幕的视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/wmv/mkv 等主流格式；输入分辨率最高 2K，输出分辨率最高 1080P |
| `media_output_destination` | string | 否 | 指定产物存储位置：`vod://<空间名>` 存到视频点播、`tos://<桶名>` 存到对象存储。**设置后结果里的 `video_url` 会返回 `vod://`/`tos://` 地址而不是 24 小时临时下载链接**。首次使用前须先在 MediaKit 控制台完成写入授权 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/erase-video-subtitle' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/source_video.mp4"
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-erase-video-subtitle-123456789",
  "request_id": "202604272009269BA43D771246AFF513F0",
  "status": "completed",
  "result": {
    "video_url": "https://example.volcvideo.com/output.mp4?auth_key=...",
    "duration": 180.5
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** `result.video_url`

---

### 6.32 剧本还原（重绘场景） `drama-script-vertical`

**作用：** 分析多集短剧视频，输出**可复用的角色、场景、预处理资产**与**逐集剧情分镜剧本**，产出一份可索引全部交付物的 manifest 清单。适合短剧重绘、出海重制预处理、剧情解说等二次创作场景。

它面向**多集批量**分析，比 6.16 的 `drama-script` 多了三种用法：

- **首次分析**：传入第一批剧集，完整跑资产解析 + 预处理 + 剧本生成
- **接续分析（增量）**：新剧集只传新增的，用 `previous_manifest_url` 指向前序任务的 manifest，系统复用已识别出的角色/场景，**保证跨批次一致性**
- **分步执行**：先跑 `return_type: ["asset","preprocess"]` 拿资产并人工核对，再传 `current_asset_url` + `return_type: ["script"]` 跳过资产分析直接出剧本——省时省钱

> **产物是 manifest.json 清单，不是可播视频。** 它不生成解说成片；需要成片请用 [6.17 `drama-recap`](#617-解说视频生成-drama-recap) 或 [6.18 `drama-recap-vertical`](#618-解说视频生成短剧行业模型-drama-recap-vertical)。

- **端点：** `POST /v1/mediakit/drama-script-vertical`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `videos` | array[object] | 是 | 待分析的短剧视频列表，**1–100 个**。每项需同时给出视频地址与它在整部短剧中的集序号 |
| `videos[].video_url` | string | 是 | 视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 MP4/FLV/TS/AVI/MOV/WMV/MKV 等格式，**暂不支持 HLS (M3U8)**；单任务累计时长建议 20 分钟以内（实测最高约 60 分钟，但耗时显著拉长）；同一任务内各视频分辨率必须一致，建议不超过 1080p；**视频必须含内嵌硬字幕** |
| `videos[].episode` | int | 是 | 该视频在整部短剧中的集序号（从 0 或 1 开始均可）。跨任务接续分析时必须保持同一剧集序号不变 |
| `drama_type` | string | 否 | 短剧类型：`AI`（默认，AI 短剧，含仿真人、2D/3D 动漫）、`Human`（真人短剧） |
| `return_type` | array[string] | 否 | 要返回的结果类型，可任意组合（至少 1 个），不传默认三项全返回：`asset`（角色/关系/场景等资产分析）、`script`（逐集分镜剧本）、`preprocess`（基础分镜、人物锚点等预处理结果） |
| `previous_manifest_url` | string | 否 | 前序任务的 manifest.json 地址。传入后复用前序任务的有效分析结果，保证角色等资产跨批次一致，并提高效率 |
| `current_asset_url` | string | 否 | 已有资产清单 manifest.json 地址。传入即**跳过资产分析**、直接用它进入后续分析，可显著降低耗时与成本 |
| `script_param` | object | 否 | 剧情分镜时长配置 |
| `script_param.min_duration` | number | 否 | 分镜时长下限（秒），范围 [1, 120]，默认 10 |
| `script_param.max_duration` | number | 否 | 分镜时长上限（秒），范围 [5, 120] 且不得小于 `min_duration`，默认 30。为保证剧情完整，不可再分的镜头可能超出此上限 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/drama-script-vertical' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "videos": [
      { "video_url": "https://example.com/drama_episode_01.mp4", "episode": 1 },
      { "video_url": "https://example.com/drama_episode_02.mp4", "episode": 2 }
    ],
    "drama_type": "Human",
    "return_type": ["asset", "script", "preprocess"],
    "script_param": {
      "min_duration": 10,
      "max_duration": 30
    }
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-drama-script-vertical-123456789",
  "request_id": "20240521150000ABCDEFG12345",
  "status": "completed",
  "result": {
    "result_url": "https://example.volcvideo.com/manifest.json?auth_key=...",
    "input_duration": 245.5,
    "duration": 245.5
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** `result.result_url` — manifest.json 清单地址（24 小时有效），通过它索引本次任务交付的全部产物文件。**它不是视频**，需下载后按清单取件。

---

### 6.33 视频元信息获取 `probe-video-metadata`

**作用：** 探测一个视频 URL 的媒资元信息——容器层信息、视频流信息、音频流信息，**不转码、不产出新文件**。适合入库前校验格式与编码、按分辨率/码率分流处理策略、核对接入素材是否符合处理前置条件（比如某个工具要求「必须同时含视频流与音频流」）等场景。

- **端点：** `POST /v1/mediakit/probe-video-metadata`
- **计费：** 按次，0.0001 元/次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待探测的视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/wmv/mkv 等主流格式 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/probe-video-metadata' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_video.mp4"
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-probe-video-metadata-123456789",
  "request_id": "20240521150000ABCDEFG12345",
  "status": "completed",
  "result": {
    "format_meta": {
      "format": "mp4",
      "duration": 245.5,
      "size": 48123456
    },
    "video_stream_meta": {
      "codec": "h264",
      "width": 1920,
      "height": 1080,
      "frame_rate": 30
    },
    "audio_stream_meta": {
      "codec": "aac",
      "sample_rate": 44100,
      "channels": 2
    }
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** 无文件产物。三段元信息（`format_meta` / `video_stream_meta` / `audio_stream_meta`）就在 `result` 里，网关不返回 `url` 字段。

---

### 6.34 音视频合成 `mux-audio-video`

**作用：** 给一个已有视频配上新的音轨——可以**混音**（保留视频原声，与新音频同时播放）也可以**替换**（丢掉原声，只用新音频）。适合为无声素材或希望保留现场声的视频加 BGM、为成片换配音整轨、以及把画面与一段独立录制的旁白合到一起等场景。与 [7.7 音频混合](#77-音频混合-mix-audio) 的分工是：那个只处理音频、输出音频；这个的输入必带一路视频，输出是视频。

- **端点：** `POST /v1/mediakit/mux-audio-video`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待处理的视频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp4/flv/ts/avi/mov/wmv/mkv 等主流格式，最高 4K |
| `audio_url` | string | 是 | 要合入的音频地址，协议支持同上；支持 mp3/aac/wav/m4a 等主流格式 |
| `is_audio_reserve` | boolean | 否 | 是否保留视频原有音轨。`true`（默认）：原声与新音频**混合**；`false`：丢掉原声，**替换**为新音频 |
| `is_video_audio_sync` | boolean | 否 | 是否启用**时长对齐**，默认 `false`。不启用时输出总时长取两者中较长者：音频更长则视频流结束后**维持末帧画面**到音频结束，视频更长则音频流结束后**保持静音**到视频结束 |
| `sync_mode` | string | 否 | 启用对齐后以谁为基准：`video`（默认）/ `audio` |
| `sync_method` | string | 否 | 启用对齐后的处理方式：`trim`（默认，裁剪较长者）/ `speed`（变速使两者等长） |

> 官方**建议优先用 `trim`**：它不改变播放速率与音调，只舍掉多余部分，听感观感最自然。`speed` 只适合音视频时长差异在 **10% 以内**的场景；差异过大时画面会明显卡顿（慢放）或鬼畜（快放），音频还会变调。
>
> 单个输入文件建议不超过 10 GB。

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/mux-audio-video' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/my_vlog.mp4",
    "audio_url": "https://example.com/bgm.mp3"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-mux-audio-video-1704500",
  "message": ""
}
```

拿到 `task_id` 后用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 为 `completed` / `failed`：

```json
{
  "success": true,
  "task_id": "amk-tool-mux-audio-video-1704500",
  "task_type": "mux-audio-video",
  "status": "completed",
  "result": {
    "video_url": "https://storage.example.com/outputs/xxx.mp4?auth_key=...",
    "resolution": "540p",
    "duration": 240.578
  },
  "expires_at": 1774615474,
  "finished_at": 1774442675
}
```

**产物字段：** `result.video_url`（约 24h 有效的临时下载链接，请及时转存）、`result.resolution`（输出分辨率档）、`result.duration`（**秒**）。默认输出容器为 **MP4**。

> ⚠️ 拿 `.mp4` 后缀当心：`?auth_key=...` 是签名参数，拼接其他参数时要用 `&` 而不是 `?`。

**计费：时长 × 分辨率换算系数 × 单价（元/分钟）**

官方对音视频合成不是单一的「元/分钟」——它按**输出分辨率**再乘一个换算系数：

| 输出分辨率档 | 换算系数 |
|---|---|
| 4K（3840×2160）及以下 | 24 |
| 2K（2560×1440）及以下 | 12 |
| 1080P（1920×1080）及以下 | 6 |
| 720P（1280×720）及以下 | 3 |
| 540P（720×540）及以下 | 2 |
| 480P（640×480）及以下 | 1.5 |
| 360P（480×360）及以下（含纯音频） | 1 |

即：`费用 = 输出时长（分钟）× 系数 × 单价`。系数取**输出分辨率**（`result.resolution`）落进的那一档，不是输入视频的分辨率——比如输入 1080P、输出 540P，按 540P 的 `2` 算。

> ⚠️ **网关目前按平坦的「元/分钟」计费，没有实现这个分辨率系数。** 结算路径（`AdjustBillingOnCompleteWithClamp`）用的倍率只含真实时长与提交时已知的维度，而分辨率只有**任务完成后**从 `result.resolution` 才拿得到，现有结构里没有承载「响应派生倍率」的位置。**本期对客零价，两种口径的结果都是 0，不影响**；但**开始收费前必须先决定**：要么把该系数补进结算，要么知悉并按平坦单价定价（等于对所有分辨率档收同一个价，低分辨率档多收、高分辨率档少收）。跟踪工单：**IKITTA**。
>
> 另一处未实测：本工具的上游 slug 与产物键取自官方文档，**尚未在真实上游调用过**（本仓踩过文档 slug 写错的前例——文档写 `scale-image`、真 slug 是 `resize-image`）。验收时以「返回的 400 来自上游并报缺字段」为准——那是 slug 存在的证据。

---


---

## 7. 音频工具（异步，8 个）

全部为异步工具，端点为 `POST /v1/mediakit/{tool}`。流程同视频工具。除「音频元信息获取」按次计费外，其余均按处理时长计费。

> 其中 `extract-audio`（视频抽音轨）、`trim-audio`（音频截取）、`mix-audio`（音频混合）、`adjust-audio-speed`（音频调速）**不在火山官方那三个工具集合内**——它们不属于视频 33 / 图像 27 / 音频 4 的任何一类，是分别应下游同名需求额外接入的，上游各有独立 slug 与独立产物。其余 4 个是官方音频集合的完整登记。
>
> 本节工具的输出都是**纯音频**，在官方的分辨率换算系数表里落在最低档（系数 `1`），所以下面各节的「元/分钟」就是完整口径，**不涉及分辨率系数**——那个只影响输出视频的 [6.34 音视频合成](#634-音视频合成-mux-audio-video)。
>
> 三条集合外工具（`mix-audio` / `adjust-audio-speed` 及视频侧的 `mux-audio-video`）**均尚未在真实上游调用过**，上游 slug 与产物键取自官方文档（本仓踩过文档 slug 写错的前例——文档写 `scale-image`、真 slug 是 `resize-image`）。`extract-audio` / `trim-audio` 则是已实测核对的。

### 7.1 人声背景音分离 `separate-voice`

**作用：** 把一段音频/视频里的人声与背景音拆成互相独立的音轨文件，默认输出「纯净人声」和「完整伴奏」两条轨；可选择更贴合内容类型的分离场景，把短剧/口播素材进一步拆成人声、音乐、音效三轨。适合配音替换、卡拉 OK 伴奏提取、视频二次创作中单独调人声或换 BGM、以及先提纯人声再送 ASR 以提升识别准确率等场景。

- **端点：** `POST /v1/mediakit/separate-voice`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 否 | 待分离的视频地址，支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议。与 `audio_url` 二选一（视频、音频输入各传其一） |
| `audio_url` | string | 否 | 待分离的纯音频文件地址，协议支持同上。与 `video_url` 二选一 |
| `scene` | string | 否 | 分离场景，决定输出几轨：`Audio`（默认，通用音视频，二分法，输出人声 + 背景音）、`Music`（音乐，二分法，输出人声 + 背景音）、`Drama`（短剧，三分法，输出人声 + 音乐 + 音效）、`Narrate`（口播，三分法，输出人声 + 音乐 + 音效） |
| `output_format` | string | 否 | 输出音频格式，可选 `aac`、`mp3`、`wav`、`m4a`、`flac`，默认 `aac` |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/separate-voice' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_video.mp4",
    "scene": "Audio",
    "output_format": "aac"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-separate-voice-12345678",
  "task_type": "separate-voice",
  "status": "completed",
  "result": {
    "voice_audio_url": "https://example.volcvideo.com/voice.aac?auth_key=...",
    "background_audio_url": "https://example.volcvideo.com/background.aac?auth_key=...",
    "duration": 120.5
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

> `scene` 取 `Drama` 或 `Narrate` 时为三轨输出，`result` 中为 `voice_audio_url` + `music_audio_url` + `sfx_audio_url`，不含 `background_audio_url`。

**产物字段：** 多轨产物。本网关取人声轨 `result.voice_audio_url`；伴奏轨在 `result.background_audio_url`，音乐场景另有 `result.music_audio_url` / `result.sfx_audio_url`，完整结构见上方响应示例

---

### 7.2 音频转码 `transcode-audio`

**作用：** 把音频从一种格式转换成另一种格式，并在转码的同时精细调整编码参数与响度——容器格式（如 WAV 转 MP3）、码率模式与码率、音量均衡、元信息保留/改写都可控。适合为不同播放终端适配格式、压缩体积优化网络传输、以及把媒资库里来源不一的音频统一响度标准化等场景。

- **端点：** `POST /v1/mediakit/transcode-audio`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `audio_url` | string | 是 | 待转码音频地址，支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议。不支持 `video_url` 输入 |
| `container_format` | string | 否 | 输出容器格式，示例取值 `M4A`。不传时的默认行为为输出 MP3、128kbps、48000Hz（其余可选取值文档未列出，可参照支持的音频格式推断） |
| `audio.bitrate_mode` | string | 否 | 码率模式：`cbr` 恒定码率，编码器严格保持码率恒定、文件体积可精确预测，适合对带宽稳定性要求高的流式传输；`cae` 自适应码率，编码器按音频复杂度动态调整，相同平均码率下音质更好、体积更小，仅 AAC 支持 |
| `audio.bitrate_kbps` | integer | 否 | 目标码率（kbps）。默认 128 |
| `audio.volume_method` | string | 否 | 音量均衡方式，设为 `"2Pass"` 时启用两遍响度标准化 |
| `audio.volume_integrated_loudness` | number | 否 | 目标响度（LUFS），如 `-14` 表示标准化到流媒体平台常用的 -14 LUFS |
| `audio.volume_true_peak` | number | 否 | 安全真实峰值上限（dBTP），如 `-1.5` |
| `metadata_keep_tags` | array\<string\> | 否 | 指定需要从源文件保留的元信息标签名列表，如 `["title", "artist"]` |
| `metadata_add_tags` | array\<object\> | 否 | 新增或覆盖元信息标签，元素为 `{key, value}` 对象 |
| `metadata_add_tags[].key` | string | 条件必填 | 标签名。填写 `metadata_add_tags` 时必填（推断） |
| `metadata_add_tags[].value` | string | 条件必填 | 标签值。若 key 与 `metadata_keep_tags` 保留的标签同名，此处的 value 会覆盖源文件的值（推断） |

> 文档正文提到还可调整采样率与声道数（默认 48000Hz），但正文、示例与参数表中均未给出对应字段名，此处不臆测。

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/transcode-audio' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "audio_url": "https://example.com/your_audio.m4a",
    "container_format": "M4A",
    "audio": {
      "bitrate_mode": "cae",
      "bitrate_kbps": 128,
      "volume_method": "2Pass",
      "volume_integrated_loudness": -14,
      "volume_true_peak": -1.5
    },
    "metadata_keep_tags": ["title", "artist"],
    "metadata_add_tags": [
      {"key": "album", "value": "Remastered 2024"}
    ]
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-transcode-audio-123456789",
  "status": "completed",
  "result": {
    "audio_url": "https://example.com/transcode_output.mp3?auth_key=...",
    "duration": 240.8
  },
  "expires_at": 1777531298,
  "created_at": 1777358413,
  "finished_at": 1777358500
}
```

**产物字段：** `result.audio_url`

---

### 7.3 视频抽音轨 `extract-audio`

**作用：** 把视频里的音轨单独抽出来存成一个音频文件，视频画面丢弃。与 [7.1 人声背景音分离](#71-人声背景音分离-separate-voice) 不同——那个是把音频拆成人声与伴奏两条轨，本工具只做「视频 → 音频」的提取，不做分离。适合从视频素材里取配音/原声做二次利用、给语音转写准备纯音频输入、以及把视频里的 BGM 单独导出等场景。

- **端点：** `POST /v1/mediakit/extract-audio`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `video_url` | string | 是 | 待提取音轨的视频地址（公网 HTTP/HTTPS URL）。缺此字段上游直接报 `InvalidParameter: required: missing properties: ["video_url"]` |
| `format` | string | 否 | 输出音频格式，取值 `mp3` / `m4a` / `ogg` / `flac` / `wav`。**默认 `m4a`**。非法值会被上游拒绝，不会静默回退 |

> 本工具不在火山官方「视频 / 图像 / 音频」三个工具集合内，参数以上游实测为准。传参一律走**网关**，不要直连上游。

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/extract-audio' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "video_url": "https://example.com/your_video.mp4",
    "format": "m4a"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-extract-audio-1163743796738",
  "message": ""
}
```

拿到 `task_id` 后用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 为 `completed` / `failed`：

```json
{
  "success": true,
  "task_id": "amk-tool-extract-audio-1163743796738",
  "status": "completed",
  "result": {
    "audio_url": "https://example.com/extracted.m4a?auth_key=...",
    "duration": 38.961
  }
}
```

**产物字段：** `result.audio_url`（约 24h 有效的临时下载链接，请及时转存）、`result.duration`（秒，精确值）。

---

### 7.4 音频截取 `trim-audio`

**作用：** 按指定的起止时间点从一段较长的音频里「切」出一个新片段，生成一个独立的音频文件。与 [7.3 视频抽音轨](#73-视频抽音轨-extract-audio) 不同——那个是换容器（视频 → 音频、取整条音轨），本工具是**按时间裁剪**，入参必须给起止秒数。适合从长录音里截取核心对话、从完整演出里剪出单曲片段、以及先裁短再送 ASR 以省识别成本等场景。

- **端点：** `POST /v1/mediakit/trim-audio`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `audio_url` | string | 是 | 待裁剪音频地址，支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议。**注意参数名是 `audio_url`**：本工具与视频裁剪（`/api/v1/tools/trim-video`）流程完全一致，唯一区别就是这里换成音频地址 |
| `start_time` | number | 是 | 裁剪起点，**单位秒**，数值类型。如 `930` 表示第 15 分 30 秒 |
| `end_time` | number | 是 | 裁剪终点，**单位秒**，数值类型，必须晚于 `start_time` |

> 建议提交前自行校验：`end_time > start_time`，且两者都在源文件总时长之内。无效参数会被上游拒绝、任务失败，白跑一趟。
>
> 本工具不在火山官方「视频 / 图像 / 音频」三个工具集合内，参数以上游实测为准。传参一律走**网关**，不要直连上游。

**输出规格**

默认输出容器为 **m4a**。

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/trim-audio' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "audio_url": "https://example.com/full_recording.m4a",
    "start_time": 930,
    "end_time": 960
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-trim-audio-1456354114306",
  "message": ""
}
```

拿到 `task_id` 后用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 为 `completed` / `failed`：

```json
{
  "success": true,
  "task_id": "amk-tool-trim-audio-1456354114306",
  "task_type": "trim-audio",
  "status": "completed",
  "result": {
    "audio_url": "https://example.com/clip.m4a?auth_key=...",
    "duration": 3
  },
  "expires_at": 1790127215,
  "created_at": 1790040806,
  "finished_at": 1790040816
}
```

**产物字段：** `result.audio_url`（约 24h 有效的临时下载链接，请及时转存）、`result.duration`（秒）。注意这里的 `duration` 是**裁剪后片段**的时长，不是源文件时长——按它结算。

---

### 7.5 语音端点识别 `detect-voice-activity`

**作用：** 从音频/视频里找出「哪几段有人在说话」——自动定位有效语音的起止时间，把有人声的片段与静音、背景噪声区分开，返回一份带时间戳的语音片段列表。适合按说话段落自动切片、跳过静音段再做 ASR/转写以节省识别成本、以及给后续分析或模型训练准备干净音频输入等场景。

- **端点：** `POST /v1/mediakit/detect-voice-activity`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `audio_url` | string | 否 | 待识别音频地址，支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议。与 `video_url` 二选一 |
| `video_url` | string | 否 | 待识别视频文件地址（直接从视频音轨检测），协议支持同上。与 `audio_url` 二选一 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/detect-voice-activity' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "audio_url": "https://example.com/your_audio.wav"
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-detect-voice-activity-786355206146",
  "task_type": "detect-voice-activity",
  "status": "completed",
  "result": {
    "duration": 94.68,
    "segment_count": 27,
    "voice_segments": [
      {"start_time": 5.52, "end_time": 7.28},
      {"start_time": 7.52, "end_time": 8.56},
      {"start_time": 9.68, "end_time": 10.96},
      {"start_time": 11.44, "end_time": 12.48},
      {"start_time": 13.68, "end_time": 14.72}
    ]
  },
  "expires_at": 1788888888,
  "created_at": 1788666666,
  "finished_at": 1788666777
}
```

> `result.voice_segments` 为对象数组，每项含 `start_time` / `end_time`（单位：秒），代表一段有效语音的起止时间；`result.segment_count` 为片段总数（示例为 27，此处仅节选前 5 段）。

**产物字段：** 无文件产物，语音分段见上方响应示例的 `result.voice_segments[]`

### 7.6 音频元信息获取 `probe-audio-metadata`

**作用：** 探测一个音频 URL 的媒资元信息——容器层信息与音频流信息，**不转码、不产出新文件**。适合入库前校验格式与编码、按采样率/声道数分流处理策略、核对接入素材是否符合前置条件等场景。与 [6.33 视频元信息获取](#633-视频元信息获取-probe-video-metadata) 同族，只是输入换成纯音频。

- **端点：** `POST /v1/mediakit/probe-audio-metadata`
- **计费：** 按次，0.0001 元/次

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `audio_url` | string | 是 | 待探测的音频地址。支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp3/m4a/wav/wma/amr/aac/ogg/flac 等格式 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/probe-audio-metadata' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "audio_url": "https://example.com/your_audio.mp3"
  }'
```

**响应**

提交成功后返回 `task_id`，用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 变为 `completed`：

```json
{
  "success": true,
  "task_id": "amk-tool-probe-audio-metadata-123456789",
  "request_id": "20240521150000ABCDEFG12345",
  "status": "completed",
  "result": {
    "format_meta": {
      "format": "mp3",
      "duration": 183.2,
      "size": 4392145
    },
    "audio_stream_meta": {
      "codec": "mp3",
      "sample_rate": 44100,
      "channels": 2
    }
  },
  "expires_at": 1777464650,
  "created_at": 1777291767,
  "finished_at": 1777291851
}
```

**产物字段：** 无文件产物。两段元信息（`format_meta` / `audio_stream_meta`）就在 `result` 里，网关不返回 `url` 字段。

---

### 7.7 音频混合 `mix-audio`

**作用：** 把多路音频**混成一条新音频**——背景音乐、音效、旁白各自独立成文件，合成时同时播放。适合给播客做带 BGM 与音效的片头、把多段分段录音并成一段完整节目、以及为纯人声素材铺一层背景音乐等场景。与 [6.34 音视频合成](#634-音视频合成-mux-audio-video) 的区别是：那个的输出是视频且输入必带视频，这个只进出音频、不碰画面。

- **端点：** `POST /v1/mediakit/mix-audio`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `audio_urls` | string[] | 是 | 待混合的音频地址**数组**，最少 1 个、最多 100 个。每项支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议；支持 mp3/wav/flac 等主流格式 |

> 单个输入文件建议不超过 10 GB。

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/mix-audio' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "audio_urls": [
      "https://example.com/narration.mp3",
      "https://example.com/background_music.mp3"
    ]
  }'
```

**响应**

```json
{
  "success": true,
  "task_id": "amk-tool-mix-audio-1501200",
  "message": ""
}
```

拿到 `task_id` 后用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 为 `completed` / `failed`：

```json
{
  "success": true,
  "task_id": "amk-tool-mix-audio-1501200",
  "task_type": "mix-audio",
  "status": "completed",
  "result": {
    "audio_url": "https://example.volcvideo.com/mixed_output.mp3?auth_key=...",
    "duration": 185.5
  }
}
```

**产物字段：** `result.audio_url`（约 24h 有效的临时下载链接，请及时转存）、`result.duration`（**秒**）。默认输出容器为 **MP3**。结算按 `result.duration` 走。

> **输出时长由最长的输入决定。** 混一段 30 秒旁白与一段 60 秒 BGM，成品是 60 秒：前 30 秒两条同时播放，后 30 秒只剩 BGM。按多长的素材送进来就按多长计费，先裁短再混能省钱。
>
> ⚠️ 上游 slug 与产物键取自官方文档，**尚未在真实上游调用过**。验收时以「返回的 400 来自上游并报缺字段」为准。

---

### 7.8 音频调速 `adjust-audio-speed`

**作用：** 改变一段音频的播放倍速（快放 / 慢放），并**通过算法保持音调自然**——不是简单重采样导致的「变尖 / 变沉」。适合播客与有声书的倍速版、教学演示与口播的节奏调整、以及把素材时长对齐到目标时长等场景。

- **端点：** `POST /v1/mediakit/adjust-audio-speed`
- **计费：** 按时长（元/分钟）

**请求参数**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `audio_url` | string | 是 | 待调速音频地址，支持公网 HTTP/HTTPS URL、本地上传 (`mediakit://`)、火山引擎视频点播 (`vod://`)、火山引擎对象存储 (`tos://`) 四种协议 |
| `speed` | number | 是 | 播放倍速。**大于 `1.0` 快放**（如 `2.0` 表示 2 倍速），**小于 `1.0` 慢放**（如 `0.5` 表示半速）。`1.0` 为原速 |

**调用示例**

```bash
curl -X POST 'https://<网关域名>/v1/mediakit/adjust-audio-speed' \
  -H 'Authorization: Bearer sk-<你的网关令牌>' \
  -H 'Content-Type: application/json' \
  -d '{
    "audio_url": "https://example.com/your_audio.mp3",
    "speed": 2.0
  }'
```

**响应**

拿到 `task_id` 后用 `GET /v1/mediakit/tasks/{task_id}` 轮询，直到 `status` 为 `completed` / `failed`。响应形状与视频调速一致，只是产物键为 `audio_url`：

```json
{
  "success": true,
  "task_id": "amk-tool-adjust-audio-speed-123456789",
  "task_type": "adjust-audio-speed",
  "status": "completed",
  "result": {
    "audio_url": "https://storage.example.com/outputs/xxx.mp3?auth_key=...",
    "duration": 16.47
  }
}
```

**产物字段：** `result.audio_url`（约 24h 有效的临时下载链接，请及时转存）、`result.duration`（调速**后**的时长，**秒**）。按它结算。

> **输出时长 ≈ 输入时长 ÷ `speed`。** 一段 32.94 秒的音频按 `2.0` 处理得到约 16.47 秒。注意计费按**输出**时长走，快放会**减少**收费时长。
>
> 产物键为 `audio_url`（**非**视频调速的 `video_url`）——两者流程完全一致，唯一区别就是路径与这个入参/产物键名。
>
> ⚠️ **本工具尚未在真实上游调用过**，上游 slug 取自官方文档（本仓踩过文档 slug 写错的前例——文档写 `scale-image`、真 slug 是 `resize-image`）。验收时以「返回的 400 来自上游并报缺字段」为准。
>
> 另：「视频调速」`adjust-video-speed` 与本工具同族但**网关未登记**（不在本期范围），调用 `POST /v1/mediakit/adjust-video-speed` 会得到 `unknown mediakit tool`。

---


---

## 8. 错误处理

三类错误，响应形状不同：

### 8.1 鉴权失败（HTTP 401）

```json
{
  "success": false,
  "message": "未提供令牌..."
}
```

### 8.2 同步工具错误（HTTP 4xx/5xx）

走网关统一的错误信封：

```json
{
  "error": {
    "message": "mediakit upstream error ...",
    "type": "new_api_error",
    "param": "",
    "code": "..."
  }
}
```

常见情形：

| HTTP | 原因 | 处理 |
|---|---|---|
| `400` | 工具名未登记（拼错 slug） | 对照第 4 节总览表检查路径 |
| `400` | 把**异步**工具打到了 `/v1/mediakit/sync/` 上 | 异步工具改用 `POST /v1/mediakit/{tool}` |
| `400` | 请求体里传了 `batch_count > 1` | 只传 `1` 或不传 |
| `400` | 该模型未在网关配定价 | 联系管理员配置模型定价 |
| `400` | 上游业务失败（HTTP 200 + `success: false`） | 看 `error.message`，上游原因 |
| `401` | 令牌无效 / 未提供 | 检查 `Authorization` 头 |

### 8.3 异步工具错误

**提交阶段**（`POST /v1/mediakit/{tool}`）：

```json
{
  "code": "unknown_tool",
  "message": "unknown mediakit tool: xxx",
  "data": null
}
```

常见情形：

| HTTP | 原因 | 处理 |
|---|---|---|
| `400` | 工具名未登记（拼错 slug） | 对照第 4 节总览表检查路径 |
| `400` | 把**同步**工具打到了异步路由 | 同步工具改用 `POST /v1/mediakit/sync/{tool}` |
| `400` | 该模型未在网关配定价 | 联系管理员配置模型定价 |
| `400` | 请求体不符合上游 schema（缺字段、类型错、枚举值越界） | 看 `message` 里的上游原文，改请求体——重试不会成功 |
| `502` | 上游返回成功却没给 `task_id` | 上游侧异常，联系管理员 |

提交阶段上游以 HTTP 200 + `success: false` 表达业务失败，网关把这一种统一映射成 `400`。
实测这类失败只出现在**请求体不符合上游 schema**（缺字段、类型错、枚举值越界，如 `fps: 999`）。
素材层面的问题——URL 打不开、内容与工具不符——上游会正常受理任务，失败延后到轮询期以
`status: failed` 表达，不在提交阶段报错（这一点与 [8.2](#82-同步工具错误http-4xx5xx) 的同步工具不同：
同步工具内联下载素材，素材不可用会直接 `400`）。

**任务失败**不通过 HTTP 状态码表达——任务会正常受理，之后查询时 `status` 变成 `failed`，失败原因在 `message` 里：

```json
{
  "success": true,
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "status": "failed",
  "message": "上游失败原因..."
}
```

**失败不计费**，提交时的预扣额度会全额退回。

---

## 9. 上线前的管理员配置

> 本节是给**部署/运维**看的，调用方可以跳过。

要让这些工具真正可用，除了部署带 MediaKit 支持的代码，还要在网关里配好三样：

1. **建一个 MediaKit 渠道**（渠道类型选 MediaKit）
   - **API Key 填在渠道的「密钥」字段**，不是你自建的别的字段——MediaKit 用单头 `Authorization: Bearer <apiKey>` 鉴权，天然适配渠道密钥机制
   - 上游域名走渠道的「代理/Base URL」字段，默认 `https://mediakit.cn-beijing.volces.com`，一般不用改
2. **把要开放的模型勾进渠道的模型列表**
   - 对客模型名是 `mediakit-<工具名>`，例如 `mediakit-enhance-image`。共 69 个，按需勾选即可分批放量
   - 少勾一个，对应的工具就无法路由
3. **配模型定价**
   - **同步工具必须配**，否则会直接 `400`（报 `has no model price configured`）；按次/按量的工具填固定价即可
   - 异步工具也要配——按时长的工具按「元/分钟」填，提交时按预估时长预扣、完成后按真实时长结算差额
   - 3 个元信息工具按 [3.6](#36-计费口径) 的口径填：视频/音频元的 0.0001 元/次、图像元的 0.00024 元/次（脚本会自动带上这三个价）
   - **本期零价的 4 个工具要显式配 `0`，不是不配**：`mediakit-trim-audio`、`mediakit-mix-audio`、`mediakit-mux-audio-video`、`mediakit-adjust-audio-speed`。网关的硬规则是「未配价格直接 400」，所以「免费」的正确形态是配零价；不配会被当成没定价而拒掉。脚本里它们各自有独立变量（`@price_trim` / `@price_mix_audio` / `@price_mux_audio_video` / `@price_adjust_audio_spd`），**不跟随** `@price_per_call`——它们是时长类工具、单价单位是元/分钟，与按次的通用单价不同量纲

另有两个容易踩的点：

- **令牌的模型白名单**：如果给调用方的令牌限制了可用模型，需要把 `mediakit-<工具名>` 加进白名单，否则会被令牌层拦掉。
- **`mediakit_default_minutes`**：渠道设置里的一项（0–60），用于时长类工具**提交时拿不到真实时长**的估算预扣。填 0 时预扣退回按 `model_price` 基准额度（约 1 分钟量），真实时长的差额在任务完成时补扣——不是「预扣为 0、先用后付」。

一键配置脚本：`scripts/mediakit-bootstrap.sql`（建渠道 + 写全 69 个模型 + 派生路由表 + 可选写定价，幂等可重复执行）。

> 若目标环境设了 `MEMORY_CACHE_ENABLED=true`，直接改数据库不会立刻生效，需要重启服务或触发一次渠道缓存刷新。

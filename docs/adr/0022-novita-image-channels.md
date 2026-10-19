# 新开 Novita 两个同步图片渠道（Gemini 协议 / OpenAI 协议）

Novita AI 提供两个互不兼容的图片端点：`https://api.novita.ai/openai` 走 OpenAI 图片协议（`/v1/images/generations`、`/v1/images/edits`），`https://api.novita.ai/gemini` 走 Gemini 原生协议（`{base}/v1/models/{model}:generateContent`）。两者协议、路径与请求体形状都不同，因此各开一个渠道类型。

**Status:** accepted

## 背景

下游客户端统一使用 OpenAI 图片请求体（与既有 `jiekouAI` / HighwayAPI 渠道一致），模型来自 Novita 的 `gpt-image-2` 系列与 Gemini `gemini-*-image`（nano-banana-2）系列。需要把这两批模型接入 `/v1/images/generations` 与 `/v1/images/edits`。

现有 Gemini 渠道不能直接复用：

- `gemini.ConvertImageRequest` 只接受 `imagen*` 模型，其余直接报错，且构造的是 imagen 的 `:predict` 请求体。
- 图片响应分支同样按 `imagen` 前缀判断，`gemini-*-image` 会落进 chat handler。
- Gemini 版本号是按模型名全局配置的（默认 `v1beta`），Novita 需要 `/v1`；而 `gemini-3-pro-image`、`gemini-2.5-flash-image` 与 Google 官方模型同名，改全局配置会打到现有 Google 渠道。

OpenAI 渠道可以直接承接 `gpt-image-2`：`ConvertImageRequest` 对 generations/edits 均为透传，`GetRequestURL` 拼 `base_url + 请求路径`，把 base_url 指向 `https://api.novita.ai/openai` 即可。

## 决策

新增两个同步渠道，均注册进 `RelayFormatOpenAIImage` → `ImageHelper` 链路，**不进**异步任务系统：

- `ChannelTypeNovitaGemini`（type 68，包 `relay/channel/novitagemini/`），默认 base_url `https://api.novita.ai/gemini`。
- `ChannelTypeNovitaOpenAI`（type 69，包 `relay/channel/novitaopenai/`），默认 base_url `https://api.novita.ai/openai`。

### 渠道 68（NovitaGemini）

- URL：`{base_url}/v1/models/{url.PathEscape(UpstreamModelName)}:generateContent`，版本段固定 `v1`，不读全局 `gemini.version_settings`。
- 请求转换：`prompt` → `contents[0].parts[0].text`；generations 固定 `responseModalities:["IMAGE"]`，edits 为 `["TEXT","IMAGE"]`。
- 编辑输入：multipart 的 `image` / `image[]` / `image[N]` 文件，或 JSON `images[].image_url` 的 data URL；每个输入图转为一个 `inlineData` part。`http(s)://` 外链显式拒绝（SSRF）。单图与总解码体积分别有上限（20MB / 40MB），最多 16 张。
- `mask` 不支持，显式返回 400（不静默忽略）。
- `size` 不映射、不写 `imageConfig`；`n` 只允许 1；不支持流式；`response_format` 只允许 `b64_json`（multipart 请求同样校验）。
- 响应转换：`candidates[].content.parts[].inlineData.data` → `data[].b64_json`；无图报错。上游内容策略拦截（`IMAGE_RECITATION`、`SAFETY`、`PROHIBITED_CONTENT` 等）返回 400 并在错误中保留 `finishReason` / `blockReason`；其他无图情况返回 500，不触发重试。
- 计费：解析 `usageMetadata`（含 thought tokens）；completion 为 0 但有图时按每张 1400 token 兜底，与既有 Gemini 原生图片路径口径一致。
- 默认模型：实测通过的 9 个。`gemini-3.1-flash-lite-image` 在对接账号下返回 403，未纳入默认列表。

### 渠道 69（NovitaOpenAI）

- 薄包装 `openai.Adaptor`，只覆盖 `GetModelList` 与 `GetChannelName`。
- 请求、响应、错误映射全部复用 OpenAI 适配器，零新增转换逻辑。
- 参数约束由上游决定：与渠道 68 不同，69 不限制 `n=1`、不禁止 `stream`、不强制 `b64_json`；OpenAI 适配器把客户端字段原样转发。运营配置时不要把两个渠道的参数能力写成同一套。
- 计费沿用 `normalizeOpenAIUsage`，把上游 `input_tokens` / `output_tokens` 映射为 prompt/completion token。
- 默认模型：`gpt-image-2`、`gpt-image-2.5-flare-oai`、`gpt-image-2.5-sunburst-oai`。

### 共同约定

- 两个渠道都标记为不支持渠道测试，避免管理员点一次「测试」就真实出图（单张最慢实测 102 秒）。
- 渠道 68 在 `DoRequest` 显式拒绝透传模式：否则 `ImageHelper` 会跳过转换，把客户端原始 OpenAI 请求体直接打到 Gemini 端点。
- 旧渠道 63（jiekouAI / HighwayAPI）保持不变，迁移期可并行。

## 理由

按 ADR-0003-xfai-video-channel 的渠道边界原则，边界由上游协议与端点决定。把 Gemini 图片支持塞进现有 Gemini 渠道会同时改动三个共享判断点，并引入按模型名全局生效的版本冲突；独立渠道把风险隔离在一个新包里。

渠道 69 复用 OpenAI 适配器而不是重写，是因为 Novita 的 `/openai` 与 OpenAI 图片协议完全一致，任何转换都只会增加偏差。

计费选 token 而不是按张，是因为上游返回真实 `usageMetadata`（不同模型单次 1126–1866 token，含 thought），按张固定价会把成本差异拉平。

## 影响

- 新增渠道类型 68 / 69、两个 APIType、`ChannelType2APIType` 映射、`GetAdaptor` 分支、`ChannelBaseURLs` 第 68/69 项与前端类型配置。
- 两个渠道加入 `unsupportedTestChannelTypes`。
- 下游模型名需要管理员在渠道模型列表或模型映射里配置；`jiekouAI` 渠道的既有配置不受影响。

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

新增两个同步渠道，均注册进 `RelayFormatOpenAIImage` → `ImageHelper` 链路，**不进**异步任务系统。渠道 68 之后又扩出一条 Gemini 原生链路（`RelayFormatGemini` → `GeminiHelper`），见下文「渠道 68 的原生 Gemini 下游链路」。

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
- 默认模型：实测通过的 10 个。`gemini-3.1-flash-lite-image` 在对接账号下返回 403，未纳入默认列表；`-as` 系列走 Ai Studio 资源池，历史上（2026-10-19）有成功调用记录，2026-11-04 复测失败，属间歇性可用——保留在默认列表，但不要当成默认路径。`nano-banana-2-t2i` 是在下游名与上游名不一致的渠道里当映射源名用的（渠道 34 即 `nano-banana-2-t2i → gemini-nano-banana-2.1`），默认列表带上它，新渠道开箱即能用官方文档里的模型名调用；对已显式配置 `models` 的渠道无影响。

### 渠道 68 的原生 Gemini 下游链路

渠道 68 同时承接两套下游协议，由下游请求格式决定走哪条，不新开渠道类型：

| 下游入口 | 网关链路 | 适配器行为 |
| --- | --- | --- |
| `/v1/images/generations`、`/v1/images/edits` | `RelayFormatOpenAIImage` → `ImageHelper` | 翻译成 Gemini 请求体，响应还原成 OpenAI images 结构 |
| `/v1/models/{model}:generateContent`、`/v1beta/models/{model}:generateContent` | `RelayFormatGemini` → `GeminiHelper` | 语义透传，非流式响应原字节回下游 |

不拆渠道的理由：两者是同一个上游、同一个 key、同一个 base_url，协议差异在请求到达适配器后已经由 relay format 区分开，再开一个渠道类型只是让运营把同一份配置抄两遍。

- URL 版本段仍固定 `/v1`；流式动作为 `:streamGenerateContent?alt=sse`，并设 `DisablePing`，避免 ping 帧污染 SSE。
- 请求方向是**语义透传**：适配器只做批量拒绝与内联图上限（16 张 / 单张 20MB / 合计 40MB，先按编码长度判定再解码），其余原样 `return request`。`generationConfig.imageConfig`、`responseModalities`、`thinkingConfig` 等已建模字段全量到达上游；未建模字段在解析-序列化往返中丢失。非透传模式下 `GeminiHelper` 的渠道级 SystemPrompt、thinking 适配与 `param_override` 会先于适配器生效，所以「语义透传」不等于请求体逐字保真；**透传模式（全局或渠道 `PassThroughBodyEnabled`）下 `GeminiHelper` 直接转发原始 body，SystemPrompt 与 `param_override` 都不会生效**，只有模型映射仍会执行。
- 透传模式（全局或渠道 `PassThroughBodyEnabled`）会跳过 `ConvertGeminiRequest`，上述校验由 `DoRequest` 对已解析的 `info.Request` 再执行一遍，不会因透传而失效。图片链路依旧拒绝透传。
- 响应方向复用 `gemini.GeminiTextGenerationHandler` / `GeminiTextGenerationStreamHandler`：整包读出、旁路解析 `usageMetadata` 计费、原始字节写回下游，不重新序列化。非流式读取在适配器侧套了 64MB 上限，防止异常上游把整包读进内存；共享处理器的「每张 1400 token」兜底在适配器侧按 16 张封顶，上游真实 `usageMetadata` 不受影响。
- 流式方向不是原字节：共享的流式处理器逐事件重写成 `data: ...\n\n` 帧，事件内容本身不变。
- `DoRequest` 的透传拦截只保留给图片模式。原生链路的原始体本来就是 Gemini 协议，运维开全局透传时应当放行。
- 非生图动作（`:embedContent`、`:batchEmbedContents`、`:countTokens` 等）在 `DoRequest` 直接返回 400。`Path2RelayMode` 把它们也归入 `RelayModeGemini`，而 `GetRequestURL` 只会拼 `:generateContent`；不拦的话这些请求会被改写成一次真实的图片生成，既语义错位又产生计费。向量端点还额外绕过了 `ConvertGeminiRequest`（`GeminiEmbeddingHandler` 直接序列化请求体），只能在 `DoRequest` 这一层拦。
- 共存约束：翻译链路不支持流式、丢失 `size`/`quality`/`background`/`output_format`/`moderation`、`n` 只能为 1；原生链路不丢参数，支持流式。两条链路的错误表与参数能力不要合并成一套文档描述。
- 流式**不提供渐进图**：上游实测只发一个 `data` 事件（`ttfb=2.71s`、`total=229.80s`、4.78MB），整图在最后一次性到达。保留流式是因为客户端协议需要，不是为了缩短等待或规避前置 nginx 的 600 秒上限。

### 渠道 69（NovitaOpenAI）

- 薄包装 `openai.Adaptor`，只覆盖 `GetModelList` 与 `GetChannelName`。
- 请求、响应、错误映射全部复用 OpenAI 适配器，零新增转换逻辑。
- 参数约束由上游决定：与渠道 68 不同，69 不限制 `n=1`、不禁止 `stream`、不强制 `b64_json`；OpenAI 适配器把客户端字段原样转发。运营配置时不要把两个渠道的参数能力写成同一套。
- 计费沿用 `normalizeOpenAIUsage`，把上游 `input_tokens` / `output_tokens` 映射为 prompt/completion token。
- 默认模型：`gpt-image-2`、`gpt-image-2.5-flare-oai`、`gpt-image-2.5-sunburst-oai`。

### 共同约定

- 两个渠道都标记为不支持渠道测试，避免管理员点一次「测试」就真实出图（单张最慢实测 102 秒）。
- 渠道 68 在 `DoRequest` 对图片模式显式拒绝透传模式：否则 `ImageHelper` 会跳过转换，把客户端原始 OpenAI 请求体直接打到 Gemini 端点。原生 Gemini 模式反过来——透传正是期望行为。
- 旧渠道 63（jiekouAI / HighwayAPI）保持不变，迁移期可并行。

## 理由

按 ADR-0003-xfai-video-channel 的渠道边界原则，边界由上游协议与端点决定。把 Gemini 图片支持塞进现有 Gemini 渠道会同时改动三个共享判断点，并引入按模型名全局生效的版本冲突；独立渠道把风险隔离在一个新包里。

渠道 69 复用 OpenAI 适配器而不是重写，是因为 Novita 的 `/openai` 与 OpenAI 图片协议完全一致，任何转换都只会增加偏差。

计费选 token 而不是按张，是因为上游返回真实 `usageMetadata`（不同模型单次 1126–1866 token，含 thought），按张固定价会把成本差异拉平。

## 影响

- 新增渠道类型 68 / 69、两个 APIType、`ChannelType2APIType` 映射、`GetAdaptor` 分支、`ChannelBaseURLs` 第 68/69 项与前端类型配置。
- 两个渠道加入 `unsupportedTestChannelTypes`。
- 渠道 68 的原生链路依赖 `relay/channel/gemini` 的两个处理器；novitagemini 引用 gemini 不构成循环依赖（gemini 不引用 novitagemini）。改这两个处理器会同时影响 Google Gemini 渠道与 Novita Gemini 原生链路。
- 下游模型名需要管理员在渠道模型列表或模型映射里配置；`jiekouAI` 渠道的既有配置不受影响。

## 修订记录

**2026-11-05 修订（IKT0TG）：** 渠道 68 新增「原生 Gemini 下游链路」（见上文同名小节）。两点补充说明：

- **未把渠道 68 加入 `streamSupportedChannels`**（`relay/common/relay_info.go`）。`AGENTS.md` 的 relay 规则要求新渠道确认上游是否支持 `StreamOptions`；此处结论是不支持也不需要：Gemini 原生协议没有 `stream_options` 字段，而该映射的唯一消费者是 `relay/compatibile_handler.go` 与 OpenAI / ali 适配器这类 OpenAI 格式路径，用于设置 `SupportStreamOptions`。Gemini 原生链路（`GeminiHelper`）从不读这个标志，加进去不会产生任何行为，只会让后续读者误以为渠道 68 支持 `stream_options`。
- 原生链路的流式能力由 `info.IsStream` 独立驱动（`GetRequestURL` 切换 `:streamGenerateContent?alt=sse`），与 `streamSupportedChannels` 无关。

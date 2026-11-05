# nano-banana-2 调用文档（实测版）

**日期：** 2026-11-04（2026-11-05 补充 Gemini 原生链路）
**模型：** `nano-banana-2-t2i`
**渠道：** NovitaGemini（type 68）

同一个渠道承接两套下游协议，二选一：

| 链路 | 下游协议 | 网关内部 | 状态 |
|---|---|---|---|
| **Gemini 原生** | Gemini `:generateContent` | 语义透传 | 推荐，参数全量生效 |
| OpenAI Images | `/v1/images/generations` | 翻译成 Gemini | 保留兼容，图片参数会被丢弃 |

> 本文只记录**实际调用并通过**的内容。未验证的项目单独标注。

---

## 1. 基本信息

### 1.1 Gemini 原生链路（推荐）

| 项 | 值 |
|---|---|
| 路径 | `POST /v1beta/models/{model}:generateContent`（`/v1/models/...` 同样可用） |
| 流式路径 | `POST /v1beta/models/{model}:streamGenerateContent?alt=sse` |
| 鉴权 | `Authorization: Bearer <API_KEY>` |
| 请求格式 | `application/json`，Gemini 原生请求体 |
| 返回格式 | Gemini 原生响应体，`inlineData.data` 为 base64 图片 |
| 图片尺寸 | 由 `generationConfig.imageConfig` 决定 |
| 实测耗时 | 非流式 230–320 秒（单张） |

### 1.2 OpenAI Images 链路（旧）

| 项 | 值 |
|---|---|
| 路径 | `POST /v1/images/generations` |
| 返回格式 | **仅 `b64_json`**，无 `url` 字段 |
| 图片尺寸 | 固定 1376×768，`size` 无效 |
| 实测耗时 | **310–320 秒**（单张） |

两条链路都受前置 nginx **600 秒**硬上限约束，建议客户端超时 ≥ 360 秒。

---

## 2. Gemini 原生调用（推荐）

下游直接按 Gemini 协议调用。适配器只做批量拒绝与内联图上限校验，已建模字段
（`generationConfig`、`contents`、`systemInstruction` 等）全量转发；未建模字段会在
网关解析-序列化往返中丢失。

非透传模式下，渠道级 SystemPrompt、thinking 适配与 `param_override` 会先于适配器生效。
**若渠道或全局开启了请求体透传（`PassThroughBodyEnabled`），网关直接转发原始 body，
这三者都不会生效**（模型映射仍会执行）。

本渠道只代理 `:generateContent` / `:streamGenerateContent`；`:embedContent`、
`:batchEmbedContents`、`:countTokens` 等其他动作一律 400。

### 2.1 非流式（已验证 ✅）

```bash
curl -X POST 'https://{api_domain}/v1beta/models/nano-banana-2-t2i:generateContent' \
  -H "Authorization: Bearer sk-****" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{"role": "user", "parts": [{"text": "a cute cat playing with yarn"}]}],
    "generationConfig": {"responseModalities": ["IMAGE"]}
  }'
```

### 2.2 流式（已验证 ✅）

```bash
curl -N -X POST 'https://{api_domain}/v1beta/models/nano-banana-2-t2i:streamGenerateContent?alt=sse' \
  -H "Authorization: Bearer sk-****" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{"role": "user", "parts": [{"text": "a cute cat playing with yarn"}]}],
    "generationConfig": {"responseModalities": ["IMAGE"]}
  }'
```

返回 `text/event-stream`，本次实测只有 **一个** `data:` 事件，详见 2.5 警告 2。

### 2.3 出图尺寸：`generationConfig.imageConfig`

```json
{
  "contents": [{"role": "user", "parts": [{"text": "a cute cat"}]}],
  "generationConfig": {
    "responseModalities": ["IMAGE"],
    "imageConfig": {"aspectRatio": "16:9", "imageSize": "2K"}
  }
}
```

`imageConfig` 是原生链路能控制尺寸的原因。OpenAI Images 链路没有对应字段，忽略 `size`。

### 2.4 响应结构

```json
{
  "candidates": [{
    "content": {
      "role": "model",
      "parts": [
        {"text": "..."},
        {"inlineData": {"mimeType": "image/png", "data": "iVBORw0KGgo..."}}
      ]
    },
    "finishReason": "STOP"
  }],
  "usageMetadata": {
    "promptTokenCount": 6,
    "candidatesTokenCount": 1913,
    "totalTokenCount": 1919
  }
}
```

非流式响应体由网关**原字节回下游**，不做重新序列化。`usageMetadata` 只用于计费（按真实 token，含 thought tokens）。流式响应会被逐事件重写成 `data: ...\n\n` 帧，事件内容本身不变。

### 2.5 三个必读警告

1. **不传 `responseModalities` 会多出一张图。** 上游默认 `["TEXT", "IMAGE"]`，可能同时产出 thought 图和正式图两张，并按真实 token 计费。要单张就显式传 `["IMAGE"]`。
2. **流式不提供渐进图。** 实测：`ttfb=2.71s`、`total=229.80s`、响应体 4,780,986 字节，但只有 **1 个** `data:` 事件——整张图在最后一次性到达。首字节省 3 秒，出图时间与非流式相当，不要把它当成缩短等待的方案。
3. **两条链路都卡在 nginx 600 秒。** 单张实测 230–320 秒，余量不到一半。改图请求会更慢，见第 6 节。

---

## 3. OpenAI Images 调用（旧链路）

### 3.1 最小请求（已验证 ✅）

```bash
curl -X POST https://{api_domain}/v1/images/generations \
  -H "Authorization: Bearer sk-****" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nano-banana-2-t2i",
    "prompt": "a cute cat playing with yarn",
    "n": 1
  }'
```

实测：`HTTP 200`，319.3 秒，1,792,723 B，1376×768 PNG。

### 3.2 带完整字段（已验证 ✅）

```json
{
  "model": "nano-banana-2-t2i",
  "prompt": "a cute cat playing with yarn",
  "n": 1,
  "size": "1024x1024",
  "quality": "high",
  "response_format": "b64_json"
}
```

实测：`HTTP 200`，310.2 秒，1,605,663 B，**1376×768**。

> `size` 和 `quality` **被忽略**。两次实测输出尺寸都是 1376×768，与传参无关。需要控制尺寸请改用第 2 节的 Gemini 原生链路。

### 3.3 响应结构

```json
{
  "created": 1793766875,
  "data": [
    { "b64_json": "iVBORw0KGgo..." }
  ]
}
```

只有 `created` 和 `data` 两个字段。`completion_tokens` 由网关从上游 `usageMetadata` 旁路解析；上游未给 completion 时按每张 **1400 token** 兜底。

---

## 4. 参数支持矩阵（OpenAI Images 链路）

| 参数 | 是否支持 | 实测结果 |
|---|---|---|
| `model` | ✅ 必填 | `nano-banana-2-t2i` |
| `prompt` | ✅ 必填 | 空字符串返回 400 `prompt is required` |
| `n` | ⚠️ 只能是 1 | `n=2` → 400 `novitagemini channel only supports n=1` |
| `size` | ❌ 忽略 | 传 1024x1024 仍输出 1376×768 |
| `quality` | ❌ 忽略 | 传 high 无变化 |
| `response_format` | ⚠️ 只能是 `b64_json` 或不传 | `url` → 400 |
| `stream` | ❌ 不支持 | `true` → 400 `does not support streaming` |
| `mask` | ❌ 不支持 | 返回 400 |
| `background` | ❌ 丢弃 | 适配器不转发给上游 |
| `output_format` | ❌ 丢弃 | 输出固定为 PNG |
| `moderation` | ❌ 丢弃 | 适配器不转发给上游 |

> 这条链路的适配器构造的上游请求体只包含 `prompt`、`responseModalities` 和（编辑时的）输入图。
> Gemini 原生链路不受此表约束——那边是语义透传，已建模字段全量转发。

---

## 5. 模型名与映射

| 配置 | 上游模型 | 结果 |
|---|---|---|
| 无映射 | `nano-banana-2-t2i` | ✅ 200 出图 |
| 映射到 `gemini-nano-banana-2.1` | `gemini-nano-banana-2.1` | ✅ 200 出图（线上渠道 34 的实际配置） |
| 映射到 `gemini-nano-banana-2.1-as` | 同左 | ⚠️ 2026-11-04 复测失败：`upstream response contains no image` |

映射语法：

```json
{ "nano-banana-2-t2i": "目标模型名" }
```

注意：映射的**源名**必须写在下游请求里，渠道 models 里也必须列源名。

**关于 `-as` 系列：** 它走 Ai Studio 资源池，历史上（2026-10-19）该渠道有
`gemini-nano-banana-2.1-as` 的成功记录，2026-11-04 复测失败。属间歇性可用，
不要断言「永久不可用」，但也不要把它配成默认路径。

---

## 6. 改图接口（暂不可用）

OpenAI Images 链路的两种入参都试过，结果一致：

| 方式 | 请求 | 结果 |
|---|---|---|
| multipart | `-F 'image=@apple.png'` + `model=nano-banana-2-t2i` | **504 Gateway Time-out，600.1 秒** |
| JSON | `images[].image_url` 传 data URL | **504 Gateway Time-out，600.1 秒** |

`nano-banana-2-i2i` 模型名则直接 503（该分组未绑定）：

```
POST /v1/images/edits
-F 'model=nano-banana-2-i2i'
→ 503 模型 nano-banana-2-i2i 无可用渠道（distributor）
```

504 由前置 `nginx/1.20.1` 返回，说明请求进了网关但 600 秒内没有响应。

Gemini 原生链路可以带 `inlineData` 参考图（即在第 2 节的请求体 `parts` 里额外加一个
`{"inlineData":{"mimeType":"image/png","data":"<base64>"}}`），但**改图是否能在 600 秒内返回尚未验证**，不在本次可用范围内。

---

## 7. 报错对照

| HTTP | message | 原因 |
|---|---|---|
| 400 | `novitagemini channel only supports n=1` | `n > 1` |
| 400 | `novitagemini channel does not support streaming` | OpenAI Images 链路传了 `stream: true` |
| 400 | `novitagemini channel only supports b64_json response format` | `response_format: "url"` |
| 400 | `prompt is required` | prompt 为空 |
| 400 | `novitagemini channel does not support batch requests` | 原生链路传了 `requests[]` |
| 400 | `novitagemini channel only supports :generateContent and :streamGenerateContent` | 调了 `:embedContent` / `:batchEmbedContents` / `:countTokens` 等非生图动作 |
| 400 | `too many inline images` / `is too large` / `too large in total` | 原生链路内联图超限（16 张 / 单张 20MB / 合计 40MB） |
| 400 | `inline image N is not valid base64` | 原生链路 `inlineData.data` 不是合法 base64 |
| 500 | `upstream response contains no image` | 上游 200 但没返回图片（见第 5 节） |
| 503 | `模型 xxx 无可用渠道（distributor）` | 渠道 models 里没这个名字 |

---

## 8. 快速自检

### 8.1 Gemini 原生（推荐）

```bash
KEY='sk-****'
curl -sS -m 600 'https://{api_domain}/v1beta/models/nano-banana-2-t2i:generateContent' \
  -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' \
  -d '{"contents":[{"role":"user","parts":[{"text":"a cute cat"}]}],"generationConfig":{"responseModalities":["IMAGE"]}}' \
  -o nb-native.json -w 'http=%{http_code} time=%{time_total}s\n'

python3 -c "
import json,base64,struct
d=json.load(open('nb-native.json'))
img=[p['inlineData']['data'] for c in d['candidates'] for p in c['content']['parts'] if 'inlineData' in p][0]
b=base64.b64decode(img)
print('dims',struct.unpack('>II', b[16:24]),'bytes',len(b),'magic',b[:8].hex())
print('usage',d.get('usageMetadata'))
"
```

通过标准：`http=200`，magic `89504e470d0a1a0a`，`usageMetadata` 非空。

### 8.2 OpenAI Images（旧）

```bash
KEY='sk-****'
curl -sS -m 360 https://{api_domain}/v1/images/generations \
  -H "Authorization: Bearer $KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"nano-banana-2-t2i","prompt":"a cute cat","n":1}' \
  -o nb.json -w 'http=%{http_code} time=%{time_total}s\n'

python3 -c "
import json,base64,struct
d=json.load(open('nb.json'))
b=base64.b64decode(d['data'][0]['b64_json'])
print('dims',struct.unpack('>II', b[16:24]),'bytes',len(b),'magic',b[:8].hex())
"
```

通过标准：`http=200`，magic `89504e470d0a1a0a`，尺寸 1376×768。

# New API 网关（blue-whale-api）

聚合 40+ 上游 AI 厂商、对外提供统一 API 的网关，含用户管理、计费与限流。本文档是网关领域的统一语言术语表。

## Language

### 异步任务

**公开任务ID**:
网关下发给客户端的任务标识，形如 task_xxx；客户端凭它查询任务状态与领取结果。
_Avoid_: 任务ID（含糊，也可能指上游任务ID）

**上游任务ID**:
上游厂商受理同一请求后返回的任务标识，仅用于网关与上游通信。历史任务缺失该标识时以公开任务ID充当。
_Avoid_: 真实ID、内部ID

**结果URL（result_url）**:
任务成功后的结果（视频等）访问地址，形态为上游直链或代理URL二者之一。上游返回直链时优先原样透传存储并返回客户端；上游仅给 data: 内嵌视频（如 Vertex base64）或无地址时，落代理URL。
_Avoid_: 视频地址、下载链接

**代理URL**:
网关下发的结果代理地址，形如 /v1/videos/{公开任务ID}/content；网关代客户端向源头取回流式返回，客户端无需源头直连权限。
_Avoid_: content 链接

### 渠道与上游

**渠道（channel）**:
网关接入某一具体上游的适配单元，边界由上游的协议、端点与鉴权决定，与模型厂商解耦。参数一致但上游不同，就是不同渠道（火山直连 = doubao 渠道，聚合重卖 = XFAI 渠道）。
_Avoid_: 上游、厂商（三者是不同概念，不可混用）

### 渠道配额与限额

**渠道原始成本（original cost）**:
渠道记账使用的上游原始消耗量，不含用户分组折扣（折扣前）。周期限额与硬上限都按它判定，与用户侧计费（折扣后 quota）解耦。
_Avoid_: 折扣后额度、用户侧额度

**周期限额（period quota limit）**:
渠道在一个周期（`period_hours`，默认 24h）内允许消耗的渠道原始成本上限；0 = 不限。周期由重置任务按 `period_reset_time` 滚动推进并清零已用。
_Avoid_: 周期额度、限流额度

**硬上限（hard quota limit）**:
渠道终身累计消耗的渠道原始成本上限；0 = 不限。达到后渠道不再被选中（不自动恢复，仅重置任务对周期项清零，硬上限不回）。
_Avoid_: 硬限额、累计上限

**兜底渠道（fallback channel）**:
限额为 0（不限）且优先级低于受限渠道的渠道；当受限渠道全部超限被过滤后，由它接管全部流量。
_Avoid_: 备用渠道、应急渠道

**音频生成（DoubaoAudio）**:
豆包语音·音频生成大模型（seed-audio-1.0）的同步 HTTP 接口（`POST /api/v3/tts/create`），一次请求返回完整音频，可生成配乐、音效、任意音色内容。区别于语音合成。
_Avoid_: TTS、语音合成

**语音合成（WS TTS）**:
火山引擎语音合成接口，通过 WebSocket 把文本流式合成为人声朗读音频。区别于音频生成。
_Avoid_: 音频生成、音频大模型

### 素材库

**素材库（material library）**:
网关内一个共享、单租户的素材资源能力，透明反向代理 ai-tokenhub 素材库 API（火山 ARK 兼容接口）。凭证（AK/SK）内置在全局 JSON 配置 `material_library` 中，下游客户端不持有，租户隐含在凭证中。
_Avoid_: 上游控制台、渠道（它不是渠道，不提供模型请求）

**素材组（material group）**:
按"真人主体/虚拟人像"划分的资源集合，上游称 seedance asset group。一个真人素材组只对应一个真人主体。
_Avoid_: 素材库（那是整体能力，组是其中的一个集合）

**真人素材组（LivenessFace group）**:
需真人通过火山 H5 刷脸认证才能启用的素材组；认证经 `CreateVisualValidateSession` 发起，回调由调用方自持，组 id 需上游开放后才能记录归属。
_Avoid_: 真人组、人脸组

**虚拟素材组（AIGC group）**:
同步创建即用、无需认证的素材组（`CreateAssetGroup`，`GroupType: AIGC`）。
_Avoid_: 虚拟组

**素材（material / asset）**:
素材组下的单个图片/视频/音频，生成任务以 `asset://<assetId>` 引用，经上游 `ListAssets` 按组查询。
_Avoid_: 文件、附件

**素材状态**:
Active（就绪）、Processing（处理中）、Failed（失败）。状态以上游为准，本地不镜像。
_Avoid_: 状态码

# 把 DeepSeek Search / Fetch 按成功调用次数对外售卖

DeepSeek 在 2026-10-19 开放了两个独立工具端点：`POST /v1/tools/search` 与 `POST /v1/tools/fetch`。两者用 Bearer 鉴权、按成功请求次数计费（上游：每 1,000 次 5 元），并且聊天链路里的服务端搜索（Anthropic `web_search_*` server tool、Responses `web_search`）也会在 2026-11-01 起按同样的口径收费。网关决定把这两种能力作为付费 API 售卖给下游，并统一聊天链路与独立端点的计费。

**Status:** accepted

## 背景

- 独立端点目前只能由持有 DeepSeek 官方 Key 的调用方直接访问；网关的下游客户没有 DeepSeek Key，只有网关 Key。
- 聊天链路的服务端搜索此前存在计费缺口：Claude 路径用全局 `web_search` 价（$10/1K），Responses 路径只识别 `web_search_preview`，DeepSeek 的 `web_search` 调用完全不扣费。
- 工具端点没有模型名，无法走现有 `分组 + 模型` 的渠道选择；但请求仍需复用已有 DeepSeek 渠道的 Key、优先级、权重与渠道限额。
- 上游计费规则明确：只有 HTTP 200 且「Search 的 `results` 非空 / Fetch 返回了网页正文」才收费；失败、无结果、超时都不收费。`count` 不影响价格。

## 决策

### 对外端点

新增四个路由，行为一致：

- `/tools/search`、`/tools/fetch`
- `/v1/tools/search`、`/v1/tools/fetch`

中间件为 `TokenAuth` + `ModelRequestRateLimit`，与普通 relay 共用用户/令牌额度与窗口限流；不经过 `Distribute`，因为请求里没有模型。

### 请求与响应

- 请求体原样透传到 `{渠道 base_url}/v1/tools/{search|fetch}`，只做必要校验：`query` 必填且不超过 128 字符、`count` 在 1–50、`url` 必填且为无凭证的 http/https 绝对地址。未知字段保留。
- 响应原样透传上游状态码、`Content-Type` 与 body。非 200 不做错误改写。
- 网关侧限制：请求体 1MB、响应体 10MB、上游超时 120 秒。网关不重试；上游 429 直接返回给调用方。

### 渠道选择

新增 `model.GetRandomSatisfiedChannelByType(group, channelType)`：只按「分组 + 渠道类型」选渠道，忽略模型维度，复用现有优先级、权重、渠道周期/硬限额过滤。`auto` 分组按用户可用分组依次尝试。工具端点固定选择 DeepSeek 渠道（type 43）。

### 计费

- 新增工具价 `deepseek_search` = `deepseek_fetch` = 5.0（美元/千次，1:1，不做汇率换算），管理员可在 `tool_price_setting.prices` 覆盖。
- 单次价格 = `price / 1000 × QuotaPerUnit × 分组倍率`。默认 5 美元/千次对应 2500 quota/次。
- 先预扣一次的价格；上游返回 200 且满足成功条件才结算，否则全额退款。网络错误、非 200、空结果都退款。
- 结算成功后更新用户已用额度/请求数、渠道原始成本，并写一条消耗流水；模型名使用伪模型 `deepseek-search` / `deepseek-fetch`。未计费但上游 200 的请求也写一条 quota=0 的流水，`other.billable=false`。
- 不重试意味着不会出现「用户被扣两次、上游只成功一次」的窗口。

### 聊天链路统一

- 工具价表新增 `web_search:deepseek-*` = 5.0 覆盖项，DeepSeek 模型的服务端搜索与独立端点同价；其他模型仍走 `web_search` 默认 $10/1K。
- Claude 路径改为 `GetToolPriceForModel("web_search", 模型名)`，保留 `usage.server_tool_use.web_search_requests` 作为次数来源。
- Responses 路径识别 `web_search` 工具类型：非流式统计输出 item 中 `web_search_call` 的数量，流式在 `response.output_item.done` 上计数；不再按「声明了工具」计一次。`web_search_preview` 的既有行为保持不变。

## 理由

- **按上游口径计费**：只有上游真正产生结果才向网关客户收费，避免网关在无结果/失败时替上游成本买单，也避免客户为无效搜索付费。
- **复用 DeepSeek 渠道而不是新建工具渠道**：Key、优先级、权重、限额都已在渠道里，新端点只是同一上游的另一组 URL；新建渠道会让运营重复配置并让限额分裂。
- **伪模型名**：日志与账单需要一个稳定的模型标识；复用真实模型名会让工具调用混进对话模型统计，也无法在日志里区分 Search 与 Fetch。
- **透传**：上游响应结构（`results`、`content`、错误码）是客户直接消费的契约，网关改写只会引入偏差；网关只在计费判定时读取字段。
- **统一价格**：DeepSeek 官方对独立端点与聊天服务端搜索收同一个价，网关若维持两个价，会出现「同一能力换个入口价格翻倍」的套利。

## 影响

- 工具端点上线即计费；上游自身从 2026-11-01 起对服务端搜索收费，届时网关收入与上游成本对齐。
- Responses 的 `web_search` 仅 `deepseek-v4-pro` 生效；`deepseek-flash` 上声明该工具不会产生 `web_search_call`，因此不会扣费。
- 一个 `web_search_call` output item 无论 action 是 `search` 还是 `open_page` 都记一次 Search 调用；两者上游同价，日志层面不再细分。
- 令牌开启「可用模型限制」时，工具端点按 DeepSeek 模型权限授权：白名单必须包含至少一个该分组下 DeepSeek 渠道提供的模型（如 `deepseek-chat`），或显式包含伪模型 `deepseek-search` / `deepseek-fetch`；否则返回 403。未开启模型限制的令牌不受影响。
- 工具端点按「分组 + 渠道类型」选路，不依赖渠道的模型列表；模型列表为空的 DeepSeek 渠道同样会被选中，但无法为受限令牌提供授权依据（受限令牌需显式配置伪模型）。
- 渠道级 `header_override` / `param_override` / 代理设置不会应用到工具端点；工具请求只带 `Authorization`、`Content-Type`、`Accept` 三个头。需要额外头或自定义代理的 DeepSeek 接入应直接改渠道 `base_url`，或后续显式扩展该适配。
- 响应体上限 10MB、上游超时 120 秒。超限或超时会按失败退款，但上游可能已经计费，这部分成本由网关承担。
- 现有 `tool_price_setting.prices` 若显式把新键写成 0，会覆盖默认价；运营需要确认配置。

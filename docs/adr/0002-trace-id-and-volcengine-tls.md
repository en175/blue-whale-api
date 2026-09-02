# 跨系统 trace_id 链路追踪 + 火山引擎 TLS 日志中心接入

本文档记录 Blue-Whale 多系统（blue-whale-api / blue-whale-pay / blue-whale-ai）的跨系统链路追踪（`X-Trace-Id`）设计，以及将日志接入火山引擎日志服务（Torch Log Service，TLS）的方案。

**Status:** proposed（D4 字段表已于 2026-08-26 被 supersede，见下）

> **D4 字段表已失效。**跨三仓统一日志字段的决策见
> `blue-whale-agent/docs/decisions/2026-08-26-cross-repo-log-schema-and-error-codes.md`。
> 该决策把字段名对齐 OpenTelemetry 语义约定（`container` → `service_name` + `service_instance_id`、
> `level` → `severity_text`、`message` → `body` 等），并统一了级别值域、时间格式与缺省值。
> **新代码不要按本文 D4 的字段表写。**D1/D2/D3/D5 仍然有效。

## 背景

Blue-Whale 已由单服务演化为多系统：

- **blue-whale-api**（Go，new-api 二开）：LLM 网关与计费，对外提供 OpenAI 兼容 API + 管理端。
- **blue-whale-pay**（Java）：支付体系（`blue_whale_pay_web` + `blue_whale_pay_server`，logback）。
- **blue-whale-ai**（TypeScript / pnpm workspace）：agent-worker、generation-worker 等 Node 服务 + `blue_whale_web` 前端。

排查问题时日志分散在多个系统，缺乏一个贯穿全链路的关联 ID，无法按一次业务操作串联各系统日志。同时已有需求将日志接入火山引擎 TLS 日志中心统一检索。

现状：仅 blue-whale-api 有单请求标识 `request_id`（`X-Oneapi-Request-Id`，见 `middleware/request-id.go`），不跨系统；其余系统无任何请求/链路标识。

## 决策

### D1：新增 `X-Trace-Id` 链路标识，与 `request_id` 并存

- 新增 HTTP header `X-Trace-Id`，语义为**跨系统链路标识**。
- 与现有 `request_id` 并存、职责分离：
  - `request_id`：单次网关请求标识，响应头继续返回客户端（API 契约不变）。
  - `trace_id`：跨系统链路标识，贯穿 blue-whale-ai → blue-whale-api → 上游/火山、以及支付回调等所有环节。
- 值格式沿用 `common.NewRequestId()`（时间戳 + 随机串，见 `common/utils.go:282`），保证全局唯一且可读。

### D2：透传优先，缺省生成（入口不固定）

最外层入口不固定（外部用户可直连网关 API / 管理端，也可经 blue-whale-ai，支付也有独立回调入口）。因此每个服务的 HTTP 入口统一执行：

1. 请求 header 有 `X-Trace-Id` → 沿用（透传）。
2. 无 → 本服务生成一个（同 `NewRequestId` 格式）。

同一链路透传不重生成，保证链路上 trace_id 唯一。改动对称：各系统各加一个入口过滤器 + 出站 header 透传。

### D3：接入火山引擎 TLS（SDK Producer 直写，方案 A）

- 采用应用内 **Go SDK Producer 异步批量写入**（`github.com/volcengine/volc-sdk-golang`），替代采集器 / docker logging driver 方案。
- 理由：日志量可控，改动集中于 `logger` 包；不引入额外部署组件；Producer 异步 + 失败静默降级，对请求链路零影响。
- 备选方案 B（filebeat/LogCollector 采集 stdout）与 C（docker logging driver）因引入组件 / 支持弱而放弃。

### D4：TLS 日志结构化（键值对 + 索引）—— 字段表已被 supersede

> 本节字段表保留作历史记录。现行字段表见
> `blue-whale-agent/docs/decisions/2026-08-26-cross-repo-log-schema-and-error-codes.md` D2。

- 所有经 `logger` 包的日志（业务日志 + 经 `TLSLogHook` 注入的系统日志 `SysLog`/`SysError`）发送时组装统一键值对（平铺下划线命名，跨系统 pay/ai 对齐）：
  `{level, time, container, env, version, request_id, trace_id, caller, func, message}`
  - `service_name`：常量 `blue-whale-api`（IKB29O 二轮）。曾经是 `CONTAINER_NAME` 优先、
    缺省回退主机名/容器名——三层兜底，配错时没有一层会红。服务名是代码的事实，不由部署填。
  - `deployment_environment_name`：`NODE_ENV`，缺省 `local`（三仓同一个变量名、同一个缺省）。
  - `service_instance_id` / `host_ip`：主机名-PID 与第一块非环回网卡，都由进程自己得出，没有变量。
  - `service_version`：`common.Version`，构建时注入。
  - `caller`（文件:行号）/`func`（模块内函数名）：`runtime.Caller`/`FuncForPC` 惰性计算，仅 TLS 启用时产生开销。
  - 系统日志无请求上下文：`request_id=SYSTEM`、`trace_id` 为空。
  - gin 访问日志不进 TLS（控制量）；`pb.Log.Time` 使用毫秒时间戳（与 SDK 内部兜底语义对齐）。
- TLS 索引：
  - `level`、`container`、`env`、`version`、`request_id`、`trace_id`、`caller`、`func` → 关键字（keyword，不开分词）。
  - `message` → 文本（text，开分词 + 中文分词）。message 是自由文本，非 JSON，故用 text 而非 json。

### D5：控制台配置（一次性）

- 创建 Project（如 `blue_whale_dev`）→ 创建 Topic（如 `blue-whale-dev-30`）→ 启用索引（全文 + 键值）→ 添加键值字段 `level` / `request_id` / `trace_id` / `message`。
- 创建 AccessKey（IAM 用户），赋 TLS 写权限。
- 记录 Endpoint + TopicID。
- 应用侧环境变量：`TLS_ENDPOINT` / `TLS_AK` / `TLS_SK` / `TLS_TOPIC_ID`（四项齐备才启用）；可选 `TLS_REGION`（缺省 cn-beijing，其他区域必须显式配置）。服务标识不再是变量，见上。

## 各系统改动清单

### blue-whale-api（Go）

| # | 文件 | 改动 |
|---|---|---|
| 1 | `common/constants.go` | 新增 `TraceIdKey = "X-Trace-Id"` |
| 2 | `middleware/request-id.go` | 升级为「透传优先，缺省生成」，写入 context + 响应头 |
| 3 | `logger/logger.go` `logHelper` | 日志行追加 `trace_id` |
| 4 | 出站 HTTP 请求（`relay/channel/api_request.go`、`service/http.go`） | 请求头加 `X-Trace-Id` |
| 5 | `service/http.go` `ShouldCopyUpstreamHeader` | 捕获上游返回的 `X-Trace-Id` |
| 6 | `logger/tls_writer.go`（新增） | TLS Producer 封装 + 失败降级 |
| 7 | `main.go` | 初始化 TLS writer（读环境变量） |
| 8 | 部署 compose | 加 `TLS_ENDPOINT` / `TLS_AK` / `TLS_SK` / `TLS_TOPIC_ID` 环境变量 |

### blue-whale-pay（Java）

| # | 文件 | 改动 |
|---|---|---|
| 1 | 新增 `TraceIdFilter` | HTTP 入口：透传优先、缺省生成，写入 MDC `trace_id` |
| 2 | `logback-spring.xml`（各 module） | pattern 加 `[%X{trace_id}]` |
| 3 | 出站 HTTP 调用封装 | 从 MDC 读 `trace_id` 设 header |

### blue-whale-ai（TypeScript）

| # | 文件 | 改动 |
|---|---|---|
| 1 | `packages/core` 新增 `trace.ts` | `getOrCreateTraceId()` 工具 |
| 2 | 各 app 入口 | 解析 `X-Trace-Id` 注入请求上下文 |
| 3 | `packages/core/src/new-api-client.ts` `fetchImpl` | 出站请求头加 `X-Trace-Id` |
| 4 | 各 worker 日志 | 追加 trace_id 字段 |

## 实施顺序

1. **blue-whale-api 先行**：链路必经点，改动独立可测。
2. blue-whale-pay、blue-whale-ai 随后。
3. TLS 控制台索引补 `trace_id` 字段。
4. 验证：各系统发一条日志，TLS 按 `trace_id` 检索整条链。

## 影响与权衡

- **API 契约**：`request_id` 响应头不变；新增 `X-Trace-Id` 请求/响应头，向后兼容（可选）。
- **性能**：TLS 写入走异步 Producer，不影响请求链路；失败静默降级，不影响主服务日志。
- **安全**：AccessKey 走环境变量，不落代码。
- **检索**：索引只对后续新写入日志生效（TLS 规则：索引修改不回溯旧数据）。

## 决策补记：存量 SysLog/SysError 不做批量改造（2026-08-07，IK7309）

**背景**：接入 TLS 后发现全项目 349 处 `common.SysLog/SysError` 调用中，128 处（37%，其中 SysError 39 处）位于带请求上下文的函数内。这些调用签名无 ctx，日志进 TLS 后 `request_id=SYSTEM`、`trace_id` 为空，无法按链路检索。根因是上游 new-api 在请求路径混用不带 ctx 的 SysLog（历史债务），叠加 Go 无 goroutine-local（MDC）、上下文只能显式传递的语言约束。

**评估**：修复需新增 `SysLogCtx/SysErrorCtx` + hook 升级 + go/ast 精确替换 128 处（参数名/类型不统一，普通 sed 不可行）。行为风险极小（日志本已进 TLS，仅补字段），成本为大量机械改动。

**决策：不修**。理由：
1. 核心链路（relay 转发、支付下单）已有完整 trace，排障主场景已覆盖；
2. 这 128 处集中在管理端/渠道适配层的零散日志，非排障关键路径；
3. 改动量与收益不成比例。

**配套约定**（已写入 AGENTS.md Backend Rules / Logging）：
- 新增代码一律使用 `logger.Log*(ctx, ...)`；禁止在带请求上下文的函数里调用 `SysLog/SysError`；
- `SysLog/SysError` 仅限无请求上下文的系统级场景；
- 存量债务按需单点改（排障确实需要某处 trace_id 时，提独立小任务）。

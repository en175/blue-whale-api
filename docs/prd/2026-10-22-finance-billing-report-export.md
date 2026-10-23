# V待确认【财务报表】管理员账单核对与 Excel/CSV 导出

| 项目 | 内容 |
|---|---|
| 文档类型 | 标准 PRD（涉及计费、资金、前后端、数据库迁移） |
| 状态 | 待评审 |
| 版本 | v0.2 |
| 日期 | 2026-10-22 |
| 产品负责人 | 待指定 |
| 研发负责人 | 待指定 |
| 测试负责人 | 待指定 |
| 目标产品版本 | **待确认** |
| 关联业务父需求 | IKPZBH（Gitee） |
| 关联开发子任务 | IKPZBS（后端）、IKPZBV（前端）；测试任务由测试按 SOP 创建 |
| 影响范围 | `blue-whale-api` 后端、`web/default` 前端、`quota_adjustments` 新表、`top_ups` 加列 |

> 评审前置：本文档需产品确认目标版本后，才可进入业务代码修改。PRD 未确认前只做只读调研。

---

## 1. 背景与目标

### 1.1 现状

管理员目前只能在使用日志页逐条查看记录，无法按“用户 + 时间 + 模型”直接得到财务口径的汇总，也没有任何导出能力。财务核对时需要手工拼接多处数据：

| 数据 | 当前存放 | 当前问题 |
|---|---|---|
| 模型消耗 | `logs`，`type=2` | 只能逐条看，没有按用户/模型汇总 |
| 退款 | `logs`，`type=6` | 与消耗混在同一列表，无法直接算净消耗 |
| 支付充值 | `top_ups` 表 | 日志表 `type=1` 的 `quota=0`，金额只在文本里，无法直接汇总 |
| 管理员手动调账 | `logs`，`type=3` 审计日志 | 日志归属**操作者**，目标用户在 `other.op.params.target_user_id`；金额是展示字符串（如 `¥0.100000 额度`），不是可计算数值 |

### 1.2 目标

1. 新增管理员专用「财务报表」页面，供财务集中查看、筛选、核对和导出。
2. 支持按**用户、时间范围、模型**筛选。
3. 提供“消耗、退款、支付充值、管理员手动调账”四类流水的明细和汇总。
4. 支持导出 Excel（多 Sheet）和 CSV（按页签单表）。
5. 导出数据可追溯：明确数据来源、统计口径、精确/估算标记和导出条件。
6. 不改变现有计费规则和用户侧账务结果；本次只增加报表读取能力和必要的账务埋点。

### 1.3 非目标

- **不做余额对账**：本报表只覆盖四类流水，不包含新用户赠送、邀请赠送、签到、兑换码、订阅、活动等入账，因此 `充值 + 调账 - 净消耗` 不等于余额变化。页面和导出必须明确标注。
- 不做完整财务系统：不做发票、合同、银行流水对账、审批流。
- 不改变任何计费、退款、充值、扣费的计算公式。
- 不补做历史账务订正；历史缺口只做标注，不伪造精确值。
- 不提供历史每日余额快照和期初/期末余额对账。
- 不做用户自助财务报表。

---

## 2. 已确认的产品决策

| # | 决策 | 结论 |
|---|---|---|
| D1 | 入口 | 独立「财务报表」页，面向财务集中核对；不绑定用户列表行操作 |
| D2 | “管理员充值”口径 | 仅管理员手动**增加 / 减少 / 覆盖**用户额度；不包含管理员补单 |
| D3 | 管理员调账事件模型 | 独立 `quota_adjustments` 表；不写入 `logs.type=1`，不破坏“方向由 type 表达” |
| D4 | 导出结构 | Excel 多 Sheet：说明、用量汇总、资金汇总、消耗、退款、充值、管理员调整 |
| D5 | 报表边界 | 只做四类流水，不做余额对账；其他入账（赠送/签到/兑换码/订阅）不包含 |
| D6 | 历史数据 | 双合计：精确合计与估算合计分列；估算永不并入精确合计 |
| D7 | 权限 | 管理员及以上（role >= 10） |
| D8 | 时区 | 页面和导出统一 UTC+8 |
| D9 | 充值归期 | 按 `complete_time`；只统计 `status=success` |
| D10 | 用户筛选 | 单选用户 + 全部用户 |
| D11 | CSV 形态 | 按页签单表导出；Excel 负责多 Sheet |
| D12 | 导出上限 | 366 天 / 单 Sheet 10 万行 / 单次总计 30 万行 / 5 分钟超时 |
| D13 | Excel 实现 | 后端 `github.com/xuri/excelize/v2` |
| D14 | 金额列 | 原始 quota + 按导出时展示设置换算的金额 |
| D15 | 数据库改动 | 接受：新建 `quota_adjustments`，`top_ups` 增加 `quota_credited`；发版时 GORM AutoMigrate 自动迁移 |

---

## 3. 前置条件

| # | 前置条件 | 状态 | 缺失影响 | 补齐人 |
|---|---|---|---|---|
| P1 | 目标产品版本 | **待确认** | 无法确定提测和发布窗口 | 产品 |
| P2 | 允许新增 Go 依赖 `github.com/xuri/excelize/v2` | 已确认 | — | 研发负责人 |
| P3 | 允许新建 `quota_adjustments`、`top_ups` 加 `quota_credited` | 已确认 | — | 产品 / 研发 |
| P4 | 历史数据采用“双合计”口径 | 已确认 | — | 产品 / 财务 |
| P5 | 财务人员使用哪个账号 | 待确认 | 当前用管理员账号；如需独立财务角色另开需求 | 产品 / 运维 |
| P6 | 日志保留策略 | 待确认 | 日志被清理的历史区间无法核对，导出说明需提示 | 运维 / 产品 |

---

## 4. 用户与权限

| 角色 | 权限 |
|---|---|
| 超级管理员（role=100） | 查看、筛选、导出全部用户数据 |
| 管理员（role=10） | 查看、筛选、导出全部用户数据 |
| 普通用户（role=1） | 无入口；接口返回 403 |
| 财务人员 | 使用管理员账号；独立财务角色不在本次范围 |

规则：

- 前端侧边栏「财务报表」仅管理员可见。
- 后端所有报表与导出接口使用 `middleware.AdminAuth()`。
- 每次导出成功写一条管理审计日志：`action=billing_report.export`，记录筛选条件、格式、各 Sheet 行数、导出人、耗时。
- 导出文件不包含令牌 key、渠道 key、用户密码、IP 等敏感字段。

---

## 5. 页面与交互

### 5.1 入口

- 路由：`/billing-report`
- 侧边栏：`Admin` 分组新增「财务报表」/ `Billing Report`，图标使用 `FileSpreadsheet` 或 `ReceiptText`。
- 页面标题：`财务报表` / `Billing Report`。

### 5.2 页面结构

```
┌──────────────────────────────────────────────────────────────────────────┐
│ 财务报表                                                                  │
│ ┌──────────────┬──────────────────┬──────────────┬──────────────────────┐ │
│ │ 用户 [全部 ▾] │ 时间 [近30天 ▾]   │ 模型 [输入…]  │ [查询] [重置]         │ │
│ └──────────────┴──────────────────┴──────────────┴──────────────────────┘ │
│ ┌──────────┬──────────┬──────────┬────────────────────┬──────────────────┐ │
│ │ 总消耗    │ 总退款    │ 净消耗    │ 支付充值(实付/到账)  │ 管理员调账净额     │ │
│ │ q / ¥    │ q / ¥    │ q / ¥    │ ¥x / q(精确+估算)   │ q(精确+估算)      │ │
│ └──────────┴──────────┴──────────┴────────────────────┴──────────────────┘ │
│ ┌──────────────────────────────────────────────────────────────────────┐ │
│ │ [用量汇总] [资金汇总] [消耗明细] [退款明细] [充值明细] [管理员调整]      │ │
│ │                                                    [导出 Excel] [导出 CSV] │
│ ├──────────────────────────────────────────────────────────────────────┤ │
│ │  表格（服务端分页）                                                     │ │
│ └──────────────────────────────────────────────────────────────────────┘ │
│  口径说明：本报表不是余额对账表，不含赠送/签到/兑换码/订阅等其他入账。        │
└──────────────────────────────────────────────────────────────────────────┘
```

### 5.3 筛选栏

| 筛选项 | 控件 | 默认值 | 规则 |
|---|---|---|---|
| 用户 | 可搜索下拉（用户名 / ID / 显示名） | 全部用户 | 单选；按 `user_id` 精确过滤 |
| 时间范围 | 日期时间范围选择器 | 近 30 天（含今天） | 必填；开始 <= 结束；最大跨度 366 天 |
| 模型 | 文本输入 | 全部模型 | 精确匹配；支持 `%` 通配，规则与现有日志筛选一致 |
| 查询 / 重置 | 按钮 | — | 查询写入 URL search params；重置恢复默认值 |

### 5.4 六个页签

| 页签 | 内容 | 分页 |
|---|---|---|
| 用量汇总 | 用户 × 模型维度的消耗、退款、净消耗、请求次数、tokens | 前端分页（汇总行数通常较少） |
| 资金汇总 | 用户维度的支付充值、实付金额、管理员调账净额、净消耗、当前余额 | 前端分页 |
| 消耗明细 | 每条 `type=2` 消耗日志 | 服务端分页 |
| 退款明细 | 每条 `type=6` 退款日志 | 服务端分页 |
| 充值明细 | 每条 `top_ups.status=success` 订单，按 `complete_time` | 服务端分页 |
| 管理员调整 | 新表 `quota_adjustments` + 历史 `type=3` 解析结果 | 服务端分页 |

### 5.5 导出交互

- 主按钮「导出 Excel」：导出当前筛选条件下的全部 6 个数据 Sheet，另加 1 个「导出说明」Sheet。
- 下拉「导出 CSV」：导出当前页签对应的单个 CSV。
- 导出期间按钮显示 loading；同一管理员同时只允许 1 个导出任务，重复请求返回 429。
- 导出完成前不关闭弹窗/不跳转，失败给出明确错误信息。
- 导出成功后 toast 提示文件名和行数。

### 5.6 空态、加载与错误

- 无数据：表格显示空态；汇总卡片显示 0；导出仍生成只含表头的文件。
- 加载：筛选栏和表格显示 skeleton；导出按钮 loading。
- 参数错误：toast 提示具体原因（如“开始时间不能晚于结束时间”“时间范围不能超过 366 天”）。
- 导出超限：toast 提示缩小时间范围或增加用户/模型条件。
- 历史数据提示：在「充值明细」「管理员调整」「资金汇总」顶部显示黄色提示条：
  “含 N 条历史记录金额无法精确重建，已标记「估算/不可解析」，不计入精确合计。”

---

## 6. 数据口径（关键规则）

### 6.1 四类流水

| 类别 | 数据源 | 过滤条件 | 精确度 |
|---|---|---|---|
| 消耗 | `logs` | `type=2` | 精确 |
| 退款 | `logs` | `type=6` | 精确 |
| 支付充值 | `top_ups` | `status=success`，按 `complete_time` | 实付金额精确；到账额度新数据精确、历史估算 |
| 管理员调账 | `quota_adjustments` + 历史 `logs.type=3` | 按目标用户和时间 | 新数据精确；历史估算/不可解析 |

**不包含**：新用户赠送、邀请赠送、签到、兑换码、订阅、活动、余额转移等其他入账。因此本报表**不是余额对账表**。

### 6.2 时间口径

| 数据 | 时间字段 | 过滤方式 | 导出显示 |
|---|---|---|---|
| 消耗 | `logs.created_at` | `>= start AND <= end` | `时间(UTC+8)` |
| 退款 | `logs.created_at` | 同上 | `时间(UTC+8)` |
| 支付充值 | `top_ups.complete_time`；为 0 时回退 `create_time` | 仅 `status=success` | `完成时间(UTC+8)` |
| 管理员调账（新） | `quota_adjustments.created_at` | 同上 | `时间(UTC+8)` |
| 管理员调账（历史） | `logs.created_at`（`type=3`） | 仅解析新表起始时间之前 | `时间(UTC+8)` |

- 数据库存储 Unix 秒；页面和导出统一按 `Asia/Shanghai (UTC+8)` 展示。
- 充值订单创建但未完成的，不计入充值明细和汇总。

### 6.3 用户口径

- 以 `user_id` 为准；用户名、显示名仅用于展示和搜索。
- 用户被删除后，历史 `logs.username` 仍保留；导出显示日志中的用户名，并在用户 ID 后标注“已删除”（如能查到删除状态）。
- 管理员调账历史日志归属操作者，必须按 `other.op.params.target_user_id` 归到目标用户；若目标就是操作者本人（参数缺省），使用日志的 `user_id`。

### 6.4 模型口径

- 消耗和退款按 `logs.model_name` 分组；为空时归入 `(未标注模型)`。
- 支付充值和管理员调账没有模型维度，只在资金汇总和明细中展示。
- 退款可能发生在消耗之后的其他时间窗口，因此**单个时间窗口内净消耗可能为负数**。不截断、不修正，如实展示。

### 6.5 指标定义

| 指标 | 公式 / 来源 | 说明 |
|---|---|---|
| 消耗额度 | `SUM(logs.quota) WHERE type=2` | 原始 quota |
| 退款额度 | `SUM(logs.quota) WHERE type=6` | 退款日志的 quota 为正数 |
| 净消耗额度 | 消耗额度 - 退款额度 | 可为负 |
| 请求次数 | `COUNT(*) WHERE type=2` | 消耗日志条数 |
| 输入 tokens | `SUM(prompt_tokens) WHERE type=2` | — |
| 输出 tokens | `SUM(completion_tokens) WHERE type=2` | — |
| 消耗金额 / 退款金额 | 额度按导出时展示设置换算 | 仅参考，原始 quota 为准 |
| 支付充值到账额度 | 新订单 `top_ups.quota_credited`；历史按规则估算 | 分精确/估算两列 |
| 实付金额 | `SUM(top_ups.money)` | 法币实付，精确 |
| 管理员调账净额 | `quota_adjustments.quota_delta` 合计；历史解析 | 加为正、减为负、覆盖为 after-before；分精确/估算两列 |
| 当前余额 | `users.quota` | 当前值，**不是**截止筛选结束时间的余额 |

### 6.6 额度与金额换算

- 原始 `quota` 是账务主单位；金额列是辅助展示。
- 换算规则使用导出时点的当前配置：`QuotaDisplayType`、`USDExchangeRate`、`CustomCurrencyExchangeRate`、`QuotaPerUnit`。
- 「导出说明」Sheet 必须写明本次导出使用的 `QuotaPerUnit`、展示类型和汇率。
- 历史记录按当前配置换算可能产生偏差，需在说明中标注“金额按导出时配置换算”。

---

## 7. 字段清单

### 7.1 用量汇总（用户 × 模型）

| 列 | 类型 | 说明 |
|---|---|---|
| 用户ID | number | `user_id` |
| 用户名 | text | `username` |
| 模型 | text | `model_name`，空为 `(未标注模型)` |
| 请求次数 | number | `COUNT(type=2)` |
| 输入tokens | number | — |
| 输出tokens | number | — |
| 消耗额度 | number | quota |
| 退款额度 | number | quota |
| 净消耗额度 | number | 消耗 - 退款 |
| 消耗金额 | number | 当前展示设置换算 |
| 退款金额 | number | 当前展示设置换算 |
| 净消耗金额 | number | 当前展示设置换算 |

最后一行：`总计`，对各数值列求和。

### 7.2 资金汇总（用户）

| 列 | 类型 | 说明 |
|---|---|---|
| 用户ID | number | — |
| 用户名 | text | — |
| 支付充值到账额度-精确 | number | `quota_credited` 精确合计 |
| 支付充值到账额度-估算 | number | 历史订单按规则估算 |
| 支付充值估算条数 | number | — |
| 支付实付金额 | number | `SUM(money)`，精确 |
| 支付订单数 | number | 仅 success |
| 管理员调账净额-精确 | number | 新表精确合计 |
| 管理员调账净额-估算 | number | 历史日志解析 |
| 管理员调账不可解析条数 | number | 不计入任何合计 |
| 净消耗额度 | number | 消耗 - 退款 |
| 当前余额 | number | 参考值，非期末余额 |
| 数据提示 | text | 如“含 3 条历史估算记录”“含 1 条不可解析记录” |

最后一行：`总计`，精确列和估算列分别求和。

### 7.3 消耗明细

| 列 | 类型 | 说明 |
|---|---|---|
| 时间(UTC+8) | datetime | `created_at` |
| 用户ID | number | — |
| 用户名 | text | — |
| 模型 | text | — |
| 令牌名称 | text | `token_name` |
| 分组 | text | `group` |
| 渠道ID | number | `channel_id` |
| 渠道名称 | text | 关联渠道名；渠道删除则留空 |
| 请求ID | text | `request_id` |
| 输入tokens | number | — |
| 输出tokens | number | — |
| 总tokens | number | 输入 + 输出 |
| 消耗额度 | number | quota |
| 消耗金额 | number | 当前展示设置换算 |
| 是否流式 | yes/no | `is_stream` |

### 7.4 退款明细

| 列 | 类型 | 说明 |
|---|---|---|
| 时间(UTC+8) | datetime | — |
| 用户ID | number | — |
| 用户名 | text | — |
| 模型 | text | — |
| 令牌名称 | text | — |
| 分组 | text | — |
| 渠道ID | number | — |
| 退款额度 | number | quota |
| 退款金额 | number | 换算 |
| 退款原因 | text | `other.reason` |
| 关联任务ID | text | `other.task_id` |
| 内容 | text | `content` |

### 7.5 充值明细

| 列 | 类型 | 说明 |
|---|---|---|
| 完成时间(UTC+8) | datetime | `complete_time` |
| 创建时间(UTC+8) | datetime | `create_time` |
| 用户ID | number | — |
| 用户名 | text | — |
| 订单号 | text | `trade_no` |
| 支付方式 | text | `payment_method` |
| 支付网关 | text | `payment_provider` |
| 实付金额 | number | `money`，精确 |
| 订单额度(原始) | number | `amount`，按订单流含义保留原值 |
| 到账额度 | number | `quota_credited`；历史按规则估算 |
| 到账来源 | text | `精确` / `估算` / `不可用` |
| 状态 | text | 仅导出 `success` |

### 7.6 管理员调整明细

| 列 | 类型 | 说明 |
|---|---|---|
| 时间(UTC+8) | datetime | — |
| 目标用户ID | number | — |
| 目标用户名 | text | — |
| 操作人ID | number | 新表 `operator_id`；历史从 `admin_info` 取 |
| 操作人用户名 | text | 同上 |
| 调整类型 | text | 增加 / 减少 / 覆盖 |
| 调整额度 | number | 有符号；覆盖为 after-before |
| 调整前额度 | number | 新表精确；历史可能为空 |
| 调整后额度 | number | 新表精确；历史可能为空 |
| 调整金额 | number | 按当前展示设置换算 |
| 数据来源 | text | `精确` / `估算` / `不可解析` |
| 备注/内容 | text | `note` 或历史 `content` |

---

## 8. 导出文件

### 8.1 Excel（`.xlsx`）

文件名：`billing-report_{YYYY-MM-DD}_{YYYY-MM-DD}.xlsx`

Sheet 顺序：

1. `导出说明`
2. `用量汇总`
3. `资金汇总`
4. `消耗明细`
5. `退款明细`
6. `充值明细`
7. `管理员调整`

`导出说明` 内容：

| 项 | 内容 |
|---|---|
| 导出时间 | `YYYY-MM-DD HH:mm:ss (UTC+8)` |
| 导出人 | 用户名 + 用户 ID |
| 筛选-用户 | 全部 / 用户名(ID) |
| 筛选-时间 | 开始 ~ 结束 |
| 筛选-模型 | 全部 / 模型名 |
| 额度单位 | 原始 quota |
| QuotaPerUnit | 当前值 |
| 金额换算 | 展示类型 + 汇率（如 1 USD = 7.2 CNY） |
| 消耗/退款口径 | `logs.type=2/6` |
| 充值口径 | `top_ups.status=success`，按 `complete_time` |
| 管理员调账口径 | 仅手动加/减/覆盖；不含补单；新表精确、历史解析 |
| 报表边界 | 不是余额对账表，不含赠送/签到/兑换码/订阅等其他入账 |
| 历史数据说明 | 估算/不可解析条数与原因 |
| 行数统计 | 各 Sheet 行数 |

格式要求：

- 首行加粗、冻结首行、开启自动筛选。
- 时间列格式 `yyyy-mm-dd hh:mm:ss`。
- 额度、金额、tokens 使用数值类型，不使用带单位字符串，确保 Excel 可求和。
- 金额列保留 6 位小数；quota 列为整数。
- 空值导出为空单元格，不写 `null` / `-`。
- 单 Sheet 超过导出行数上限前拦截，不生成截断文件。

### 8.2 CSV

文件名：`billing-report_{sheet}_{YYYY-MM-DD}_{YYYY-MM-DD}.csv`

- 单页签导出，字段与对应 Sheet 一致。
- UTF-8 with BOM，保证 Excel 直接打开中文不乱码。
- 使用 `\r\n` 换行。
- 数值列不加单位。
- 文本单元格做 CSV 公式注入防护：以 `=`, `+`, `-`, `@`, tab, CR 开头的**文本**单元格前置单引号；数值列不处理，避免破坏负数。
- CSV 不含「导出说明」Sheet；页面上的口径说明作为唯一说明来源。

### 8.3 导出限制

| 限制 | 值 | 处理 |
|---|---|---|
| 最大时间跨度 | 366 天 | 超过返回 400 |
| 单明细 Sheet 最大行数 | 100,000 | 超过返回 400，提示缩小范围 |
| 单次导出总行数 | 300,000 | 超过返回 400 |
| 单次导出超时 | 5 分钟 | 超时返回 500，不生成部分文件 |
| 并发导出 | 同一管理员 1 个 | 重复请求返回 429 |

---

## 9. 接口设计

所有接口挂在 `/api/billing-report` 下，统一 `middleware.AdminAuth()`。

### 9.1 汇总

`GET /api/billing-report/summary`

Query 参数：

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `user_id` | int | 否 | 精确用户 |
| `username` | string | 否 | 用户名（与 `user_id` 同时传时取交集） |
| `start_timestamp` | int64 | 是 | 秒级 |
| `end_timestamp` | int64 | 是 | 秒级 |
| `model_name` | string | 否 | 精确或 `%` 通配 |

返回：

```json
{
  "success": true,
  "data": {
    "usage": [{ "user_id": 1, "username": "u1", "model_name": "m1", "request_count": 10, "prompt_tokens": 100, "completion_tokens": 200, "consume_quota": 1000, "refund_quota": 100, "net_quota": 900 }],
    "funds": [{ "user_id": 1, "username": "u1", "topup_quota_exact": 5000, "topup_quota_estimated": 0, "topup_money": 10, "topup_count": 2, "admin_adjust_quota_exact": 100, "admin_adjust_quota_estimated": 0, "admin_adjust_unparsed_count": 0, "net_consume_quota": 900, "current_quota": 4200 }],
    "totals": { "consume_quota": 0, "refund_quota": 0, "net_quota": 0, "topup_quota_exact": 0, "topup_quota_estimated": 0, "topup_money": 0, "admin_adjust_quota_exact": 0, "admin_adjust_quota_estimated": 0 },
    "warnings": [{ "code": "historical_estimate", "count": 1 }]
  }
}
```

### 9.2 明细

```
GET /api/billing-report/consume
GET /api/billing-report/refund
GET /api/billing-report/topup
GET /api/billing-report/adjustment
```

公共 Query 参数：`user_id`, `username`, `start_timestamp`, `end_timestamp`, `model_name`, `p`, `page_size`。

返回标准分页结构：

```json
{ "success": true, "data": { "page": 1, "page_size": 50, "total": 123, "items": [] } }
```

### 9.3 导出

`GET /api/billing-report/export`

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `format` | `xlsx` / `csv` | 是 | — |
| `sheet` | string | CSV 必填 | `consume` / `refund` / `topup` / `adjustment` |
| 其余 | — | — | 同汇总接口 |

响应：

- `xlsx`：`Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`
- `csv`：`Content-Type: text/csv; charset=utf-8`
- `Content-Disposition: attachment; filename*=UTF-8''...`

### 9.4 错误码

| HTTP | 场景 |
|---|---|
| 400 | 时间缺失、开始 > 结束、跨度超限、参数非法、行数超限 |
| 403 | 非管理员 |
| 429 | 同一管理员已有导出任务 |
| 500 | 数据库/文件生成失败、导出超时 |

错误返回沿用 `common.ApiErrorMsg` / i18n，中文、英文都要有。

---

## 10. 数据模型与写入变更

### 10.1 新表 `quota_adjustments`

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | bigint PK | 自增 |
| `user_id` | bigint | 目标用户，索引 |
| `operator_id` | bigint | 操作人，索引 |
| `action` | varchar(20) | `add` / `subtract` / `override` |
| `quota_delta` | bigint | 有符号调整额；覆盖为 after-before |
| `quota_before` | bigint | 调整前余额 |
| `quota_after` | bigint | 调整后余额 |
| `note` | varchar(255) | 备注，可为空 |
| `created_at` | bigint | 秒级时间戳 |

索引：

- `idx_quota_adjustments_user_created (user_id, created_at)`
- `idx_quota_adjustments_created (created_at)`

写入规则：

- 在 `ManageUser` 的 `add_quota` 分支内，与用户余额更新放在**同一事务**。
- 事务内对用户行加 `lockForUpdate`，读取 `quota_before`，执行加/减/覆盖，写入 `quota_after` 和 `quota_delta`，插入调账行，提交。
- 事务提交后再写 `type=3` 操作审计日志；日志库可能独立，无法与主库同事务，失败只记录系统错误，不回滚调账。
- 调账行是财务账本，优先于审计日志。

### 10.2 `top_ups` 增加 `quota_credited`

| 列 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `quota_credited` | bigint | 0 | 订单完成时实际加到用户账上的 quota |

- 所有完成路径写入：Stripe、Creem、Waffo、Waffo Pancake、BlueWhalePay 钱包流、BlueWhalePay 档位流、Epay、管理员补单。
- 迁移需同时支持 SQLite / MySQL / PostgreSQL；GORM AutoMigrate 自动 `ADD COLUMN`。
- 建议同时增加索引 `idx_topups_status_complete_time (status, complete_time)`，服务报表按完成时间查询；若生产评估 `top_ups` 很小可省略。
- 新记录 `quota_credited > 0` 视为精确。
- 历史记录 `quota_credited = 0`，按以下规则估算并标记：

| 历史订单特征 | 到账额度估算 | 来源标记 |
|---|---|---|
| `trade_no` 以 `RCH` 开头（档位流，raw tokens） | `amount` | 精确（不依赖 QuotaPerUnit） |
| `payment_provider=creem` | `amount` | 精确 |
| `payment_provider=stripe` | `money * 当前 QuotaPerUnit` | 估算 |
| 其他（waffo / pancake / epay / bluewhalepay 钱包流） | `amount * 当前 QuotaPerUnit` | 估算 |

### 10.3 历史管理员调账解析

- 新表上线后，调账记录只从 `quota_adjustments` 读取。
- 历史记录从 `logs.type=3` 解析，且仅解析 `created_at < 新表首行 created_at` 的记录，避免与新表重复计数。
  - 若新表为空，则解析全部历史 `type=3`。
  - 新代码同时写新表和 `type=3` 审计日志；以首行时间作为分界，不会双计。
- 解析字段：
  - `other.op.action`：`user.quota_add` / `user.quota_subtract` / `user.quota_override`。
  - 目标用户：`other.op.params.target_user_id`；缺省时取日志 `user_id`（管理员改自己）。
  - 操作人：`other.admin_info.admin_id` / `admin_username`；缺省时取日志 `user_id` / `username`。
  - 金额：`add/subtract` 解析 `params.quota`；`override` 解析 `params.from` 和 `params.to`，差额 = to - from。
- 展示字符串格式：`＄x 额度`、`¥x 额度`、`自定义符号x 额度`、`N 点额度`。
  - `点额度` 视为精确。
  - 货币格式按当前汇率换算，标记“估算”。
  - 解析失败标记“不可解析”，不计入任何合计。
- 历史解析在 Go 内完成，不做跨数据库 JSON SQL，避免 MySQL/PostgreSQL/SQLite/ClickHouse 方言差异和非法 JSON 报错。
- `type=3` 记录量小，按时间范围拉取并加行数上限（如 5 万条），超过则要求缩小范围。

### 10.4 `logs` 其他注意事项

- 日志库可能配置为 ClickHouse；报表查询统一走 `LOG_DB`，订单/调账查询走主库 `DB`。
- 不新增 `logs` 列；现有 `(user_id,id)`、`(created_at,type)`、`model_name`、`username` 索引满足本次主查询。
- 不修改现有日志清理任务；被清理的历史日志不会出现在报表中，需在「导出说明」提示“受日志保留策略影响”。

### 10.5 迁移与上线

- 本项目无独立 migration 文件；GORM `AutoMigrate` 在 master 节点启动时执行。
- 发版流程：改 struct → `/deploy-blue-whale-api` 构建推送 → 容器重启 → AutoMigrate 自动建表/加列。
- 回滚：`restart.sh` 备份 `new-api.prev`；回滚二进制即可。新增表和列保留，旧代码忽略，不破坏数据。
- 新列对历史行默认 0；AutoMigrate 不做数据回填。
- 大表风险：本次只改 `top_ups`（表小）和新建空表，不改 `logs`；风险可控。

---

## 11. 异常与边界

| 场景 | 预期 |
|---|---|
| 开始时间晚于结束时间 | 400，提示“开始时间不能晚于结束时间” |
| 时间范围超过 366 天 | 400，提示最大跨度 |
| 用户不存在 | 400，提示“用户不存在” |
| 模型无匹配 | 正常返回空结果和空文件 |
| 无任何数据 | 汇总为 0；导出生成只含表头的文件 |
| 历史管理员调账金额不可解析 | 明细行保留，来源=`不可解析`，不计入总计，页面/文件显示提示 |
| 历史充值无法精确还原 | 明细行保留，来源=`估算`，计入估算合计，不计入精确合计 |
| 净消耗为负 | 如实展示负数，不截断 |
| 退款日志 model_name 为空 | 归入 `(未标注模型)` |
| 并发完成充值回调 | 只统计 `status=success` 的订单，按订单 ID 去重 |
| 历史成功订单 `complete_time=0` | 回退 `create_time` 归期，并在“到账来源”旁标注“完成时间缺失” |
| `QuotaPerUnit <= 0` | 金额列留空，精确性不受影响；页面和导出说明标注“金额换算不可用” |
| 调账事务失败 | 余额和调账行一起回滚，不产生半条记录 |
| 调账成功但审计日志写入失败 | 调账保留；记录系统错误；财务账本以新表为准 |
| 导出时数据库断开 | 500，不生成部分文件，不写成功审计 |
| 客户端断开导出连接 | 服务端停止查询和写文件，清理临时文件 |
| ClickHouse 日志库 | 查询、分页、聚合均可用 |
| SQLite / MySQL / PostgreSQL | 迁移、查询、导出均可用 |

---

## 12. 性能与安全

### 12.1 性能

- 30 天、10 万条消耗日志的目标：汇总查询 < 3 秒，Excel 导出 < 30 秒（生产硬件，具体以压测为准）。
- Excel 使用流式写入（`excelize.StreamWriter`），避免一次性加载全部行。
- 明细查询使用游标/分批读取，避免 `OFFSET` 深分页；导出内部按 `(created_at, id)` 递增扫描。
- 汇总 SQL 只取必要列，避免 `SELECT *`。
- 导出前先做行数预估；超过上限直接拒绝，不创建临时文件。

### 12.2 安全

- 所有接口 `AdminAuth`；导出动作写审计日志。
- SQL 参数化，禁止拼接用户输入；模型名通配沿用现有 `sanitizeLikePattern`。
- CSV 公式注入防护。
- 不导出任何密钥、令牌 hash、用户密码、IP。
- 临时文件写入系统临时目录，导出完成后立即删除；文件名使用随机后缀，避免并发冲突。
- 导出文件名不含用户输入，防止响应头注入。

---

## 13. 验收标准

| # | 场景 | 预期 |
|---|---|---|
| A1 | 管理员打开页面 | 默认近 30 天、全部用户/模型；6 个页签和汇总卡片正常渲染；页面标注“不是余额对账表” |
| A2 | 按用户 + 时间 + 模型筛选 | 6 个页签数据均只包含匹配记录；汇总卡片随之变化 |
| A3 | 消耗与退款 | 预扣后发生退款的记录，消耗明细和退款明细各出现一条；净消耗 = 消耗 - 退款 |
| A4 | 负净消耗 | 窗口内退款大于消耗时，净消耗展示负数，不截断 |
| A5 | 支付充值 | 完成一笔 Stripe/Waffo/Creem/BlueWhalePay 订单；充值明细显示实付金额和到账额度，新订单来源=`精确` |
| A6 | 管理员加额度 | 新表新增一条 `action=add`、`quota_delta=+N` 的记录；调整页签显示 `+N`，操作人正确 |
| A7 | 管理员减额度 / 覆盖 | 减额度显示 `-N`；覆盖显示 `after-before`，前后额度精确 |
| A8 | 历史管理员调账 | 可解析的旧记录标记 `估算`；不可解析的记录标记 `不可解析`，不计入任何合计，并显示提示 |
| A9 | 历史充值 | 无 `quota_credited` 的老订单按规则估算并标记来源，实付金额精确 |
| A10 | 双合计 | 精确合计和估算合计分列；估算值不出现在精确合计中；两者与明细一致 |
| A11 | Excel 导出 | 7 个 Sheet；数值列可求和；时间列 UTC+8；筛选条件和口径写在「导出说明」 |
| A12 | CSV 导出 | 当前页签字段与 Excel 对应 Sheet 一致；UTF-8 BOM；数值列无单位；公式注入被转义 |
| A13 | 空数据导出 | 生成只含表头的文件，不报错 |
| A14 | 权限 | 普通用户访问页面和接口均被拒；侧边栏不可见 |
| A15 | 参数校验 | 时间倒置、跨度超限、行数超限均返回 400 和明确提示 |
| A16 | 并发导出 | 同一管理员第二次导出返回 429 |
| A17 | 审计 | 每次成功导出写入 `billing_report.export` 管理审计日志，含筛选条件和行数 |
| A18 | 跨数据库 | SQLite / MySQL / PostgreSQL 下迁移、查询、导出结果一致 |
| A19 | ClickHouse 日志库 | 配置 ClickHouse 时查询和导出正常 |
| A20 | 数据一致性 | 各明细行数与汇总口径一致；汇总卡片 = 对应明细合计 |
| A21 | 日志保留 | 日志被清理后，报表只统计仍存在的日志，不报错 |
| A22 | 调账事务 | 模拟调账中途失败，余额和调账行都不变；不出现半条记录 |
| A23 | 上线下线 | 发版后 AutoMigrate 自动建表/加列；回滚二进制后旧版本正常启动，新增数据不被破坏 |

---

## 14. 测试建议

- 后端表驱动测试：
  - 充值到账额度规则（各 provider + `RCH` 前缀 + 历史估算）。
  - 管理员调账历史字符串解析（USD/CNY/TOKENS/CUSTOM/非法格式/覆盖差额）。
  - 新表调账事务与行锁：加/减/覆盖的前后额度、delta 符号。
  - 汇总公式（消耗 - 退款 = 净消耗；精确/估算分列；负净消耗）。
  - 日志解析分界：新表首行之前的 `type=3` 才解析，之后不双计。
  - 时间范围与行数上限校验。
  - CSV BOM、表头、公式注入转义。
  - 权限：非管理员 403。
  - 跨数据库迁移（SQLite/MySQL/PostgreSQL）。
- 前端测试：
  - 筛选参数与 URL 同步。
  - 6 个页签切换和空态。
  - 导出按钮 loading/错误提示。
  - 权限隐藏。
- 手工验收：覆盖 A1–A23，至少准备 1 个历史订单、1 个历史管理员调账、1 个新管理员调账、1 个退款任务。

---

## 15. 原型与附图

本 PRD 附 ASCII 线框图（5.2）。评审时如需高保真原型，可用前端组件先行出图，评审通过后再进入开发。

页面变化说明：

- 新增独立页面和侧边栏入口，不改动现有用户管理、使用日志页面的布局。
- 新增 6 个页签和 1 个导出下拉；不复用使用日志页的表格，避免筛选和列定义互相影响。
- 复用现有 `SectionPageLayout`、`DataTablePage`、日期范围选择器、用户搜索接口和 i18n 机制。

---

## 16. 开放问题

| # | 问题 | 建议 | 需谁确认 |
|---|---|---|---|
| 1 | 目标产品版本 | 待填 | 产品 |
| 2 | 财务人员使用哪个管理员账号 | 先共用管理员账号；独立角色另开需求 | 产品 / 运维 |
| 3 | 日志保留策略 | 在「导出说明」提示；必要时调整保留期 | 产品 / 运维 |
| 4 | 是否给 `top_ups` 增加 `(status, complete_time)` 索引 | 生产表大则加；表小可省 | 研发 / DBA |
| 5 | 是否需要“期初/期末余额对账” | 本次不做；如需要另开需求加每日余额快照 | 产品 / 财务 |
| 6 | 是否需要包含订阅、兑换码、签到等其他入账 | 本次不做；如需要另开需求 | 产品 |

---

## 17. 变更记录

| 版本 | 日期 | 变更 | 作者 |
|---|---|---|---|
| v0.1 | 2026-10-22 | 初稿：确认独立入口、管理员充值口径、多 Sheet 导出、PRD 先行 | Agent |
| v0.2 | 2026-10-22 | 按评审结论修订：管理员调账改为独立 `quota_adjustments` 表；确认四类流水不做余额对账；双合计；权限、时区、充值归期、CSV、上限、金额口径、数据库迁移方案 | Agent |

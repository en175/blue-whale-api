# options — 全局配置表

!!! info "模块 / 定位"
    **模块:** 计费 / 系统配置

    **定位:** 全局 key-value 配置,所有价格倍率 + 系统设置的**唯一存放点**

    **关联:** 价格在 `options`｜元数据在 `models`｜路由在 `abilities`｜消费在 `logs`

    **别在这找:** 模型图标/描述/厂商(那是 `models` 表)

## 表结构

| 列名 | 类型 | 可空 | 键 | 含义 |
|---|---|---|---|---|
| `key` | varchar(191) | NO | **主键** | 配置项名。取值见下方 key 字典 |
| `value` | longtext | YES | — | 配置值。价格类为 JSON 字符串(模型名→倍率) |

> 表只有 2 列 —— 全部价格信息塞在 `ModelRatio` 这一行 value 的大 JSON 里,**不是一个模型一行**。

## key 字典(常见取值)

| key | value 类型 | 含义 | 实测示例 |
|---|---|---|---|
| `ModelRatio` | JSON map | **输入 token 倍率**,ratio=1 ⇔ $2/百万 | `{"doubao-seed-2-1-pro-260628":0.42857}` |
| `CompletionRatio` | JSON map | **输出/输入倍数** | `{"doubao-seed-1-6":10}` |
| `ModelPrice` | JSON map | **按次美元价**(图片/音频),$/次 | `{"doubao-seedream-5-0-pro":0.0857}` |
| `CacheRatio` | JSON map | **缓存命中倍率**(0~1) | `{"doubao-seed-1-6":0.2}` |
| `GroupRatio` | JSON map | **分组倍率** {default/vip/svip} | `{"default":0.99,"vip":0.9,"svip":0.85}` |
| `quota_display_type` | 字符串 | 显示币种 | `CNY` |
| `USDExchangeRate` | 数字 | 展示汇率(仅展示层) | `7` |

## 计费公式

```text
输入价(¥/百万token) = ModelRatio × $2 × USDExchangeRate
输出价(¥/百万token) = ModelRatio × CompletionRatio × $2 × USDExchangeRate
用户实付           = 输入或输出价 × GroupRatio[用户分组]
按次计费(图片)    = ModelPrice × GroupRatio[用户分组]
```

!!! example "实测推演 · doubao-seed-2-1-pro-260628(输入)"
    `ModelRatio=0.42857` → 0.42857 × $2 × 7 = **¥6.0/百万** ← UI「官方价」

    default 分组 ×0.99 → **¥5.94/百万** ← UI「我们价」

    节省 = 1 − 0.99 = **1%** ← **分组折扣,不是对火山的让利**

!!! warning "易错点 / 约束"
    - 模型价格在 `options.ModelRatio`,**不在 `models` 表**。改 models 图标不影响价格
    - 改倍率 = **整体读写整段 JSON**(读出 map→改一个 key→写回),不是 `UPDATE` 一行
    - **漏配 ModelRatio = 漏收**:`service/task_billing.go:281` 无 ratio 直接 return 不扣费,日志不报错,用户白嫖
    - 「官方价/我们价/节省」同源 ModelRatio:官方价=default 不打折,我们价=×分组倍率,节省=分组折扣 —— 不是三个数据源

## 关联表

| 表 | 关系 |
|---|---|
| `models` | 模型**展示**元数据(图标/描述/厂商)。通过**模型名**隐式关联,无物理外键 |
| `abilities` | 模型**路由**(模型↔渠道↔分组)。决定能不能用、走哪个渠道 |
| `channels` | 渠道(凭证/key/url)。价格与渠道无关,全局唯一 |
| `logs` | 消费记录。实际扣的 quota = 上面公式算出的值 |
| `users` | 用户分组(group 字段)决定 GroupRatio 用哪个系数 |

> 以上均为**应用层关联**(无物理外键),tbls 的 ER 图画不出来 —— 这就是需要手写业务字典的原因。

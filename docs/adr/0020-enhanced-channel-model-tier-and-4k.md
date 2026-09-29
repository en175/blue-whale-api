# 火山（增强）渠道按模型分档校验，4k 参与抽签并可超分到 4k

渠道 64 的抽签降档从 1080p / 720p 扩到 4k（可降 1080p / 720p / 480p 三档，比例走配置），超分目标从 720p / 1080p 扩到 4k，同时按模型分档在**提交期**校验请求档：4k 只允许 Seedance 2.0，fast / mini 不支持 1080p 与 4k，1.0 / 1.5 系列整体不收。样片（draft）请求一律 400。

**Status:** accepted

## 背景

渠道 64「火山（增强）」靠「抽签降档 + MediaKit 超分回请求档」降本，对客口径与扣费始终按请求档。原设计有三处限制：

1. `needsLotteryRoll` 只认 1080p / 720p，4k 请求没有降本路径——而 Seedance 2.0 已支持 4k 生成。
2. 抽签与超分完全不看模型。请求档超出模型能力时不提前拒绝，要等上游报错再全额退预扣；超分目标也被硬编码白名单锁在 720p / 1080p。
3. 渠道模型列表继承自 `task/doubao` 包，没有 2.5 与 mini。

把 2.5 放进渠道 64 会激活一个 ADR 0019 记录过的缺口：`metadata.draft` 在豆包适配器上零把关透传（ADR 0019 背景 1 逐字记录），而当时的处置是「新开渠道 66，不碰 64」——靠把 2.5 挡在外面回避。前提一旦消失，缺口就暴露。

同时发现一个休眠 bug：真实 2.5 模型 ID `doubao-seedance-2-5-260628` 用连字符 `2-5`，而画布系列判定匹配的是 `"2.5"`（点号），对真实 ID 恒为 false——2.5 的 480p 会误用 2.0 画布（`864×496` 而非 `854×480`），产生真实剥皮误差。既有测试用假名 `doubao-seedance-2.5-preview` 掩盖了它。2.5 不进 64 时碰不到，本次一并修。

## 决策

### 1. 档位能力按模型系列判定，提交期 400

| 模型系列 | 480p | 720p | 1080p | 4k |
|---|---|---|---|---|
| Seedance 2.5 | ✓ | ✓ | ✓ | ✗ |
| Seedance 2.0 | ✓ | ✓ | ✓ | ✓ |
| Seedance 2.0 fast | ✓ | ✓ | ✗ | ✗ |
| Seedance 2.0 mini | ✓ | ✓ | ✗ | ✗ |
| Seedance 1.0 / 1.5 | ✗ | ✗ | ✗ | ✗ |

守卫放在 `ValidateRequestAndSetAction`，返回 400 `invalid_resolution`。**不放在 `BuildRequestBody`**：后者在预扣费之后，且 `relay/relay_task.go` 把它返回的一切错误包成 500、错误码不可自定义。

系列判定用模型名子串：含 `1-0` / `1.0` / `1-5` / `1.5` → 1.x；含 `2.5` 或 `2-5` → 2.5；其余 → 2.0 系列（未知模型名沿用宽松默认）。fast / mini 用 `fast` / `mini` 子串。空 `resolution` 与未知名档位（如 `2k`）放行交上游报错，本次不收紧。

已知限制：该方法跑在 `helper.ModelMappedHelper` 之前，拿到的是映射前的模型名。本渠道无别名（见决策 5），不构成问题。

### 2. 4k 参与抽签，可降 1080p / 720p / 480p

新增 `4k_to_1080p_percent` / `4k_to_720p_percent` / `4k_to_480p_percent` 三个渠道配置项，沿用既有的单次 `[0,100)` 前缀区间语义，三项和 ≤ 100，全 0 即关闭。`resolutionRank` 已有 `Res4k`，抽签命中后 `IsDowngrade` 自动成立，剥皮 / 超分 / 对客改写全部无需新增分支。

### 3. 超分目标支持 4k

MediaKit「画质增强（大模型版）」现支持 4k 输出，`submitEnhance` 的白名单从 `720p` / `1080p` 改为 `720p` / `1080p` / `4k`（不含 480p——480p 请求永不降档，不会成为超分目标）。新增 `RateTier4k`；`enhance_costs.rate_tier` 列宽 `varchar(16)` 足够，无需迁移。

**官方 4k 元/分钟单价为 20 / 40 / 80**（对应 ≤30 / >30 且 ≤60 / >60 且 ≤120 fps，官方换算系数 8 / 16 / 32 × 2.5 元/分钟），`yuanPerMinute` 按此填入。`enhance_costs` 是纯审计表，不影响用户额度与渠道限额；金额算得 0（回包时长缺失、档位无价）时仍打 `LogWarn`。

### 4. draft 请求提交期 400

判定实现在 `relay/channel/task/volcengine25/draft.go` 已有的 `isDraftRequest`，本次**上移**到 `taskcommon.IsDraftRequest` 供两个渠道共用（渠道 66 的调用点改为直接引用，行为逐位不变）。

不采用「64 渠道关闭超分」这条替代路线：对客 `resolution` 由 `rewriteUserVisibleTaskData` 按请求档硬编码，`EstimateBilling` 读未改写的 `metadata.resolution`，剥皮又因未降档而跳过——关掉超分仍会「按请求档扣费、回请求档、交付样片」，只是省下了超分的钱。

### 5. 渠道 64 用本包独立 ModelList

`doubao.ModelList` 渠道 45 / 54 也在用，直接改会连它们一起改。比照渠道 66 的 `GetModelList()` 覆写，渠道 64 新增本包 `ModelList`：2.5 / 2.0 / 2.0-fast / 2.0-mini，不含 1.0 / 1.5。

**这份列表不会出现在管理端模型下拉里**：64 与 66 都没有 `common.ChannelType2APIType` 映射，`controller/model.go` 的 `channelId2Models` 与 `ChannelListModels` 都按 APIType 构建，不含任务视频渠道。要让新模型真正可路由，需管理员在渠道配置里填写模型名落 `abilities` 表。

### 6. 修正 2.5 画布系列判定

判定改为同时接受 `2.5` 与 `2-5`，并统一收敛到 `SeedanceSeriesOf`。既有测试的模型名换成真实 ID `doubao-seedance-2-5-260628`，使回归可被捕获。

## 后果

- 画布表结构不变，仍是两张（2.5 / 2.0 系列）——fast / mini 与 2.0 像素完全相同，不需要第三张。四张档位表的像素数据一行未改。
- 对客行为变更（需公告）：fast / mini 请求 1080p、4k 请求非 2.0 模型，从「上游报错 + 全额退」变成提交期 400，客户端不再看到上游那句具体报错；draft 打到 64 新增 400。400 是本地错误，不触发渠道重试。
- `4k→480p` 是 20 倍上采样，上游是否接受该组合未实测。若上游对 480p 源出 4k 有限制，会走「MediaKit 输入超规格 → 计入一次失败」，3 次后任务 FAILURE 且用户仍按 4k 档扣费（ADR 0016）。建议该百分比先配 0，小流量验证后再开。
- 上线前需确认生产没有 1.0 / 1.5 模型路由到渠道 64；若有，调用会被新的 400 打断。另需在 `video_tier_pricing.price_table` 配好 2.0 的 4k 档与 2.5 各档单价，否则静默按基准价计费。

相关：ADR [0011](0011-doubao-video-lottery-channel.md)（新开渠道类型）、[0012](0012-lottery-channel-quota-uses-upstream-tier.md)、[0014](0014-enhance-retry-then-fail.md)、[0015](0015-mediakit-resolution-and-rate-card.md)、[0016](0016-charge-requested-tier-when-enhance-fails.md)、[0017](0017-video-token-peel-from-upstream.md)、[0019](0019-seedance-draft-mode.md)（样片模式，本渠道不接）。

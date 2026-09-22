-- ============================================================================
-- MediaKit 渠道一键配置（渠道类型 65）
-- 模型清单生成自 relay/channel/mediakit/registry.go，共 66 个工具
-- （官方三个集合 64 + 集合外的 extract-audio、trim-audio 各 1）。
-- 注册表加工具后重新生成本文件的模型段即可。
--
-- 为什么不能只配一样：建渠道 ≠ 能用，三件事都得做，缺一样就报错 ——
--   ① channels 行（密钥走 key 字段，不是 setting）
--   ② channels.models 里列全 66 个模型
--   ③ abilities 行 —— 对客路由查的是这张表，由 ② × 分组派生
--   （同步工具还要求 model_price 已配，见第 3 节，否则 400）
--
-- 方言：MySQL 5.7.8+ / MariaDB。用了 LAST_INSERT_ID、JSON_OBJECT、
--       JSON_MERGE_PATCH（8.0+，仅第 3 节需要）、SUBSTRING_INDEX 拆串。
--       SQLite 与 PostgreSQL 需改写。
--
-- 排序规则陷阱：user variable 会继承「连接的」collation（随客户端默认字符集漂移，
--       mysql CLI 不给 --default-character-set 时是 latin1_swedish_ci，给了 utf8mb4
--       则是 utf8mb4_0900_ai_ci），而 channels.name 的 collation 由建库时决定
--       （本仓部署常见 utf8mb4_unicode_ci）。两边都是 IMPLICIT，直接比较会报
--       ERROR 1267 Illegal mix of collations。所以下面与 @channel_name 的比较一律写成
--       BINARY LOWER(x) = BINARY LOWER(y)：先各自转成字节串再比，不参与 collation 协商，
--       任何连接字符集下都成立；同时用 LOWER 保住大小写不敏感的原始语义。
--
-- 执行：mysql -h <host> -u <user> -p <db> < mediakit-bootstrap.sql
-- ============================================================================

-- ▼▼▼ 只改这一段 ▼▼▼
SET @mediakit_key    := 'sk-CHANGE-ME-mediakit-api-key';  -- 换成本环境的真 key
SET @channel_name    := 'MediaKit';
SET @channel_groups  := 'default';   -- 逗号分隔，如 'default,vip,svip'
SET @default_minutes := 0;           -- 时长类工具预估分钟；0 = 预扣退回 model_price 基准额度
SET @set_price       := 0;           -- 1 = 同时写 model_price；0 = 跳过（跳过则同步工具会 400）
SET @price_per_call  := 0.5;         -- 上一行开启时用的单价（元/次）
SET @price_probe     := 0.0001;      -- 视频/音频元信息探测单价，对齐官方 0.1 元/千次
SET @price_probe_img := 0.00024;     -- 图像元信息探测单价，见下方第 3 节说明
SET @price_trim      := 0;           -- 音频截取 trim-audio 单价；本期零价（IKHB7L），不随 @price_per_call
-- ▲▲▲ 只改这一段 ▲▲▲

START TRANSACTION;

-- ── 1. 建渠道 ──────────────────────────────────────────────────────────────
-- 幂等：同名同类型的渠道已存在就跳过，不会建出第二个。
INSERT INTO channels
  (type, `key`, status, name, weight, created_time, base_url,
   models, `group`, priority, auto_ban, setting, channel_info, tag)
SELECT 65, @mediakit_key, 1, @channel_name, 0, UNIX_TIMESTAMP(), '',
       'mediakit-enhance-image,mediakit-remove-image-background,mediakit-image-ocr,mediakit-evaluate-image-quality,mediakit-erase-image,mediakit-expand-image-canvas,mediakit-smart-crop-image,mediakit-slim-image,mediakit-remove-image-elements,mediakit-translate-image-text,mediakit-face-blur-image,mediakit-generate-product-scene-image,mediakit-add-image-invisible-watermark,mediakit-extract-image-invisible-watermark,mediakit-round-corner-image,mediakit-flip-image,mediakit-rotate-image,mediakit-sharpen-image,mediakit-gaussian-blur-image,mediakit-mosaic-image,mediakit-add-image-watermark,mediakit-adjust-image-color,mediakit-resize-image,mediakit-invert-image,mediakit-crop-image,mediakit-compress-image,mediakit-probe-image-metadata,mediakit-enhance-video-generative,mediakit-enhance-video,mediakit-enhance-video-fast,mediakit-erase-video-subtitle,mediakit-erase-video-subtitle-pro,mediakit-segment-scenes,mediakit-asr-subtitles,mediakit-video-ocr,mediakit-analyze-video-highlights,mediakit-analyze-video-storyline,mediakit-generate-highlights-microdrama,mediakit-generate-highlights-minigame,mediakit-generate-highlights-movie,mediakit-matte-greenscreen-video,mediakit-matte-portrait-video,mediakit-drama-script,mediakit-drama-script-vertical,mediakit-drama-recap,mediakit-drama-recap-vertical,mediakit-add-video-invisible-watermark,mediakit-extract-video-invisible-watermark,mediakit-assess-video-quality,mediakit-transcode-video,mediakit-remux-video,mediakit-martencode-video,mediakit-face-blur-video,mediakit-face-swap-video,mediakit-semantic-segment,mediakit-video-frame-interpolation,mediakit-lip-sync,mediakit-reframe-video-to-vertical,mediakit-extract-frames,mediakit-probe-video-metadata,mediakit-separate-voice,mediakit-transcode-audio,mediakit-extract-audio,mediakit-trim-audio,mediakit-detect-voice-activity,mediakit-probe-audio-metadata', @channel_groups, 0, 1,
       JSON_OBJECT(
         'force_format', false,
         'thinking_to_content', false,
         'proxy', '',
         'pass_through_body_enabled', false,
         'system_prompt', '',
         'system_prompt_override', false,
         'mediakit_default_minutes', @default_minutes
       ),
       '{"is_multi_key": false, "multi_key_mode": "", "multi_key_size": 0, "multi_key_status_list": null, "multi_key_polling_index": 0}',
       ''
FROM DUAL
WHERE NOT EXISTS (
  SELECT 1 FROM channels WHERE BINARY LOWER(name) = BINARY LOWER(@channel_name) AND type = 65
);

-- 拿渠道 id：新建用 LAST_INSERT_ID()，已存在则回退查一次。
SET @channel_id := LAST_INSERT_ID();
SET @channel_id := IF(
  @channel_id = 0,
  (SELECT id FROM channels WHERE BINARY LOWER(name) = BINARY LOWER(@channel_name) AND type = 65 ORDER BY id DESC LIMIT 1),
  @channel_id
);

-- 已存在的渠道也把模型列表补全（覆盖式，等于在控制台里把模型勾满）。
UPDATE channels SET models = 'mediakit-enhance-image,mediakit-remove-image-background,mediakit-image-ocr,mediakit-evaluate-image-quality,mediakit-erase-image,mediakit-expand-image-canvas,mediakit-smart-crop-image,mediakit-slim-image,mediakit-remove-image-elements,mediakit-translate-image-text,mediakit-face-blur-image,mediakit-generate-product-scene-image,mediakit-add-image-invisible-watermark,mediakit-extract-image-invisible-watermark,mediakit-round-corner-image,mediakit-flip-image,mediakit-rotate-image,mediakit-sharpen-image,mediakit-gaussian-blur-image,mediakit-mosaic-image,mediakit-add-image-watermark,mediakit-adjust-image-color,mediakit-resize-image,mediakit-invert-image,mediakit-crop-image,mediakit-compress-image,mediakit-probe-image-metadata,mediakit-enhance-video-generative,mediakit-enhance-video,mediakit-enhance-video-fast,mediakit-erase-video-subtitle,mediakit-erase-video-subtitle-pro,mediakit-segment-scenes,mediakit-asr-subtitles,mediakit-video-ocr,mediakit-analyze-video-highlights,mediakit-analyze-video-storyline,mediakit-generate-highlights-microdrama,mediakit-generate-highlights-minigame,mediakit-generate-highlights-movie,mediakit-matte-greenscreen-video,mediakit-matte-portrait-video,mediakit-drama-script,mediakit-drama-script-vertical,mediakit-drama-recap,mediakit-drama-recap-vertical,mediakit-add-video-invisible-watermark,mediakit-extract-video-invisible-watermark,mediakit-assess-video-quality,mediakit-transcode-video,mediakit-remux-video,mediakit-martencode-video,mediakit-face-blur-video,mediakit-face-swap-video,mediakit-semantic-segment,mediakit-video-frame-interpolation,mediakit-lip-sync,mediakit-reframe-video-to-vertical,mediakit-extract-frames,mediakit-probe-video-metadata,mediakit-separate-voice,mediakit-transcode-audio,mediakit-extract-audio,mediakit-trim-audio,mediakit-detect-voice-activity,mediakit-probe-audio-metadata' WHERE id = @channel_id;

-- ── 2. 派生 abilities ──────────────────────────────────────────────────────
-- 对齐 model/ability.go 的 AddAbilities：group × model 笛卡尔积，
-- enabled 取渠道状态，priority / weight / tag 照抄渠道行。
-- 分组从 @channel_groups 自动拆，不会和 channels.group 走岔。
DELETE FROM abilities WHERE channel_id = @channel_id;

INSERT INTO abilities (`group`, model, channel_id, enabled, priority, weight, tag)
SELECT g.grp, m.model, c.id,
       CASE WHEN c.status = 1 THEN 1 ELSE 0 END,
       c.priority, c.weight, c.tag
FROM channels c
CROSS JOIN (
    -- 把 @channel_groups 按逗号拆成行（最多 10 组，够用）
    SELECT SUBSTRING_INDEX(SUBSTRING_INDEX(@channel_groups, ',', n.n), ',', -1) AS grp
    FROM (SELECT 1 AS n UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT 4
          UNION ALL SELECT 5 UNION ALL SELECT 6 UNION ALL SELECT 7 UNION ALL SELECT 8
          UNION ALL SELECT 9 UNION ALL SELECT 10) n
    WHERE n.n <= 1 + (CHAR_LENGTH(@channel_groups) - CHAR_LENGTH(REPLACE(@channel_groups, ',', '')))
      AND SUBSTRING_INDEX(SUBSTRING_INDEX(@channel_groups, ',', n.n), ',', -1) <> ''
) g
CROSS JOIN (
    SELECT 'mediakit-enhance-image' AS model
    UNION ALL
    SELECT 'mediakit-remove-image-background' AS model
    UNION ALL
    SELECT 'mediakit-image-ocr' AS model
    UNION ALL
    SELECT 'mediakit-evaluate-image-quality' AS model
    UNION ALL
    SELECT 'mediakit-erase-image' AS model
    UNION ALL
    SELECT 'mediakit-expand-image-canvas' AS model
    UNION ALL
    SELECT 'mediakit-smart-crop-image' AS model
    UNION ALL
    SELECT 'mediakit-slim-image' AS model
    UNION ALL
    SELECT 'mediakit-remove-image-elements' AS model
    UNION ALL
    SELECT 'mediakit-translate-image-text' AS model
    UNION ALL
    SELECT 'mediakit-face-blur-image' AS model
    UNION ALL
    SELECT 'mediakit-generate-product-scene-image' AS model
    UNION ALL
    SELECT 'mediakit-add-image-invisible-watermark' AS model
    UNION ALL
    SELECT 'mediakit-extract-image-invisible-watermark' AS model
    UNION ALL
    SELECT 'mediakit-round-corner-image' AS model
    UNION ALL
    SELECT 'mediakit-flip-image' AS model
    UNION ALL
    SELECT 'mediakit-rotate-image' AS model
    UNION ALL
    SELECT 'mediakit-sharpen-image' AS model
    UNION ALL
    SELECT 'mediakit-gaussian-blur-image' AS model
    UNION ALL
    SELECT 'mediakit-mosaic-image' AS model
    UNION ALL
    SELECT 'mediakit-add-image-watermark' AS model
    UNION ALL
    SELECT 'mediakit-adjust-image-color' AS model
    UNION ALL
    SELECT 'mediakit-resize-image' AS model
    UNION ALL
    SELECT 'mediakit-invert-image' AS model
    UNION ALL
    SELECT 'mediakit-crop-image' AS model
    UNION ALL
    SELECT 'mediakit-compress-image' AS model
    UNION ALL
    SELECT 'mediakit-probe-image-metadata' AS model
    UNION ALL
    SELECT 'mediakit-enhance-video-generative' AS model
    UNION ALL
    SELECT 'mediakit-enhance-video' AS model
    UNION ALL
    SELECT 'mediakit-enhance-video-fast' AS model
    UNION ALL
    SELECT 'mediakit-erase-video-subtitle' AS model
    UNION ALL
    SELECT 'mediakit-erase-video-subtitle-pro' AS model
    UNION ALL
    SELECT 'mediakit-segment-scenes' AS model
    UNION ALL
    SELECT 'mediakit-asr-subtitles' AS model
    UNION ALL
    SELECT 'mediakit-video-ocr' AS model
    UNION ALL
    SELECT 'mediakit-analyze-video-highlights' AS model
    UNION ALL
    SELECT 'mediakit-analyze-video-storyline' AS model
    UNION ALL
    SELECT 'mediakit-generate-highlights-microdrama' AS model
    UNION ALL
    SELECT 'mediakit-generate-highlights-minigame' AS model
    UNION ALL
    SELECT 'mediakit-generate-highlights-movie' AS model
    UNION ALL
    SELECT 'mediakit-matte-greenscreen-video' AS model
    UNION ALL
    SELECT 'mediakit-matte-portrait-video' AS model
    UNION ALL
    SELECT 'mediakit-drama-script' AS model
    UNION ALL
    SELECT 'mediakit-drama-script-vertical' AS model
    UNION ALL
    SELECT 'mediakit-drama-recap' AS model
    UNION ALL
    SELECT 'mediakit-drama-recap-vertical' AS model
    UNION ALL
    SELECT 'mediakit-add-video-invisible-watermark' AS model
    UNION ALL
    SELECT 'mediakit-extract-video-invisible-watermark' AS model
    UNION ALL
    SELECT 'mediakit-assess-video-quality' AS model
    UNION ALL
    SELECT 'mediakit-transcode-video' AS model
    UNION ALL
    SELECT 'mediakit-remux-video' AS model
    UNION ALL
    SELECT 'mediakit-martencode-video' AS model
    UNION ALL
    SELECT 'mediakit-face-blur-video' AS model
    UNION ALL
    SELECT 'mediakit-face-swap-video' AS model
    UNION ALL
    SELECT 'mediakit-semantic-segment' AS model
    UNION ALL
    SELECT 'mediakit-video-frame-interpolation' AS model
    UNION ALL
    SELECT 'mediakit-lip-sync' AS model
    UNION ALL
    SELECT 'mediakit-reframe-video-to-vertical' AS model
    UNION ALL
    SELECT 'mediakit-extract-frames' AS model
    UNION ALL
    SELECT 'mediakit-probe-video-metadata' AS model
    UNION ALL
    SELECT 'mediakit-separate-voice' AS model
    UNION ALL
    SELECT 'mediakit-transcode-audio' AS model
    UNION ALL
    SELECT 'mediakit-extract-audio' AS model
    UNION ALL
    SELECT 'mediakit-trim-audio' AS model
    UNION ALL
    SELECT 'mediakit-detect-voice-activity' AS model
    UNION ALL
    SELECT 'mediakit-probe-audio-metadata' AS model
) m
WHERE c.id = @channel_id;

COMMIT;

-- ── 3. 定价（可选，MySQL 8.0+）─────────────────────────────────────────────
-- 不配的话：同步工具被 relay/mediakit_handler.go 的硬守卫拦成 400
--   "mediakit model X has no model price configured"；
-- 异步工具既无 model_price 又无 model_ratio，报 modelPriceNotConfiguredError。
-- 合并式写入，不动已有模型的价。
--
-- 3 个元信息探测工具按上游口径单独定价（官方对它们并非免费）：
--   · probe-video-metadata / probe-audio-metadata：官方按成功处理次数 0.1 元/千次
--     → 折合 0.0001 元/次，即 @price_probe。
--   · probe-image-metadata：官方归在「图像基础编辑」计费单元，按 0.025 元/GiB、
--     自然月 10 TiB 免费额度后累进。**网关不测量图片体积**（模型定价只有元/次一档，
--     同步链路里没有任何体积项），所以只能折成元/次近似：按单图 10 MiB 上限
--     10/1024 GiB × 0.025 = 0.00024 元/次，即 @price_probe_img。
--     大图不亏、小图略多收，差额在 1e-4 元量级可忽略。
--
-- 另有一条**不随 @price_per_call** 的特例：
--   · trim-audio（音频截取）本期零价，即 @price_trim（默认 0）。它是下游
--     「音频截取」功能的上线前置（IKHB7L），对客免费，故单列一个变量而不是跟着
--     通用单价走——否则把 @price_per_call 调成别的值时会连带改掉它。要恢复收费
--     改这一行即可，不必动其它模型。
SET @new_prices := (
  SELECT JSON_OBJECTAGG(
           m.model,
           CASE m.model
             WHEN 'mediakit-probe-video-metadata' THEN @price_probe
             WHEN 'mediakit-probe-audio-metadata' THEN @price_probe
             WHEN 'mediakit-probe-image-metadata' THEN @price_probe_img
             WHEN 'mediakit-trim-audio' THEN @price_trim
             ELSE @price_per_call
           END)
  FROM (    SELECT 'mediakit-enhance-image' AS model
    UNION ALL
    SELECT 'mediakit-remove-image-background' AS model
    UNION ALL
    SELECT 'mediakit-image-ocr' AS model
    UNION ALL
    SELECT 'mediakit-evaluate-image-quality' AS model
    UNION ALL
    SELECT 'mediakit-erase-image' AS model
    UNION ALL
    SELECT 'mediakit-expand-image-canvas' AS model
    UNION ALL
    SELECT 'mediakit-smart-crop-image' AS model
    UNION ALL
    SELECT 'mediakit-slim-image' AS model
    UNION ALL
    SELECT 'mediakit-remove-image-elements' AS model
    UNION ALL
    SELECT 'mediakit-translate-image-text' AS model
    UNION ALL
    SELECT 'mediakit-face-blur-image' AS model
    UNION ALL
    SELECT 'mediakit-generate-product-scene-image' AS model
    UNION ALL
    SELECT 'mediakit-add-image-invisible-watermark' AS model
    UNION ALL
    SELECT 'mediakit-extract-image-invisible-watermark' AS model
    UNION ALL
    SELECT 'mediakit-round-corner-image' AS model
    UNION ALL
    SELECT 'mediakit-flip-image' AS model
    UNION ALL
    SELECT 'mediakit-rotate-image' AS model
    UNION ALL
    SELECT 'mediakit-sharpen-image' AS model
    UNION ALL
    SELECT 'mediakit-gaussian-blur-image' AS model
    UNION ALL
    SELECT 'mediakit-mosaic-image' AS model
    UNION ALL
    SELECT 'mediakit-add-image-watermark' AS model
    UNION ALL
    SELECT 'mediakit-adjust-image-color' AS model
    UNION ALL
    SELECT 'mediakit-resize-image' AS model
    UNION ALL
    SELECT 'mediakit-invert-image' AS model
    UNION ALL
    SELECT 'mediakit-crop-image' AS model
    UNION ALL
    SELECT 'mediakit-compress-image' AS model
    UNION ALL
    SELECT 'mediakit-probe-image-metadata' AS model
    UNION ALL
    SELECT 'mediakit-enhance-video-generative' AS model
    UNION ALL
    SELECT 'mediakit-enhance-video' AS model
    UNION ALL
    SELECT 'mediakit-enhance-video-fast' AS model
    UNION ALL
    SELECT 'mediakit-erase-video-subtitle' AS model
    UNION ALL
    SELECT 'mediakit-erase-video-subtitle-pro' AS model
    UNION ALL
    SELECT 'mediakit-segment-scenes' AS model
    UNION ALL
    SELECT 'mediakit-asr-subtitles' AS model
    UNION ALL
    SELECT 'mediakit-video-ocr' AS model
    UNION ALL
    SELECT 'mediakit-analyze-video-highlights' AS model
    UNION ALL
    SELECT 'mediakit-analyze-video-storyline' AS model
    UNION ALL
    SELECT 'mediakit-generate-highlights-microdrama' AS model
    UNION ALL
    SELECT 'mediakit-generate-highlights-minigame' AS model
    UNION ALL
    SELECT 'mediakit-generate-highlights-movie' AS model
    UNION ALL
    SELECT 'mediakit-matte-greenscreen-video' AS model
    UNION ALL
    SELECT 'mediakit-matte-portrait-video' AS model
    UNION ALL
    SELECT 'mediakit-drama-script' AS model
    UNION ALL
    SELECT 'mediakit-drama-script-vertical' AS model
    UNION ALL
    SELECT 'mediakit-drama-recap' AS model
    UNION ALL
    SELECT 'mediakit-drama-recap-vertical' AS model
    UNION ALL
    SELECT 'mediakit-add-video-invisible-watermark' AS model
    UNION ALL
    SELECT 'mediakit-extract-video-invisible-watermark' AS model
    UNION ALL
    SELECT 'mediakit-assess-video-quality' AS model
    UNION ALL
    SELECT 'mediakit-transcode-video' AS model
    UNION ALL
    SELECT 'mediakit-remux-video' AS model
    UNION ALL
    SELECT 'mediakit-martencode-video' AS model
    UNION ALL
    SELECT 'mediakit-face-blur-video' AS model
    UNION ALL
    SELECT 'mediakit-face-swap-video' AS model
    UNION ALL
    SELECT 'mediakit-semantic-segment' AS model
    UNION ALL
    SELECT 'mediakit-video-frame-interpolation' AS model
    UNION ALL
    SELECT 'mediakit-lip-sync' AS model
    UNION ALL
    SELECT 'mediakit-reframe-video-to-vertical' AS model
    UNION ALL
    SELECT 'mediakit-extract-frames' AS model
    UNION ALL
    SELECT 'mediakit-probe-video-metadata' AS model
    UNION ALL
    SELECT 'mediakit-separate-voice' AS model
    UNION ALL
    SELECT 'mediakit-transcode-audio' AS model
    UNION ALL
    SELECT 'mediakit-extract-audio' AS model
    UNION ALL
    SELECT 'mediakit-trim-audio' AS model
    UNION ALL
    SELECT 'mediakit-detect-voice-activity' AS model
    UNION ALL
    SELECT 'mediakit-probe-audio-metadata' AS model) m
);

SET @merged_prices := IF(
  @set_price = 1,
  JSON_MERGE_PATCH(
    IFNULL((SELECT value FROM options WHERE `key` = 'ModelPrice'), '{}'),
    @new_prices
  ),
  NULL
);

INSERT INTO options (`key`, value)
SELECT 'ModelPrice', @merged_prices
FROM DUAL
WHERE @set_price = 1
ON DUPLICATE KEY UPDATE value = @merged_prices;

-- ── 4. 自检 ───────────────────────────────────────────────────────────────
-- 期望：n_models = 66；abilities_rows = 66 × 分组数。
SELECT c.id                       AS channel_id,
       c.type                     AS channel_type,
       c.name                     AS channel_name,
       c.`group`                  AS grp_list,
       c.status                   AS status,
       (CHAR_LENGTH(c.models) - CHAR_LENGTH(REPLACE(c.models, ',', '')) + 1) AS n_models,
       (SELECT COUNT(*) FROM abilities a WHERE a.channel_id = c.id)          AS abilities_rows
FROM channels c
WHERE c.id = @channel_id;

-- 缓存提醒：若目标环境设了 MEMORY_CACHE_ENABLED=true，直接改库不会立刻生效，
-- 需重启服务或触发一次渠道缓存刷新（控制台改任意渠道保存一次也行）。

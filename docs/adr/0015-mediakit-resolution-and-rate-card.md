# MediaKit 超分显式传请求档，花费按官方元/分钟表记到分

调 MediaKit 时 `resolution` 只传 720p、1080p 或 4k（请求档）；不传 fps/bitrate，帧率跟 Seedance 片源。超分花费用官方「输出分辨率档 × 帧率档 → 元/分钟」× 输出时长，四舍五入到分。价表内置。2K 档 v1 不用。4K 档价表由 [ADR 0020](0020-enhanced-channel-model-tier-and-4k.md) 补入。

**Status:** accepted

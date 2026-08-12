# scripts 说明

## 公开内容已转为手工维护（2026-08-12 起）

`content/docs/` 下的公开文档现按 IK7JPE 视频生成定位**手工维护**。
**请勿重跑 `generate-openapi-docs.ts` / `generate-openapi-from-http.ts`（`bun run generate:openapi`）覆盖 content/docs**——那会复活已按 PRD 裁剪/隐藏的未开放章节（Chat/Embeddings/Management 等）。

被导航隐藏的旧章节（`content/docs/zh/api`、`zh/apps`、`en/`）文件保留在仓库中，仅从 meta.json 的 pages 数组移除，侧边栏不显示。

## check-public-surface.ts

公开面红线检查：扫描对外公开的新增章节（index/quickstart/authentication/models/video/errors/rate-limits/faq）是否包含未开放模型名（GPT-4/Claude/Gemini）、充值/免费额度文案、未开放 API 端点（chat/embeddings/images/audio/management）。

运行：

```bash
bun run check:public-surface
```

发现禁止内容时退出码非 0 并列出具体文件与命中项。

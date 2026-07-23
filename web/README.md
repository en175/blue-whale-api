# web — new-api-web-workspace

前端 monorepo 工作区（`new-api-web-workspace`），成员：

- `default/` — 默认前端
- `classic/` — 经典前端

接口文档站不在此工作区，独立于 `api-docs/`（fumadocs + fumadocs-openapi，端口 3011）：

- 开发：`cd api-docs && bun run dev`（端口 3011）
- spec 源：`api-docs/NewAPI.apifox.json`，覆盖 43 个 ai-model 接口 + management，三语 zh/en/ja。

旧的 Docusaurus 文档站 `web-doc/` 已弃用删除，见 [ADR 0024 弃用 web-doc 文档站](../../../docs/adr/0024-deprecate-web-doc.md)。

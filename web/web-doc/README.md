# Blue Whale API 文档站

基于 Docusaurus + `docusaurus-plugin-openapi-docs`，从仓库现成的 `openapi/relay.json`（AI 模型接口，35 个接口）自动渲染接口文档。

## 快速开始

```bash
npm install            # 安装依赖
npm run gen-api-docs   # 从 openapi/relay.json 生成接口页（docs/api/，已 gitignore）
npm run start          # 本地预览 http://localhost:3010
npm run build          # 生产构建到 build/
```

> `docs/api/` 是生成物，不入库。clone 后必须先跑 `npm run gen-api-docs`。

## 结构

| 路径 | 说明 |
|---|---|
| `openapi/relay.json` | AI 模型接口 OpenAPI spec（new-api 源副本，web-doc 自包含） |
| `docusaurus.config.js` | 站点配置（presets + openapi 插件 + theme + languageTabs） |
| `sidebars.js` | 侧边栏（加载 plugin slice + 拆大类/格式两层嵌套） |
| `docs/intro.md` | 首页 |
| `docs/api/` | `gen-api-docs` 生成（不入库） |

## 更新接口文档

修改 `openapi/relay.json` 后重新生成：

```bash
npm run clean-api-docs && npm run gen-api-docs
```

## 技术栈

- Docusaurus 3.10
- `docusaurus-plugin-openapi-docs` + `docusaurus-theme-openapi-docs` 5.1
- Node ≥ 18

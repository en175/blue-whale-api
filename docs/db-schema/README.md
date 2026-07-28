# new-api 数据字典

new-api 数据库的 **结构 + 业务字典** 文档。`tbls` 自动抓结构,核心表手写业务字典增强。

## 用途

- **改 DB 前先读**:动任何表前,读对应 `.md` 理解结构 + 业务规则 + 易错点(尤其「价格在 `options` 不在 `models`」这类坑)
- **AI 查表结构**:直接读 `docs/*.md` 源码,不用每次连库猜结构
- **团队浏览**:本地起站点(下方),有导航 + 搜索 + 分组

## 目录结构

```
docs/db-schema/
├── mkdocs.yml          # MkDocs Material 配置(主题/分组/搜索)
└── docs/
    ├── *.md            # 31 张表文档
    └── *.svg           # 单表 ER 图 + schema.svg 全局图
```

- **13 张核心表已增强**(计费财务 + 渠道模型):含 定位 / 表结构 / 业务规则 / 易错点 / 关联
  - `options` `channels` `abilities` `models` `vendors` `top_ups` `redemptions` `quota_data` `checkins` `subscription_plans` `subscription_orders` `user_subscriptions` `subscription_pre_consume_records`
- **其余 18 表为 tbls 结构版**(DDL / 列 / 索引),待逐步增强

> ⚠️ new-api **无物理外键**,表关系在应用层(Go 代码)。`schema.svg` 里是孤立表 —— 真实关系看各表 md 的「关联表」段。

## 本地预览(网站 + 搜索 + 导航)

```bash
cd blue_whale_api/docs/db-schema
pip install mkdocs-material      # 一次
mkdocs serve --dev-addr 127.0.0.1:8000   # 浏览器开 http://127.0.0.1:8000
```

## DB 结构变更后重新生成结构

```bash
brew install tbls
# DSN 走环境变量,勿硬编码密码
export TBLS_DSN='mysql://root:PWD@115.190.244.91:3306/new-api'
tbls doc "$TBLS_DSN" -c .tbls.yml   # .tbls.yml: docPath + comments(业务字典)
```

⚠️ **tbls 会覆盖结构部分**。13 张增强表的 md 含手写业务段落(公式/易错点/关联),全量重生成会冲掉 —— 建议结构变更不频繁时手动同步,或把业务字典迁进 `.tbls.yml` 的 `comments:`(重生成不丢)。

## 部署给团队(可选)

```bash
mkdocs build          # 生成 site/ 静态站
# 测试服 nginx 托管 site/,开端口(如 http://244.91:8001)
```

不是 API 接口,是静态网站 URL —— 人浏览器看,AI 读 md 源码。

## 维护

- 业务字典(公式/易错点)是手写知识,改表后记得更新对应 md
- 新表加入:tbls 重生成结构 + 补 md
- 数据源:测试库 `115.190.244.91/new-api`(31 表快照,2026-07-28)

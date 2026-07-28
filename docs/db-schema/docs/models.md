# models — 模型展示元数据

!!! info "模块 / 定位"
    **模块:** 渠道与模型

    **定位:** 模型的**展示属性**(图标/描述/厂商/匹配规则)。**不含价格** —— 价格在 `options.ModelRatio`。

    **关联:** 厂商 `vendors`(vendor_id)｜价格 `options`(通过模型名隐式)

## 表结构

| 列名 | 含义 |
|---|---|
| `model_name` | 模型名(唯一) |
| `description` | 描述(展示用) |
| `icon` | 图标 |
| `tags` | 标签 |
| `vendor_id` | 厂商ID → vendors |
| `endpoints` | 支持的端点(chat/image/video…) |
| `name_rule` | 名称匹配规则:0=精确,前缀/后缀/包含 |
| `sync_official` | 是否同步官方元数据 |

!!! warning "易错点"
    - **改这里只影响展示,不影响计费**。价格在 options,别来这找
    - `name_rule` 影响 ModelRatio/ModelPrice 匹配:非精确规则按前缀/后缀/包含 fallback
    - 模型能不能用看 `abilities`,不是这张表

## 关联表

- `vendors`:vendor_id
- `options`:价格(通过模型名隐式,无外键)
- `abilities`:模型可用性

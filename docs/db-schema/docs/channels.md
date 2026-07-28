# channels — 渠道(上游供应商)

!!! info "模块 / 定位"
    **模块:** 渠道与模型

    **定位:** 上游 AI 供应商接入配置 —— 凭证/地址/模型/分组/路由权重。**决定请求往哪发。**

    **关联:** 模型路由在 `abilities`(本表自动生成)｜价格在 `options`(全局,与渠道无关)｜消费按 channel_id 进 `logs`

## 关键列

| 列名 | 含义 |
|---|---|
| `type` | 渠道类型(1=OpenAI, 3=Azure, 33=AWS, 42=火山方舟…)—— **决定走哪个 adapter** |
| `key` | **上游 API key(凭证,敏感)** |
| `base_url` | 上游地址 |
| `models` | 该渠道支持的模型列表(逗号分隔)—— **abilities 的源头** |
| `group` | 服务的分组(default/vip/svip) |
| `model_mapping` | 模型名映射(请求名→上游实际名),原生支持 |
| `priority` / `weight` | 路由优先级/权重(同模型多渠道选哪个) |
| `status` | 1=启用,其他=禁用 |
| `used_quota` | 该渠道累计消耗 |
| `balance` | 上游余额(定时查询) |
| `param_override`/`header_override` | 请求参数/Header 覆盖 |
| `auto_ban` | 出错自动禁用 |

## 业务规则

```text
加渠道(后台)→ 写 channels + 自动展开 abilities(channels.models × group)
请求 → 按 model+group 找 enabled abilities → priority 高 → weight → 随机 选 channel → 转发
```

!!! warning "易错点"
    - `key` 是上游凭证,**别泄露/别提交**
    - 改 `models` 要触发 abilities 重新同步,否则新模型不可用
    - `type` 配错 → 请求格式不对、报错
    - **价格与本表无关**:同模型多渠道,价格全局唯一(在 options)

## 关联表

- `abilities`:由 `models`×`group` 自动生成的路由索引
- `options`:价格(全局,不挂渠道)
- `logs`:消费记录含 channel_id

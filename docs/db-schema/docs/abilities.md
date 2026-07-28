# abilities — 模型路由表

!!! info "模块 / 定位"
    **模块:** 渠道与模型

    **定位:** 模型↔渠道↔分组 的路由索引。**决定模型能不能用、走哪个渠道。**

    **关联:** 来源 `channels`(channel_id)｜服务对象 `users`(group)

## 表结构

| 列名 | 类型 | 键 | 含义 |
|---|---|---|---|
| `group` | varchar(64) | **PRI** | 分组(default/vip/svip) |
| `model` | varchar(255) | **PRI** | 模型名 |
| `channel_id` | bigint | **PRI** | 渠道ID → channels |
| `enabled` | tinyint(1) | — | 是否启用 |
| `priority` | bigint | MUL | 路由优先级(大优先) |
| `weight` | bigint unsigned | MUL | 同优先级下的权重 |
| `tag` | varchar(191) | MUL | 标签(批量操作用) |

> **联合主键 (group, model, channel_id)**:一个模型在一个分组通过一个渠道 = 一条记录。

## 业务规则

```text
加/改渠道 → 后端 AddAbilities/UpdateAbilities 自动生成(不用手填)
请求 → 按 model + user.group 找 enabled abilities → priority 高 → weight → 随机 选 channel
```

!!! warning "易错点"
    - **不要手动改这张表**,它由 channels 自动同步
    - 模型报「无可用渠道」= abilities 没匹配项 → 查 channels.models 是否含该模型、group 对不对、enabled 开没开
    - 渠道删除要连带清 abilities(DeleteAbilities),否则残留幽灵路由

## 关联表

- `channels`:channel_id 来源
- `users`:用户 group 决定查哪批 abilities
- `options`:价格全局,与本表无关

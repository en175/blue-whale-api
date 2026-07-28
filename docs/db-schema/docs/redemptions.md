# redemptions — 兑换码

!!! info "模块 / 定位"
    **模块:** 计费与财务

    **定位:** 兑换码(输入码 → 加 quota)。管理员创建,用户兑换。

## 关键列

| 列名 | 含义 |
|---|---|
| `key` | 兑换码(char32,唯一) |
| `quota` | 兑换可得 quota 数 |
| `status` | 未用/已用 |
| `user_id` | 创建者(管理员) |
| `used_user_id` | 使用者 |
| `expired_time` | 过期时间 |

## 业务规则

```text
管理员建码(key+quota)→ 用户输入 → 填 used_user_id + users.quota += quota + status=已用
```

## 关联表

- `users`:创建者 user_id + 使用者 used_user_id

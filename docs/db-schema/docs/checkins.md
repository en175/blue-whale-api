# checkins — 签到

!!! info "模块 / 定位"
    **模块:** 计费与财务

    **定位:** 每日签到送 quota。

## 关键列

| 列名 | 含义 |
|---|---|
| `user_id` | 用户 |
| `checkin_date` | 签到日期(varchar10,如 2026-07-28) |
| `quota_awarded` | 奖励 quota 数 |

## 业务规则

- 每日签到 → `quota_awarded` 加到 `users.quota`
- `(user_id, checkin_date)` 防重复签到

## 关联表

- `users`:签到加 quota

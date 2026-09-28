# blue-whale-ai 对接文档：账号可登录系统白名单（X-Client-System）

> 本网关新增「账号可登录系统」登录白名单（`users.allowed_systems`）。
> 蓝鲸 BFF（blue-whale-ai）对接只需改 **一处**：所有出站 new-api 请求带上来源头。

---

## 1. 背景

blue-whale-ai 与 new-api 控制台（web/default）共用同一套账号体系（`users` 表）。
需求：控制某个账号允许登录哪个系统（蓝鲸 `bluewhale` / 控制台 `api`），或两者都允许。

网关在**登录**时按请求头 `X-Client-System` 判断来源系统，账号白名单不含该来源则拒绝登录。
**只拦登录，不拦 API 调用**；已登录会话不追溯。

## 2. 唯一必需改动（blue-whale-ai）

> 以下改动属 **blue-whale-ai 仓库**（不是本网关仓库），需在 blue-whale-ai 侧开 worktree 提交。

文件：`packages/core/src/new-api-transport.ts` → `performRequest()`（统一出站入口，
login/register/usage/identity/充值 全部走它），在 X-Trace-Id 注入块之后加：

```ts
// 出站统一声明客户端系统，登录白名单据此识别蓝鲸来源。
if (!headers.has("X-Client-System")) {
  headers.set("X-Client-System", "bluewhale");
}
```

所有经 BFF 发起的 new-api 请求自动带 `X-Client-System: bluewhale`，
网关侧登录/注册默认值/白名单校验全部对齐，无需逐个接口改。

## 3. 网关侧行为（供排查，blue-whale-ai 无需改动业务逻辑）

| 场景 | 网关行为 |
| --- | --- |
| 蓝鲸登录（带 `X-Client-System: bluewhale`） | 校验账号 `allowed_systems` 含 `bluewhale`，否则 HTTP 200 + `success:false` + `"该账号未被授权登录当前系统"` |
| 控制台登录（无头 / 未知头） | 视作 `api`，校验含 `api` |
| 空白名单（存量账号） | 全放行，双系统可登录 |
| 新注册（带 bluewhale 头） | 默认 `allowed_systems = bluewhale`（不信任请求体字段） |
| 新注册（无头） | 默认 `api` |
| root 账号 | 强制含 `api`，后台不可移除 |

### 登录响应（/api/user/login）新增字段

```json
{ "success": true, "data": { "allowed_systems": "bluewhale", "role": 1, ... } }
```

`allowed_systems`：逗号分隔字符串；空串 = 全放行。

### 错误透传提示

BFF `auth.controller.ts` login catch 的 200 分支建议透传 Go 文案（与 register 对齐）：

```ts
if (error instanceof NewApiError && error.status === 200) {
  throw new NewApiError(error.message, 401, error.code); // 不再硬编码「登录凭证无效」
}
```

否则蓝鲸用户被白名单拒绝时只看到笼统的「登录凭证无效」。

## 4. 测试 curl

```bash
# 蓝鲸来源登录（账号白名单需含 bluewhale）
curl -X POST http://<gateway>/api/user/login \
  -H 'Content-Type: application/json' \
  -H 'X-Client-System: bluewhale' \
  -d '{"username":"u","password":"p"}'

# 控制台来源（不带头，默认 api）
curl -X POST http://<gateway>/api/user/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"u","password":"p"}'
```

## 5. 不变量（blue-whale-ai 侧无需处理）

- 不追溯已登录会话（BFF 的 Redis 会话、`/api/me` 均不动）。
- API 调用（sk- token）不受白名单影响。
- OAuth 回调是浏览器重定向，天然无此头，网关默认按 `api` 处理（OAuth/WeChat 新注册默认 `api`）。
- 2FA 账号：白名单校验在 TOTP 验证通过之后（`setupLogin` 出口）——凭据+2FA 先验完再查白名单，无账号探测风险；代价是需多输一次验证码才被拒。

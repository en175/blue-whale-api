# 海外/国内 API 文档站合并 · 设计文档

- **日期**: 2026-08-27
- **仓库**: `blue-whale-api`（`https://gitee.com/hangzhou-xuanfu_1/blue-whale-api.git`）
- **范围**: `web/api-docs`（仅前端内容与少量组件）+ DNS / nginx / 阿里云证书（运维 ticket，不在本仓）
- **状态**: 已批准（用户确认方案 A：直接合并 + 海外 301 兜底）

## 背景

### 现状

| 维度 | 国内 | 海外 |
|---|---|---|
| 入口 | `http://115.190.244.91:3011/zh/docs`（无域名） | `https://docs.api.yunjizi.cn/`（→ `43.156.28.30`） |
| 协议 | HTTP | HTTP 通、HTTPS 卡在 Let's Encrypt DNSSEC |
| 模型前缀 | `doubao-seedance-*`（方舟） | `dreamina-seedance-*`（BytePlus） |
| 内容差异 | — | 仅模型清单前缀不同（其余内容一致） |

海外机 `43.156.28.30`（`api.yunjizi.cn`）同时承担主网关，**不能整机退**，只下文档站容器。

### 痛点

1. 海外访问国内 IP 慢（要走 CN 出口），但反过来访问海外机反而更稳。
2. 海外文档站 HTTPS 卡 LE 签名，海外用户只能走 HTTP，体验差。
3. 两套部署增加维护成本（同步、监控、证书续期各自一套）。
4. 现有 `web/api-docs/src/components/live-models.tsx` 的 `STATIC_MODELS` 是同一个数组给 `/zh` `/en` 共用，无法体现 dreamina vs doubao 的渠道差异。

## 目标

1. 把海外域名 `docs.api.yunjizi.cn` 解析到国内 IP `115.190.244.91`，实现单实例对外。
2. `/zh` 仅展示方舟前缀模型，`/en` 仅展示 BytePlus 前缀模型。
3. 海外 `43.156.28.30` 的文档容器下线，但保留一个 nginx 301 把流量永久指向新 IP，兼顾 SEO 收口与切换期容错。
4. HTTPS 在合并前签发阿里云 DV 证书并在国内 nginx 部署，合并即可全 HTTPS。

## 非目标

- 不重写 `web/api-docs` 路由框架、不动 `[lang]/` 结构。
- 不动 `web/default/`（受 AGENTS.md 保护）。
- 不改主网关 `api.yunjizi.cn` 的任何配置。
- 不重新签约或迁移域名（保留 `docs.api.yunjizi.cn`）。
- 不合并/同步 `web/api-docs/content/docs/{zh,en}/api/ai-model/...` 下的 OpenAPI 自动生成内容（保持双份现状，仅修改视频/模型相关页面）。
- 不动价格、计费、SDK、控制台相关文案与代码。
- 不实现自动按 IP/Accept-Language 切换语言；语言由 URL 前缀 + 顶部切换器决定。

## 决策

| # | 决策 | 原因 |
|---|---|---|
| D1 | 唯一实例部署在国内 `115.190.244.91:3011` | 海外访问国内 IP 慢的问题由香港/海外用户改走云厂商加速（本次不解决）换来运维简化、HTTPS 可用、证书可控 |
| D2 | 默认中文(/zh),保留 /en 路由作为可选项 | 用户最后指令:统一中文,语言不再作为分流依据 |
| D3 | 模型清单同一页内分组:`国内可用(方舟渠道)` + `海外可用(BytePlus 渠道)`;每条带 `region` 字段 | 用户最后指令:不要按语言区分,只在列表内部区分;`/zh` `/en` 模型页都展示同一份组合列表 |
| D4 | DNS 直接切换 + 海外 301 兜底 | 不做 GeoDNS / 双栈灰度，复杂度与收益不匹配 |
| D5 | 海外 nginx 改为 `return 301 https://docs.api.yunjizi.cn$request_uri` | 切换期容错 + 长期 SEO 收口 + 旧链接兼容 |
| D6 | HTTPS 证书走阿里云免费 DV（域名已在阿里云，自动 DNS 验证） | LE 已知不可用（DNSSEC 校验失败） |
| D7 | `/en` 提交页 curl 示例的 `model` 字段改为 `dreamina-seedance-2-0-260128`;`BASE_URL` 注释保持 `https://api.yunjizi.cn`(API 域,不是文档域)不变 | `/en` 端海外用户调 API 走 `api.yunjizi.cn` 主域,与本次合并无关;`BASE_URL` 现有注释已正确 |
| D8 | 海外 301 长期保留 | 海外主网关（`api.yunjizi.cn`）仍需运行；301 配置独立于主网关 |
| D9 | 海外文档容器与镜像在切换稳定后再清理 | 切换期出问题可立即 `docker start` 恢复 |

## 架构

```
                        ┌─────────────────────────────┐
   浏览器 ─── DNS ────► │ 阿里云 DNS: docs.api.yunjizi.cn│
                        │         → 115.190.244.91     │
                        └──────────────┬──────────────┘
                                       ▼
┌──────────────────────────────────────────────────────────┐
│ 国内 115.190.244.91                                      │
│  nginx 443 (TLS, 阿里云 DV 证书)                         │
│    └─► 127.0.0.1:3011 (docker container, --restart always)│
│           └─► Next.js 16 + Fumadocs                       │
│                  ├─ /zh/* → STATIC_MODELS_ZH (doubao-)    │
│                  └─ /en/* → STATIC_MODELS_EN (dreamina-)  │
└──────────────────────────────────────────────────────────┘

            兜底 / 旧链 (长期保留)
┌──────────────────────────────────────────────────────────┐
│ 海外 43.156.28.30                                        │
│  nginx: /etc/nginx/conf.d/docs.conf                       │
│    server {                                               │
│      listen 80; server_name docs.api.yunjizi.cn;          │
│      return 301 https://docs.api.yunjizi.cn$request_uri;  │
│    }                                                      │
│  ※ docker 容器已 stop + rm,镜像已 rmi                     │
└──────────────────────────────────────────────────────────┘
```

## 组件

### 代码侧（本仓 PR 范围）

| 文件 | 改动 |
|---|---|
| `web/api-docs/src/components/live-models.tsx` | 拆出 `STATIC_MODELS_ZH`（doubao-）与 `STATIC_MODELS_EN`（dreamina-）；`LiveModels` 按 `lang` 选择数组；`L[lang].empty` 文案不变 |
| `web/api-docs/content/docs/en/models.mdx` | `description` 改为 `Models available on overseas (BYTEPLUS) channel`；`title` 保持 `Models` |
| `web/api-docs/content/docs/en/video/submit.mdx` | 5 个 curl 示例的 `model` 字段统一改为 `dreamina-seedance-2-0-260128`；`BASE_URL` 注释保持 `https://api.yunjizi.cn`（已正确，不动） |
| `web/api-docs/content/docs/en/video/query.mdx` | 检查 `model` 提及位置，同步替换（如有） |
| `web/api-docs/content/docs/en/video/download.mdx` | 检查并保持一致（一般无 `model` 字段） |
| `web/api-docs/content/docs/en/quickstart.mdx` | 不改（`BASE_URL` 注释已正确） |
| `web/api-docs/content/docs/zh/video/submit.mdx` | `BASE_URL` 注释保持 `https://yunjizi.cn`（不动） |

### 非代码侧（运维 ticket，不在本仓 PR 范围）

| # | 动作 | 责任方 | 前置 |
|---|---|---|---|
| O1 | 阿里云 SSL 证书控制台创建 DV 证书（`docs.api.yunjizi.cn`） | 用户 | — |
| O2 | 阿里云 DNS 控制台把 A 记录从 `43.156.28.30` 改为 `115.190.244.91` | 用户 | O1+O3+代码 PR 合并+国内 3011 smoke |
| O3 | 国内 nginx 80→3011、443→3011 部署（含阿里云证书） | 实施 | O1 |
| O4 | 海外 `43.156.28.30` `nginx/conf.d/docs.conf` 改为 301 配置 | 实施 | O2 同批执行可 |
| O5 | Playwright 跨洋 E2E 验证 | 实施 | O2+O3+O4 |
| O6 | 海外文档容器 `docker stop && docker rm blue-whale-api-docs` | 实施 | O5 通过 |
| O7 | 海外镜像 `docker rmi blue-whale-api-docs:latest` | 实施 | O6 |
| O8 | 阿里云证书到期前 2 周续期 | 用户 | 长期（90 天） |

## 数据流

```
Browser GET https://docs.api.yunjizi.cn/{en,zh,}/...
  → 阿里云 DNS 解析 → 115.190.244.91
  → 国内 nginx 443（证书终结,反代 127.0.0.1:3011）
  → Docker 容器（Next.js standalone, port 3011）
  → /[lang] 路由匹配（已有 Fumadocs 多语言源）
  → MDX 内容 / 组件渲染
  → /zh/models → STATIC_MODELS_ZH（doubao-）
  → /en/models → STATIC_MODELS_EN（dreamina-）
  → 返回 HTML

失败分支：
  - 证书失效 → 浏览器展示「您的连接不是私密连接」
  - 容器挂掉 → nginx 502 → 用户报错（docker --restart always 会自动拉起）
  - 旧链接命中海外 IP（DNS 切换期 / 缓存） → 海外 301 → 跳回新 IP
```

## 错误处理

| 场景 | 行为 | 备注 |
|---|---|---|
| 阿里云证书过期 | 浏览器 TLS 握手失败 | 90 天免费,需提前 2 周续期（O8） |
| 国内容器崩溃 | nginx 502 | `docker --restart always` 自动拉起；监控告警单独 ticket |
| 阿里云 DNS 解析失败 | 浏览器 DNS 错误 | 海外 301 兜底不解决 DNS 解析失败，只解决"命中旧 IP"场景 |
| 用户访问 `/en/models` 看到 doubao- 前缀 | 测试遗漏 | E2E 必须断言模型 ID 字符串存在性 |
| 旧链接 `/api/ai-model/audio/...` | 仍可访问 | Fumadocs 默认路由；本次不动 |
| 海外用户访问 `http://docs.api.yunjizi.cn` | 走海外 301 → 跳回新 IP 的 HTTPS | 切换期与长期都生效 |
| `/en/video/submit` 示例使用 `dreamina-seedance-2-0-260128` 实际在国内环境调不通 | 文档与网关语义错位 | E2E 不做 API 真实提交；只做字符串存在性断言；实际可用性由网关测试覆盖（不在本设计） |

## 测试

### PR 合并前（CI）

- `web/api-docs` build / typecheck / format 通过
- 单元测试：暂无（`live-models.tsx` 改动小,沿用现有结构）
- 字符串检查：新增 `tests/unit/model-list-by-lang.test.ts`，断言 `STATIC_MODELS_ZH` 全是 `doubao-` 前缀、`STATIC_MODELS_EN` 全是 `dreamina-` 前缀、两个数组无交集

### 国内 smoke（PR 合并后、O2 之前）

- `curl -I http://115.190.244.91:3011/zh` → 200
- `curl -I http://115.190.244.91:3011/en` → 200
- `curl -I http://115.190.244.91:3011/en/models` → 200
- `curl http://115.190.244.91:3011/en/models` → HTML 含 `dreamina-seedance-2-0-260128` 且不含 `doubao-seedance-2-0-260128`
- `curl http://115.190.244.91:3011/zh/models` → HTML 含 `doubao-seedance-2-0-260128` 且不含 `dreamina-seedance-2-0-260128`

### 跨洋 E2E（O5，DNS 切换后）

Playwright 测试，部署在能同时访问国内外的网络环境（如公司 VPN 出口 + Cloudflare WARP 双栈）：

| 用例 | 路径 | 断言 |
|---|---|---|
| `/` 跳转 | `https://docs.api.yunjizi.cn/` | 30x 到 `/zh` 或 `/en`（任一即可） |
| `/zh` 首页 | `https://docs.api.yunjizi.cn/zh` | 200，HTML 含 `模型` |
| `/en` 首页 | `https://docs.api.yunjizi.cn/en` | 200，HTML 含 `Models` |
| `/zh/models` 内容 | `/zh/models` | 含 `doubao-seedance-2-0-260128` |
| `/en/models` 内容 | `/en/models` | 含 `dreamina-seedance-2-0-260128` |
| `/en/video/submit` 示例 | `/en/video/submit` | 含 `dreamina-seedance-2-0-260128` 与 `https://api.yunjizi.cn` BASE_URL 注释 |
| HTTP → HTTPS | `http://docs.api.yunjizi.cn` | 301/302 到 `https://...` |
| HSTS | 任意 HTTPS 路径 | 响应头含 `strict-transport-security` |
| 旧链 301 | 命中海外 IP 后的 `/en` | 海外 nginx 返 301 → 跳回新 IP |
| 旧文档站 404 | `https://docs.api.yunjizi.cn/api/ai-model/audio/openai/createspeech` | 仍 200（保留 OpenAPI 自动生成内容） |

## 发布顺序

1. 本仓 PR 创建（含代码侧全部改动 + 单元测试）。
2. 用户合并 PR → 火山引擎流水线触发新镜像构建。
3. 国内 `115.190.244.91` 拉新镜像重启容器 → 国内 smoke（O1 前置）。
4. O1 阿里云证书签发。
5. O3 国内 nginx 443 部署（白天低峰 / 提前通知）。
6. O4 海外 nginx 301 配置先行（此时 DNS 还未改，海外 301 自指 443,等于本地无害）。
7. O2 阿里云 DNS A 记录切换。
8. O5 跨洋 E2E 验证。
9. O6+O7 海外容器与镜像清理。
10. 写入「蓝鲸与网关环境变量总览.md」第 7.2 节,补记海外 301 兜底配置（之前记录的"SSH 被拒"已过时）。

## 回滚

| 场景 | 回滚动作 | 耗时 |
|---|---|---|
| 国内容器/服务故障 | 阿里云 DNS 改回 `43.156.28.30`；海外 `docker start blue-whale-api-docs`；海外 nginx 301 临时改为 `return 301 https://43.156.28.30:3011$request_uri`（仅故障期） | 1-2 分钟（DNS TTL） + 30 秒（容器） |
| 代码 bug | `git revert` PR 合并提交 → 流水线重建 | 流水线一次（~12 分钟） |
| 证书签发失败 / 续期失败 | 国内 80 端口继续工作（HTTP），DNS 不变 | 0（兼容现状） |
| 海外 301 干扰 | 临时 disable 301 server 块，海外直接 404 | 即时 |

## 风险与依赖

| 风险 | 等级 | 缓解 |
|---|---|---|
| 阿里云 DV 证书签发失败（域名解析/验证环节异常） | 中 | 切前 3 天以上提前签发,留 buffer；签发失败时本次合并延期,HTTP 仍可用 |
| 国内 IP 海外访问慢 | 高（已知） | 本次不解决;海外用户首次访问新 IP 可能有 3-5s 等待（取决于云厂商出口） |
| 旧 OpenAPI 自动生成内容含 `doubao-` 字符串（与 /en 隔离） | 低 | /en 路径下不走 `ai-model/videos/sora` 等其他模型页（路由独立）;仅 `models.mdx` / `video/submit.mdx` 路径 |
| 海外 301 暴露新 IP 给攻击者 | 低 | 公开 IP,本就在 SSRF / 网络空间测绘里能看到 |
| AGENTS.md 保护规则与本设计冲突 | 无 | 本设计不修改 `web/default/` 或任何 new-api / QuantumNous 标识 |

## 验收

- [ ] PR 合并后,`/en/models` HTML 含 `dreamina-seedance-2-0-260128` 且不含 `doubao-seedance-2-0-260128`
- [ ] PR 合并后,`/zh/models` HTML 含 `doubao-seedance-2-0-260128` 且不含 `dreamina-seedance-2-0-260128`
- [ ] 国内 smoke 全通过（O1 前）
- [ ] HTTPS 证书在 `https://docs.api.yunjizi.cn/` 浏览器显示安全
- [ ] 跨洋 E2E 全通过
- [ ] 海外 `docker ps` 无 `blue-whale-api-docs` 容器
- [ ] 海外 `curl -I http://43.156.28.30/` 仍可达（主网关 `api.yunjizi.cn`）未受影响
- [ ] 「蓝鲸与网关环境变量总览.md」第 7.2 节更新

## 相关文档

- `docs/bluewhale-api-website-prd.md` — 对外官网与 API 文档需求（既有 PRD）
- `e:\IDEA_Programme\new\blue-whale-worktrees\fix-ikaxdt\docs\specs\2026-08-10-bluewhale-api-website-design.md` — 既有设计（D3 决策：仅展示 seedance-2.0）
- `e:\IDEA_Programme\new\blue-whale-worktrees\fix-ikaxdt\docs\specs\2026-08-10-bluewhale-api-website-implementation-plan.md` — 既有实施计划（5.2 决策:web/api-docs 受保护,本次不在该范围）
- `e:\IDEA_Programme\new\blue-whale-api\docs\superpowers\specs\2026-08-19-api-docs-build-optimization-design.md` — 既有镜像构建提速设计
- `e:\IDEA_Programme\new\todo\api-docs海外部署进度.md` — 海外部署现状备忘

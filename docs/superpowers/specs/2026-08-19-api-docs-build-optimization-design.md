# API 文档站镜像构建提速 · 设计文档

- **日期**: 2026-08-19
- **仓库**: `blue-whale-api`（`https://gitee.com/hangzhou-xuanfu_1/blue-whale-api.git`）
- **范围**: `web/api-docs`
- **状态**: 已批准（用户确认 A + B1 方案）

## 背景

api-docs 文档站通过火山引擎「镜像构建」流水线部署：每次 push 触发全量 git clone → Docker 构建 → 镜像推送，总耗时 **12 分钟以上**。镜像推送目标 `blue-whale-cn-beijing.cr.volces.com/blue-whale/api-docs:2.0.0`。

**约束（用户确认）**：
- 只能改动仓库内文件；火山流水线构建参数（`cacheType`/`cacheUrl`/`nydusify`）无权限修改 → `cacheType=default` 的构建层缓存每次冷启动，所有优化必须落到仓库侧。
- 每次 push 都全量构建，文档小改动也吃完整构建。

## 事实调查结论

| 项 | 现状 | 结论 |
|---|---|---|
| 构建上下文 | `web/api-docs` 约 74MB，其中 `public/assets` **65MB**（278 文件） | 上下文与镜像体积大头 |
| `public/assets` 引用 | 137 个 `/assets/` 引用路径（content+src），**130 个有真实文件**，7 个是外部 GitHub URL 的误报 | 130 个在用，不可删 |
| 孤儿资产 | `public/assets` 中 **148 文件（42MB）未被任何页面引用** | 可安全删除 |
| bun 安装 | Dockerfile `RUN npm install -g bun` 走 npmjs.org | 国内构建机可能慢/卡 |
| prebuild 网络 | `build-changelog.ts` 请求 `api.github.com`（无超时）；`generate-docs-from-gateway.ts` 请求 `GATEWAY_URL`（无超时） | 失败时长时间挂起，是耗时波动的最大嫌疑 |
| WebP 工具链 | `sharp@0.34.5` 已在 bun.lock（`next` 可选依赖，全平台二进制） | 无需新增依赖 |
| 死链 | 早期交叉核对标出的 7 个"死链"实为 changelog 里 GitHub release notes 自带的 `https://github.com/user-attachments/assets/<uuid>` 外链 | **本地无死链，无需清理** |

## 范围

做：**A1 + A2 + A3 + B1**
跳过：**A4**（`NEXT_WORKERS` 不动，留待实测）、**B2**（死链为误报）、**C**（oven/bun 基础镜像、拆仓、图片挪 TOS）

## 改动设计

### A1 — 删除孤儿 assets（约 42MB / 148 文件）

新增一次性脚本 `web/api-docs/scripts/cleanup-orphan-assets.ts`：

1. 从 `content/` + `src/` 提取所有 `/assets/<path>` 引用路径（正则排除 `github.com/user-attachments/` 外链）。
2. 遍历 `public/assets` 下全部文件，删除不在引用集合中的文件。
3. 输出：删除文件清单 + 释放字节数 + 删除后剩余文件数。

**验证**：删除后重跑交叉核对，`public/assets` 无孤儿；`bun run build` 通过。

### A2 — Dockerfile bun 安装加速

`web/api-docs/Dockerfile` 第 7 行：

```dockerfile
RUN npm install -g bun --registry=https://registry.npmmirror.com
```

### A3 — prebuild 网络 fetch 加超时

两个文件的两个 fetch 加 `signal: AbortSignal.timeout(10_000)`：

- `web/api-docs/scripts/build-changelog.ts`：`fetchGitHubReleases()` 内对 `api.github.com/repos/QuantumNous/new-api/releases` 的请求。
- `web/api-docs/scripts/generate-docs-from-gateway.ts`：`fetchDocsConfig()` 内对 `GATEWAY_URL/api/docs/config` 的请求。

超时后走现有 catch 分支：changelog 用仓库内已有旧文件兜底、site-config 用静态内容，构建继续。**避免无超时 fetch 在火山网络环境长时间挂起。**

### B1 — 被引用图片 WebP 化（约 23MB → ~6MB）

新增一次性脚本 `web/api-docs/scripts/optimize-images.ts`：

1. 提取所有被引用的 `/assets/*.png|.jpg|.jpeg` 路径（同上引用提取逻辑）。
2. 用 `sharp`（node_modules 已有）将每个 PNG/JPG 转 WebP，质量 **80**，写为同路径 `.webp`。
3. 在 26 个引用的 `.mdx` 文件中把 `.png/.jpg/.jpeg → .webp`（仅针对已转换的路径）。
4. 删除已转换的 PNG/JPG 原图。

SVG 与已为 `.webp` 的文件原样保留。

**验证**：
- 全部 `/assets/` 引用指向存在的文件（重跑交叉核对 = 0 缺失）。
- `bun run prebuild` + `bun run build` 本地通过。
- git diff 统计：`public/assets` 体积 65MB → ~6MB。

## 执行顺序

1. 建 task worktree（已完成：`blue-whale-worktrees/api-docs-build-opt`，分支 `feature/api-docs-build-opt`）。
2. 写设计文档（本文档）。
3. B1 先做（转换 + 改引用），A1 后做收尾（孤儿清理，把转换产生的旧图一并归入）。
4. A2 / A3 为独立小改动，随同一提交。
5. 本地验证（引用核验 + 构建）。
6. 提交 → PR（走 gitee MCP），每步经用户确认。

## 预期收益

- 镜像构建上下文 74MB → ~7MB，`COPY . .`、镜像层与推送显著变快。
- prebuild 不再被无超时网络请求拖住，构建时长波动收敛。
- 未来浅克隆体积同步大降（仓库工作树减少约 59MB 二进制资产）。
- 综合预期：**12min → ~5min 量级**（待实测确认）。

## 不做（后续可选）

- A4：`NEXT_WORKERS=16` → 对齐构建机核数（需实测火山构建机配置后决定）。
- C1：换 `oven/bun` 基础镜像（需验证含 Node，有兼容风险）。
- C2：api-docs 拆独立仓库（根治全仓 clone）。
- C3：图片挪 TOS/OSS（架构级，需确认运行环境可达性）。

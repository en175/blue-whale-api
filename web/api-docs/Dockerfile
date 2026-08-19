# Stage 1: Build
FROM node:22-alpine AS builder

WORKDIR /build

# Install bun (project convention from blue_whale_api/Dockerfile)
RUN npm install -g bun --registry=https://registry.npmmirror.com

# Install deps (skip postinstall — fumadocs-mdx needs source files not yet present)
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile --ignore-scripts

# Copy source
COPY . .

# Gateway URL for prebuild docs generation (optional; static fallback if unset)
ARG GATEWAY_URL
ENV GATEWAY_URL=$GATEWAY_URL

# Public gateway URL for client-side LiveModels fetch (required for live model list)
ARG NEXT_PUBLIC_API_BASE_URL
ENV NEXT_PUBLIC_API_BASE_URL=$NEXT_PUBLIC_API_BASE_URL

# Generate changelog from GitHub API (fails gracefully)
RUN bun run prebuild:run

# Generate fumadocs .source/ index (was postinstall, now explicit)
RUN npx fumadocs-mdx

# Build Next.js standalone output
# taskset 限核 0-7：规避构建机报告核数 > 实际配额导致的 Turbopack 线程超订抖动
# (node:22-alpine busybox 无 taskset，先装 util-linux；失败在编译前快速报出)
# Retry on transient network errors (font downloads, socket resets)
RUN apk add --no-cache util-linux >/dev/null && \
    taskset -c 0-7 sh -c \
    'for i in 1 2 3; do bun run build && exit 0; echo "Build attempt $i failed, retrying..."; sleep 10; done; exit 1'


# Stage 2: Runtime
FROM node:22-alpine

WORKDIR /app

ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3011

# Next.js standalone: server.js + trimmed node_modules
COPY --from=builder /build/.next/standalone ./

# Static assets (JS/CSS chunks)
COPY --from=builder /build/.next/static ./.next/static

# Public directory (favicon, robots.txt, images)
COPY --from=builder /build/public ./public

EXPOSE 3011

CMD ["node", "server.js"]
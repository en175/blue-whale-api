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
# .next 走 tmpfs（内存 I/O，规避 buildkit overlayfs 慢路径）；产物在 RUN 内拷回持久层
# Retry on transient network errors (font downloads, socket resets)
RUN --mount=type=tmpfs,target=/build/.next \
    for i in 1 2 3; do \
      if bun run build; then cp -a /build/.next /build/.next-persist && exit 0; fi; \
      echo "Build attempt $i failed, retrying..."; \
      sleep 10; \
    done; exit 1


# Stage 2: Runtime
FROM node:22-alpine

WORKDIR /app

ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3011

# Next.js standalone: server.js + trimmed node_modules
COPY --from=builder /build/.next-persist/standalone ./

# Static assets (JS/CSS chunks)
COPY --from=builder /build/.next-persist/static ./.next/static

# Public directory (favicon, robots.txt, images)
COPY --from=builder /build/public ./public

EXPOSE 3011

CMD ["node", "server.js"]
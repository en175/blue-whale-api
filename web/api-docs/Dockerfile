# Stage 1: Build
FROM node:22-alpine AS builder

WORKDIR /build

# Install bun (project convention from blue_whale_api/Dockerfile)
RUN npm install -g bun

# Install deps (skip postinstall — fumadocs-mdx needs source files not yet present)
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile --ignore-scripts

# Copy source
COPY . .

# Generate fumadocs .source/ index (was postinstall, now explicit)
RUN npx fumadocs-mdx

# Generate changelog from GitHub API (fails gracefully)
RUN bun run prebuild

# Build Next.js standalone output
RUN bun run build


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
# syntax=docker/dockerfile:1.7
# Production image for @novanas/ui — vite build → nginx static serve on :8080.
# Build context MUST be the repo root (workspace install).

# ---- deps ------------------------------------------------------------------
FROM node:22-bookworm-slim AS deps
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@9.12.0 --activate

COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY tsconfig.base.json ./
COPY packages/schemas/package.json packages/schemas/
COPY packages/ui/package.json packages/ui/

RUN --mount=type=cache,id=pnpm,target=/pnpm/store \
    pnpm config set store-dir /pnpm/store && \
    pnpm install --frozen-lockfile --filter @novanas/ui...

# ---- build -----------------------------------------------------------------
FROM deps AS build
WORKDIR /app
COPY packages/schemas packages/schemas
COPY packages/ui packages/ui

RUN pnpm --filter @novanas/schemas build \
 && pnpm --filter @novanas/ui build

# ---- runtime ---------------------------------------------------------------
FROM nginxinc/nginx-unprivileged:1.27-alpine AS runtime
COPY --from=build /app/packages/ui/dist /usr/share/nginx/html
# Default upstream config listens on 8080 and serves /usr/share/nginx/html.
# Override default.conf to add SPA-style fallback to index.html.
RUN printf 'server {\n  listen 8080;\n  server_name _;\n  root /usr/share/nginx/html;\n  index index.html;\n  location / { try_files $uri $uri/ /index.html; }\n}\n' \
    > /etc/nginx/conf.d/default.conf
EXPOSE 8080
USER 101

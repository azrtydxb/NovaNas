# syntax=docker/dockerfile:1.7

# ----- deps -----
FROM node:22-bookworm-slim AS deps
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@9.12.0 --activate

COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY tsconfig.base.json ./
COPY packages/schemas/package.json packages/schemas/
COPY packages/db/package.json packages/db/
COPY packages/api/package.json packages/api/

RUN --mount=type=cache,id=pnpm,target=/pnpm/store \
    pnpm config set store-dir /pnpm/store && \
    pnpm install --frozen-lockfile

# ----- build -----
FROM deps AS build
WORKDIR /app
COPY packages/schemas packages/schemas
COPY packages/db packages/db
COPY packages/api packages/api

RUN rm -f packages/schemas/tsconfig.tsbuildinfo \
         packages/db/tsconfig.tsbuildinfo \
         packages/api/tsconfig.tsbuildinfo
RUN pnpm --filter @novanas/schemas build \
 && pnpm --filter @novanas/db build \
 && pnpm --filter @novanas/api run build:bundle

# Materialize a self-contained app dir with production deps only.
# `pnpm deploy` resolves workspace + symlinked deps into a real
# node_modules tree that we can ship without copying the workspace.
RUN --mount=type=cache,id=pnpm,target=/pnpm/store \
    pnpm config set store-dir /pnpm/store && \
    pnpm --filter @novanas/api deploy --prod /deploy

# pnpm deploy does NOT include the workspace package's `dist/` output —
# only the source per the `files` field. Re-copy the freshly-built dist
# trees over the deploy tree so Node can resolve @novanas/schemas/dist
# and @novanas/db/dist at runtime.
RUN cp -r packages/schemas/dist /deploy/node_modules/@novanas/schemas/dist && \
    cp -r packages/db/dist      /deploy/node_modules/@novanas/db/dist

# ----- runtime -----
FROM gcr.io/distroless/nodejs22-debian12:nonroot AS runtime
WORKDIR /app
ENV NODE_ENV=production \
    PORT=8080

COPY --from=build --chown=nonroot:nonroot /deploy/dist/bundle.js /app/api.js
COPY --from=build --chown=nonroot:nonroot /deploy/node_modules /app/node_modules
# Drizzle migrations applied on api startup (#43). Ship the SQL files
# alongside the bundle; api/src/index.ts points the migrator at this
# path before serving any request.
COPY --from=build --chown=nonroot:nonroot /app/packages/db/migrations /app/migrations

USER nonroot:nonroot
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD ["/nodejs/bin/node", "-e", "fetch('http://127.0.0.1:8080/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"]

ENTRYPOINT ["/nodejs/bin/node", "/app/api.js"]

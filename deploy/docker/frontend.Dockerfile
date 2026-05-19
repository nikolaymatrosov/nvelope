# Build stage: install deps and produce the TanStack Start / Nitro server build.
# Build context is the repo root; the root .dockerignore excludes node_modules.
FROM node:24-slim AS build
ENV HUSKY=0
WORKDIR /app
RUN corepack enable
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

# Runtime stage: run the Nitro server output with plain Node.
FROM node:24-slim AS runtime
ENV NODE_ENV=production
ENV PORT=3000
WORKDIR /app
COPY --from=build /app/.output ./.output
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]

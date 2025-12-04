FROM node:24.11.1-bookworm-slim AS base

WORKDIR /app
ENV NODE_ENV=production \
    # Prevent Puppeteer from downloading Chromium during npm ci
    PUPPETEER_SKIP_DOWNLOAD=true

# ---------- Deps (dev) ----------
FROM base AS deps-dev
WORKDIR /app
ENV NODE_ENV=development

COPY package*.json ./
RUN npm ci

# ---------- Build ----------
FROM base AS build
WORKDIR /app
ENV NODE_ENV=development
COPY --from=deps-dev /app/node_modules ./node_modules
COPY . .
# Build TS -> dist
RUN npm run build

# ---------- Deps (prod only) ----------
FROM base AS deps-prod
WORKDIR /app
COPY package*.json ./
RUN npm ci --omit=dev

# ---------- Runtime ----------
FROM node:24.11.1-bookworm-slim AS runtime

# Install Chromium and minimal fonts/libs for Puppeteer
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
      chromium \
      fonts-liberation \
      tini \
      libnss3 \
      libatk-bridge2.0-0 \
      libdrm2 \
      libxkbcommon0 \
      libxcomposite1 \
      libxdamage1 \
      libxfixes3 \
      libxrandr2 \
      libgbm1 \
      libasound2 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
ENV NODE_ENV=production \
    # Use system Chromium installed above
    PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium \
    PUPPETEER_SKIP_DOWNLOAD=true \
    # Keep Chromium profile/cache on a tmpfs path that remains writable in read-only deployments
    XDG_CONFIG_HOME=/tmp/.chromium \
    XDG_CACHE_HOME=/tmp/.chromium \
    XDG_RUNTIME_DIR=/tmp/.runtime

# Create non-root user
RUN groupadd -r app && useradd -r -g app app

# Pre-create writable Chromium directories owned by the runtime user
RUN install -d -m 0700 -o app -g app /tmp/.chromium
RUN install -d -m 0700 -o app -g app /tmp/.runtime

# Copy production node_modules and built dist
COPY --chown=app:app --from=deps-prod /app/node_modules ./node_modules
COPY --chown=app:app --from=build /app/dist ./dist
COPY --chown=app:app package*.json ./
COPY --chown=app:app env_secrets_expand.sh /app/env_secrets_expand.sh

RUN chmod +x /app/env_secrets_expand.sh

USER app

# OCI labels (optionally overridden via build args)
ARG VERSION="dev"
ARG VCS_REF="unknown"
LABEL org.opencontainers.image.title="puppeteer-worker" \
      org.opencontainers.image.description="Headless Chromium worker for Merrymaker (Puppeteer)" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$VCS_REF

# Healthcheck: report healthy if node worker process is running
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD pgrep -f "dist/worker-main.js" >/dev/null || exit 1

# Default command runs the worker loop (override via CMD in deployment if needed)
ENTRYPOINT ["/usr/bin/tini", "-g", "--", "/bin/bash", "/app/env_secrets_expand.sh"]
CMD ["node", "dist/worker-main.js"]

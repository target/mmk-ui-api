# ---------- Builder ----------
FROM golang:1.25.4-bookworm AS builder


ENV CGO_ENABLED=0 \
    GOFLAGS=-trimpath

WORKDIR /src

# Leverage Docker layer cache: copy go.mod/go.sum first and download deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the rest of the source
COPY . ./

# Build the binaries (embed assets via go:embed)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -buildvcs=false -ldflags "-s -w" -o /out/merrymaker ./cmd/merrymaker

# ---------- Runtime ----------
FROM debian:bookworm-slim AS runtime

# Install minimal runtime deps and wget for HEALTHCHECK
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates wget \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r app && useradd -r -g app app

WORKDIR /app
COPY --chown=app:app --from=builder /out/merrymaker /app/merrymaker
COPY --chown=app:app env_secrets_expand.sh /app/env_secrets_expand.sh

RUN chmod +x /app/env_secrets_expand.sh

USER app
EXPOSE 8080

# OCI labels (optionally overridden via build args)
ARG VERSION="dev"
ARG VCS_REF="unknown"
LABEL org.opencontainers.image.title="merrymaker-go" \
      org.opencontainers.image.description="Merrymaker Go service" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.source="https://github.com/target/mmk-ui-api"

# Healthcheck: HTTP reachability of dedicated health endpoint
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q --spider http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/bin/bash", "/app/env_secrets_expand.sh"]
CMD ["/app/merrymaker"]

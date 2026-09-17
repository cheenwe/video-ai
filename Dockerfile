# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:24-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/baseplatform ./cmd/baseplatform

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 baseplatform && useradd --uid 10001 --gid baseplatform --no-create-home baseplatform \
    && mkdir -p /app/data && chown baseplatform:baseplatform /app/data
WORKDIR /app
COPY --from=backend /out/baseplatform /app/baseplatform
COPY --from=frontend /src/frontend/dist /app/frontend/dist
ENV BASE_HOST=0.0.0.0 BASE_PORT=8181 BASE_DATA_DIR=/app/data \
    BASE_FRONTEND_DIR=/app/frontend/dist TZ=Asia/Shanghai
USER 10001:10001
EXPOSE 8181
VOLUME ["/app/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD curl --fail --silent "http://127.0.0.1:${BASE_PORT}/api/health/live" > /dev/null || exit 1
ENTRYPOINT ["/app/baseplatform"]

# Legacy-builder compatible: no BuildKit-only directives so `docker build`
# works without DOCKER_BUILDKIT=1 support in the daemon.

FROM golang:1.26-alpine AS builder
WORKDIR /src

# go.mod/go.sum change rarely; keeping this its own layer means unchanged
# deps are rebuilt from cache even on legacy builders.
COPY go.mod go.sum ./
# Module proxy is occasionally flaky; retry with backoff before giving up.
RUN set -e; for i in 1 2 3 4 5; do \
      go mod download && break || { echo "go mod download attempt $i/5 failed"; sleep $((i * 2)); }; \
    done

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/api /app/api
COPY db/migrations /app/db/migrations

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]
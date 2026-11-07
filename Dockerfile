# syntax=docker/dockerfile:1

# Build on the native platform and cross-compile for the target one, so
# multi-arch images (linux/amd64, linux/arm64, ...) build quickly.
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /build

# Download dependencies first so they are cached between code changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/simple_http_server .

FROM alpine:3.20

# curl is kept for debugging from inside the container (docker exec ... curl).
RUN apk add --no-cache ca-certificates curl \
    && adduser -D -H -u 10001 app

COPY --from=builder /out/simple_http_server /simple_http_server

WORKDIR /
USER app

EXPOSE 8081

# The binary probes its own /healthz endpoint (respects PORT / BIND_ADDRESS).
HEALTHCHECK --interval=10s --timeout=5s --retries=3 CMD ["/simple_http_server", "-healthcheck"]
ENTRYPOINT ["/simple_http_server"]

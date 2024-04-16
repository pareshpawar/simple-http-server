FROM golang:alpine AS builder

# Set necessary environmet variables needed for our image
ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Move to working directory /build
WORKDIR /build

# Copy the code into the container
COPY . .

# Build the application
RUN go build -o simple_http_server ./simple_http_server.go


# Move to /dist directory as the place for resulting binary folder
WORKDIR /dist

# Copy binary from build to main folder
RUN cp /build/simple_http_server .

EXPOSE 8000

# Build a small image
FROM alpine
RUN apk update && apk add curl
COPY --from=builder /dist/simple_http_server /

HEALTHCHECK --interval=5s --timeout=10s --retries=3 CMD curl -sS 127.0.0.1:8081/healthcheck || exit 1
ENTRYPOINT ["/simple_http_server"]

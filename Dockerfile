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
RUN go build -o simple_http_server .

# Build a small image
FROM alpine
RUN apk update && apk add curl

COPY --from=builder /build/simple_http_server /simple_http_server
COPY --from=builder /build/html /html

WORKDIR /

EXPOSE 8081

HEALTHCHECK --interval=5s --timeout=10s --retries=3 CMD curl -sS 127.0.0.1:8081/healthcheck || exit 1
ENTRYPOINT ["/simple_http_server"]

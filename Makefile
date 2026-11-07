BINARY  := simple_http_server
IMAGE   ?= pareshpawar/simple-http-server
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build run test lint docker clean

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) .

run: build
	./$(BINARY)

test:
	go test -race ./...

lint:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)
	go vet ./...

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

clean:
	rm -f $(BINARY)

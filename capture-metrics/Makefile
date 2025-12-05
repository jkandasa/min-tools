.PHONY: build clean run test install

# Binary name
BINARY_NAME=minio-metrics

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Build the application
build:
	go build $(LDFLAGS) -o $(BINARY_NAME) .

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe .

# Clean build artifacts
clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*

# Run the application (requires MINIO_ENDPOINT and MINIO_TOKEN environment variables)
run: build
	./$(BINARY_NAME) \
		-endpoint "$(MINIO_ENDPOINT)" \
		-token "$(MINIO_TOKEN)"

# Install to /usr/local/bin
install: build
	sudo cp $(BINARY_NAME) /usr/local/bin/

# Run tests
test:
	go test -v ./...

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	golangci-lint run

# Show help
help:
	@echo "Available targets:"
	@echo "  build      - Build the binary"
	@echo "  build-all  - Build for multiple platforms"
	@echo "  clean      - Remove build artifacts"
	@echo "  run        - Build and run (requires MINIO_ENDPOINT and MINIO_TOKEN env vars)"
	@echo "  install    - Install to /usr/local/bin"
	@echo "  test       - Run tests"
	@echo "  fmt        - Format code"
	@echo "  lint       - Run linter"
	@echo ""
	@echo "Example usage:"
	@echo "  make build"
	@echo "  MINIO_ENDPOINT=https://minio.example.com MINIO_TOKEN=xxx make run"

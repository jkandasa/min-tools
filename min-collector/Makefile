.PHONY: build build-all clean run install test fmt vet lint tidy help

# Binary name
BINARY_NAME=min-collector

# Main package
MAIN_PKG=./cmd/min-collector

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Where the version information is injected
PKG=github.com/jkandasa/min-tools/min-collector/internal/config

# Build flags
# -s -w strips the symbol table and DWARF data, -trimpath removes local paths
# from the binary, CGO_ENABLED=0 keeps it static and portable.
LDFLAGS=-ldflags "-s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).BuildDate=$(BUILD_DATE)"
GOBUILD=CGO_ENABLED=0 go build -trimpath $(LDFLAGS)

# Build the application
build:
	$(GOBUILD) -o $(BINARY_NAME) $(MAIN_PKG)

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-linux-amd64 $(MAIN_PKG)
	GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(BINARY_NAME)-linux-arm64 $(MAIN_PKG)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-darwin-amd64 $(MAIN_PKG)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BINARY_NAME)-darwin-arm64 $(MAIN_PKG)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-windows-amd64.exe $(MAIN_PKG)

# Clean build artifacts
clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*

# Build and run with the defaults, logger and audit webhooks on :8080
run: build
	./$(BINARY_NAME)

# Build and run with the metrics scrape enabled as well
# MINIO_ENDPOINT and MINIO_TOKEN must be set
run-metrics: build
	./$(BINARY_NAME) \
		--metrics \
		--metrics-endpoint "$(MINIO_ENDPOINT)" \
		--metrics-token "$(MINIO_TOKEN)"

# Install to /usr/local/bin
install: build
	sudo cp $(BINARY_NAME) /usr/local/bin/

# Run tests
test:
	go test ./...

# Format code
fmt:
	go fmt ./...

# Run go vet
vet:
	go vet ./...

# Tidy dependencies
tidy:
	go mod tidy

# Run linter
lint:
	golangci-lint run

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  build-all    - Build for all supported platforms"
	@echo "  clean        - Remove build artifacts"
	@echo "  run          - Build and run with the default settings"
	@echo "  run-metrics  - Build and run with the metrics scrape enabled"
	@echo "  install      - Install to /usr/local/bin"
	@echo "  test         - Run tests"
	@echo "  fmt          - Format code"
	@echo "  vet          - Run go vet"
	@echo "  tidy         - Tidy go.mod"
	@echo "  lint         - Run golangci-lint"
	@echo ""
	@echo "Example usage:"
	@echo "  make build"
	@echo "  ./min-collector --help"
	@echo "  MINIO_ENDPOINT=https://minio.example.com MINIO_TOKEN=xxx make run-metrics"

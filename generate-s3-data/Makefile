# Generate S3 Data Makefile

BINARY_NAME=generate-s3-data
GO_VERSION=1.24

.PHONY: all build clean test run help

all: build

# Build the binary
build:
	go build -ldflags="-s -w" -o $(BINARY_NAME) .

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	go clean

# Run tests
test:
	go test -v ./...

# Download dependencies
deps:
	go mod tidy
	go mod download

# Run with default settings (requires MinIO server running on localhost:9000)
run: build
	./$(BINARY_NAME) --endpoint localhost:9000 --access-key minioadmin --secret-key minioadmin --buckets test-bucket --duration 1m

# Run with SSL
run-ssl: build
	./$(BINARY_NAME) --endpoint localhost:9443 --access-key minioadmin --secret-key minioadmin --buckets test-bucket --ssl --duration 1m

# Run indefinitely  
run-infinite: build
	./$(BINARY_NAME) --endpoint localhost:9000 --access-key minioadmin --secret-key minioadmin --buckets test-bucket --duration 0

# Run with multiple buckets
run-multi: build
	./$(BINARY_NAME) --endpoint localhost:9000 --access-key minioadmin --secret-key minioadmin --buckets "bucket1,bucket2,bucket3" --duration 2m

# Install binary to $GOPATH/bin
install: build
	cp $(BINARY_NAME) $$GOPATH/bin/

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Check Go version
check-version:
	@go version | grep -q "go$(GO_VERSION)" || (echo "Go $(GO_VERSION) required" && exit 1)

# Help
help:
	@echo "MinIO Audit Sidecar Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build          Build the binary"
	@echo "  make clean          Clean build artifacts"  
	@echo "  make test           Run tests"
	@echo "  make deps           Download dependencies"
	@echo "  make run            Run with default settings (1 minute)"
	@echo "  make run-ssl        Run with SSL enabled"
	@echo "  make run-infinite   Run indefinitely"
	@echo "  make run-multi      Run with multiple buckets (2 minutes)"
	@echo "  make install        Install binary to \$$GOPATH/bin"
	@echo "  make fmt            Format code"
	@echo "  make lint           Lint code"
	@echo "  make check-version  Check Go version"
	@echo "  make help           Show this help"
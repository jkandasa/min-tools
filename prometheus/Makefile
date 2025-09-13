# Makefile for MinIO Bucket Summary Tool

BINARY_NAME=bucket_summary
SOURCE_FILE=bucket_summary.go

.PHONY: build run test clean help

# Default target
all: build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) $(SOURCE_FILE)
	@echo "Build complete: $(BINARY_NAME)"

# Run with sample data (versions by default)
run: build
	@echo "Running with sample data (showing versions)..."
	./$(BINARY_NAME) sample.txt

# Run with test data (versions by default)
test: build
	@echo "Running with test data (showing versions)..."
	./$(BINARY_NAME) test_data.txt

# Run with size distribution
run-sizes: build
	@echo "Running with sample data (showing size distribution)..."
	./$(BINARY_NAME) sample.txt --sizes

# Run with both distributions
run-both: build
	@echo "Running with sample data (showing both distributions)..."
	./$(BINARY_NAME) sample.txt --both

# Run with custom parameters
run-top: build
	@echo "Running with sample data (top 10, showing sizes)..."
	./$(BINARY_NAME) sample.txt --sizes 10

# Clean build artifacts
clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)

# Install (copy to /usr/local/bin)
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo cp $(BINARY_NAME) /usr/local/bin/
	@echo "Installation complete"

# Show help
help:
	@echo "Available targets:"
	@echo "  build      - Build the binary"
	@echo "  run        - Build and run with sample.txt (showing versions)"
	@echo "  test       - Build and run with test_data.txt (showing versions)"
	@echo "  run-sizes  - Build and run with sample.txt (showing size distribution)"
	@echo "  run-both   - Build and run with sample.txt (showing both distributions)"
	@echo "  run-top    - Build and run with sample.txt showing top 10 with sizes"
	@echo "  clean      - Remove build artifacts"
	@echo "  install    - Install binary to /usr/local/bin"
	@echo "  help       - Show this help message"

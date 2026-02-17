.PHONY: build test clean install fmt vet lint integration-test

BINARY_NAME := kueue-bench
BUILD_DIR := bin
GO ?= go
GOLANGCI_LINT ?= golangci-lint

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/kueue-bench

install:
	@echo "Installing $(BINARY_NAME)..."
	$(GO) install ./cmd/kueue-bench

test:
	@echo "Running tests..."
	$(GO) test -v -short ./...

integration-test:
	@echo "Running integration tests..."
	$(GO) test -v -tags=integration ./test/integration/...

vet:
	@echo "Running go vet..."
	$(GO) vet ./...

fmt:
	@echo "Running go fmt..."
	$(GO) fmt ./...

lint:
	@echo "Running golangci-lint..."
	$(GOLANGCI_LINT) run ./...

clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)

# Development helpers
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

.PHONY: verify
verify: fmt lint test
	@echo "Verification complete!"

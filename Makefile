SHELL := /bin/bash
export PATH := $(HOME)/go/bin:$(PATH)

.DEFAULT_GOAL := help

# Project configuration
MODULE_NAME := github.com/rcarmo/busybox-wasm
BUILD_DIR := _build
COVERAGE_FILE := coverage.out
WASM_TARGET := wasip1

# Tool versions (for reproducibility)
GO_VERSION := 1.22
TINYGO_VERSION := 0.34
GOLANGCI_LINT_VERSION := latest
GOSEC_VERSION := latest

# Applets to build
APPLETS := echo cat ls cp mv rm head tail wc mkdir pwd busybox

.PHONY: help
help: ## Show targets
	@grep -E '^[a-zA-Z0-9_.-]+:.*?##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-18s %s\n", $$1, $$2}'

# =============================================================================
# Toolchain setup
# =============================================================================

.PHONY: setup-toolchain
setup-toolchain: ## Install Go and TinyGo via brew
	@echo "Installing Go and TinyGo..."
	@command -v brew >/dev/null 2>&1 || { echo "Error: brew is required"; exit 1; }
	@brew install go
	@brew tap tinygo-org/tools && brew install tinygo
	@echo "Go version: $$(go version)"
	@echo "TinyGo version: $$(tinygo version)"

.PHONY: check-toolchain
check-toolchain: ## Verify toolchain is installed
	@command -v go >/dev/null 2>&1 || { echo "Error: go not found. Run 'make setup-toolchain'"; exit 1; }
	@command -v tinygo >/dev/null 2>&1 || { echo "Error: tinygo not found. Run 'make setup-toolchain'"; exit 1; }

# =============================================================================
# Dependency management
# =============================================================================

.PHONY: deps
deps: check-toolchain ## Download Go module dependencies
	@go mod download
	@go mod verify

.PHONY: install
install: deps ## Install project dependencies
	@echo "Dependencies installed."

.PHONY: install-dev
install-dev: deps ## Install dev dependencies (linters, security tools)
	@echo "Installing golangci-lint..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "Installing gosec..."
	@go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
	@echo "Dev dependencies installed."

# =============================================================================
# Quality
# =============================================================================

.PHONY: vet
vet: check-toolchain ## Run go vet
	@go vet ./...

.PHONY: lint
lint: check-toolchain ## Run golangci-lint
	@golangci-lint run --timeout=5m ./...

.PHONY: security
security: check-toolchain ## Run gosec security scanner
	@gosec -quiet ./...

.PHONY: format
format: check-toolchain ## Format code with gofmt
	@gofmt -s -w .
	@go mod tidy

.PHONY: format-check
format-check: check-toolchain ## Check code formatting
	@test -z "$$(gofmt -l .)" || { echo "Code not formatted. Run 'make format'"; gofmt -l .; exit 1; }

.PHONY: check
check: vet lint format-check ## Run standard validation pipeline
	@echo "All checks passed."

# =============================================================================
# Testing
# =============================================================================

.PHONY: test
test: check-toolchain ## Run tests
	@go test -v ./...

.PHONY: test-race
test-race: check-toolchain ## Run tests with race detector
	@go test -v -race ./...

.PHONY: test-short
test-short: check-toolchain ## Run short tests only
	@go test -v -short ./...

.PHONY: coverage
coverage: check-toolchain ## Run tests with coverage
	@go test -v -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@go tool cover -func=$(COVERAGE_FILE)

.PHONY: coverage-html
coverage-html: coverage ## Generate HTML coverage report
	@go tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: dupl
dupl: ## Find duplicate code
	@command -v dupl >/dev/null 2>&1 || go install github.com/mibk/dupl@latest
	@echo "Checking for duplicate code (threshold: 50 tokens)..."
	@find ./pkg -name '*.go' -not -name '*_test.go' | xargs dupl -t 50 || true

.PHONY: test-quality
test-quality: coverage dupl ## Check test quality metrics
	@echo ""
	@echo "=== Test Summary ==="
	@echo "Coverage: $$(go tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}')"
	@echo "Test count: $$(go test -v ./... 2>&1 | grep -c '=== RUN' || echo 0)"

# =============================================================================
# Build
# =============================================================================

$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

.PHONY: build
build: check-toolchain $(BUILD_DIR) ## Build all applets as native binaries (for testing)
	@for applet in $(APPLETS); do \
		echo "Building $$applet (native)..."; \
		go build -o $(BUILD_DIR)/$$applet ./cmd/$$applet; \
	done

.PHONY: build-wasm
build-wasm: check-toolchain $(BUILD_DIR) ## Build all applets as WASM
	@for applet in $(APPLETS); do \
		echo "Building $$applet (WASM)..."; \
		tinygo build -target=$(WASM_TARGET) -o $(BUILD_DIR)/$$applet.wasm ./cmd/$$applet; \
	done

.PHONY: build-wasm-optimized
build-wasm-optimized: check-toolchain $(BUILD_DIR) ## Build optimized WASM (smaller size)
	@for applet in $(APPLETS); do \
		echo "Building $$applet (WASM optimized)..."; \
		tinygo build -target=$(WASM_TARGET) -opt=z -no-debug -o $(BUILD_DIR)/$$applet.wasm ./cmd/$$applet; \
	done

.PHONY: build-all
build-all: build build-wasm ## Build both native and WASM

# =============================================================================
# Development
# =============================================================================

.PHONY: init
init: setup-toolchain install-dev ## Initialize project (first-time setup)
	@test -f go.mod || go mod init $(MODULE_NAME)
	@echo "Project initialized. Run 'make build' to build."

.PHONY: run
run: build ## Build and run a specific applet (usage: make run APPLET=echo ARGS="hello")
	@$(BUILD_DIR)/$(APPLET) $(ARGS)

.PHONY: run-wasm
run-wasm: build-wasm ## Run WASM applet with wasmtime (usage: make run-wasm APPLET=echo ARGS="hello")
	@command -v wasmtime >/dev/null 2>&1 || { echo "Error: wasmtime not found. Install with 'brew install wasmtime'"; exit 1; }
	@wasmtime $(BUILD_DIR)/$(APPLET).wasm $(ARGS)

# =============================================================================
# Cleanup
# =============================================================================

.PHONY: clean
clean: ## Remove build artifacts
	@rm -rf $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE) coverage.html

.PHONY: clean-all
clean-all: clean ## Remove all generated files including module cache
	@go clean -cache -modcache

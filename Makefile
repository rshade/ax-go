GOLANGCI_LINT?=$(HOME)/go/bin/golangci-lint
GOLANGCI_LINT_VERSION?=2.12.2
MARKDOWNLINT?=markdownlint
MARKDOWNLINT_FILES?=AGENTS.md README.md docs/**/*.md
ACTIONLINT?=$(HOME)/go/bin/actionlint

.PHONY: all
all: build

.PHONY: build
build:
	@echo "Building ax library..."
	go build ./...

.PHONY: test
test:
	@echo "Running tests with race detector..."
	go test -race ./...

.PHONY: test-cover
test-cover:
	@echo "Running tests with coverage..."
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

.PHONY: bench
bench:
	@echo "Running benchmarks..."
	go test -run='^$$' -bench=. -benchmem ./...

.PHONY: doc-coverage
doc-coverage:
	@echo "Checking ExampleXxx coverage on the primary API..."
	go run ./internal/cmd/doccover

.PHONY: lint
lint:
	@echo "Running golangci-lint (expected version $(GOLANGCI_LINT_VERSION))..."
	@$(GOLANGCI_LINT) --version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION)" || \
		(echo "golangci-lint $(GOLANGCI_LINT_VERSION) required. Install with"; \
		echo "  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$HOME/go/bin v$(GOLANGCI_LINT_VERSION)"; exit 1)
	$(GOLANGCI_LINT) run --allow-parallel-runners
	@echo "Running markdownlint..."
	@command -v $(MARKDOWNLINT) >/dev/null 2>&1 || \
		(echo "markdownlint CLI not found. Install with"; \
		echo "  npm install -g markdownlint-cli@0.45.0"; exit 1)
	$(MARKDOWNLINT) $(MARKDOWNLINT_FILES)
	@$(MAKE) lint-actions

.PHONY: lint-actions
lint-actions:
	@echo "Running actionlint..."
	@command -v $(ACTIONLINT) >/dev/null 2>&1 || \
		(echo "actionlint not found. Install with"; \
		echo "  go install github.com/rhysd/actionlint/cmd/actionlint@latest"; exit 1)
	find .github/workflows -name '*.yml' -not -name '*.lock.yml' -print0 | xargs -0 $(ACTIONLINT)

.PHONY: validate
validate:
	@echo "Checking gofmt..."
	@test -z "$$(gofmt -s -l . | tee /dev/stderr)" || (echo "Run gofmt -s -w ."; exit 1)
	@echo "Checking go mod tidy..."
	go mod tidy -diff
	@echo "Running go vet..."
	go vet ./...
	@echo "Validation complete."

.PHONY: security
security:
	@echo "Running govulncheck..."
	@command -v govulncheck >/dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

.PHONY: ensure
ensure: ensure-golangci-lint ensure-markdownlint ensure-actionlint
	@echo "All dev tools are ready."

.PHONY: ensure-golangci-lint
ensure-golangci-lint:
	@echo "==> golangci-lint $(GOLANGCI_LINT_VERSION)"
	@$(GOLANGCI_LINT) --version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION)" || \
		(echo "    Installing golangci-lint v$(GOLANGCI_LINT_VERSION)..." && \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$HOME/go/bin v$(GOLANGCI_LINT_VERSION))
	@echo "    OK"

.PHONY: ensure-markdownlint
ensure-markdownlint:
	@echo "==> markdownlint-cli"
	@command -v $(MARKDOWNLINT) >/dev/null 2>&1 || \
		(echo "    Installing markdownlint-cli@0.45.0..." && \
		npm install -g markdownlint-cli@0.45.0)
	@echo "    OK"

.PHONY: ensure-actionlint
ensure-actionlint:
	@echo "==> actionlint"
	@command -v $(ACTIONLINT) >/dev/null 2>&1 || \
		(echo "    Installing actionlint..." && \
		go install github.com/rhysd/actionlint/cmd/actionlint@latest)
	@echo "    OK"

.PHONY: clean
clean:
	@echo "Cleaning..."
	rm -f coverage.out

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build         - Compile the library (go build ./...)"
	@echo "  test          - Run tests with the race detector"
	@echo "  test-cover    - Run tests with coverage profile"
	@echo "  bench         - Run benchmarks with -benchmem"
	@echo "  doc-coverage  - Check ExampleXxx coverage on the primary API"
	@echo "  lint          - Run golangci-lint, markdownlint, and actionlint"
	@echo "  lint-actions  - Run actionlint on GitHub workflows"
	@echo "  validate      - Check gofmt, go mod tidy, and go vet"
	@echo "  security      - Run govulncheck"
	@echo "  ensure        - Install all required dev tools at pinned versions"
	@echo "  clean         - Remove coverage artifacts"
	@echo "  help          - Show this help message"

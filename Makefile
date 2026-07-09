GOLANGCI_LINT?=$(HOME)/go/bin/golangci-lint
GOLANGCI_LINT_VERSION?=2.12.2
MARKDOWNLINT?=markdownlint
MARKDOWNLINT_FILES?=AGENTS.md README.md .github/copilot-instructions.md docs/**/*.md
ACTIONLINT?=$(HOME)/go/bin/actionlint
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-unknown)
BENCH_CPU?=1
BENCH_COUNT?=10
BENCH_BASE_REF?=$(shell git merge-base HEAD origin/main 2>/dev/null || git rev-parse --verify --quiet HEAD~1 2>/dev/null || echo origin/main)
BENCH_FLAGS=-run='^$$' -bench=. -benchmem -cpu=$(BENCH_CPU)
BENCH_COMPARE_FLAGS=$(BENCH_FLAGS) -count=$(BENCH_COUNT)

.PHONY: all
all: build

.PHONY: ci
ci: test validate lint doc-coverage bench-check

.PHONY: build
build:
	@echo "Building ax library..."
	go build ./...

.PHONY: build-example
build-example:
	@echo "Building integration example with version $(VERSION)..."
	go build -ldflags "-X main.version=$(VERSION)" -o bin/ax-integration ./examples/integration

.PHONY: test
test:
	@echo "Running tests with race detector..."
	go test -race ./...

.PHONY: test-cover
test-cover:
	@echo "Running tests with coverage..."
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

.PHONY: cover-check
cover-check: test-cover
	@echo "Checking coverage floors..."
	go run ./internal/cmd/covercheck -coverage coverage.out

.PHONY: bench
bench:
	@echo "Running benchmarks with -cpu=$(BENCH_CPU)..."
	go test $(BENCH_FLAGS) ./...

.PHONY: bench-check
bench-check:
	@echo "Checking performance regression budget against $(BENCH_BASE_REF) with -cpu=$(BENCH_CPU)..."
	@base_ref="$(BENCH_BASE_REF)"; \
	tmpdir=$$(mktemp -d); \
	base_worktree="$$tmpdir/base"; \
	base_out="$$tmpdir/bench-base.txt"; \
	current_out="$$tmpdir/bench-current.txt"; \
	cleanup() { \
		git worktree remove --force "$$base_worktree" >/dev/null 2>&1 || true; \
		rm -rf "$$tmpdir"; \
	}; \
	trap cleanup EXIT INT TERM; \
	if ! git rev-parse --verify --quiet "$$base_ref^{commit}" >/dev/null; then \
		echo "bench-check: baseline ref '$$base_ref' not found; fetch main or set BENCH_BASE_REF=<ref>" >&2; \
		exit 2; \
	fi; \
	if ! git worktree add --detach --quiet "$$base_worktree" "$$base_ref"; then \
		echo "bench-check: could not create baseline worktree for '$$base_ref'" >&2; \
		exit 2; \
	fi; \
	echo "bench-check: capturing baseline from $$base_ref"; \
	if ! (cd "$$base_worktree" && go test $(BENCH_COMPARE_FLAGS) ./...) > "$$base_out"; then \
		echo "bench-check: baseline benchmark run failed (see output below); performance budget was not checked" >&2; \
		cat "$$base_out" >&2; \
		exit 1; \
	fi; \
	echo "bench-check: capturing current worktree"; \
	if ! go test $(BENCH_COMPARE_FLAGS) ./... > "$$current_out"; then \
		echo "bench-check: current benchmark run failed (see output below); performance budget was not checked" >&2; \
		cat "$$current_out" >&2; \
		exit 1; \
	fi; \
	go run ./internal/cmd/benchcheck -baseline "$$base_out" -current "$$current_out"; \
	status=$$?; \
	exit $$status

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
	@echo "  ci            - Run test, validate, lint, and doc-coverage"
	@echo "  build         - Compile the library (go build ./...)"
	@echo "  build-example - Compile the integration example with version injection"
	@echo "  test          - Run tests with the race detector"
	@echo "  test-cover    - Run tests with coverage profile"
	@echo "  cover-check   - Enforce per-package and repo-wide coverage floors"
	@echo "  bench         - Run benchmarks with -benchmem"
	@echo "  bench-check   - Enforce the performance regression budget against BENCH_BASE_REF"
	@echo "  doc-coverage  - Check ExampleXxx coverage on the primary API"
	@echo "  lint          - Run golangci-lint, markdownlint, and actionlint"
	@echo "  lint-actions  - Run actionlint on GitHub workflows"
	@echo "  validate      - Check gofmt, go mod tidy, and go vet"
	@echo "  security      - Run govulncheck"
	@echo "  ensure        - Install all required dev tools at pinned versions"
	@echo "  clean         - Remove coverage artifacts"
	@echo "  help          - Show this help message"

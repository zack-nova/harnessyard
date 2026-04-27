SHELL := /bin/sh
.DEFAULT_GOAL := help

APP ?= hyard
PKG ?= ./...
DIST_DIR ?= .dist
BIN_DIR ?= $(DIST_DIR)/bin
COVERPROFILE ?= coverage.out
COVERHTML ?= coverage.html

GO ?= go
GOFMT ?= gofmt
GIT ?= git
MISE ?= mise
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
GORELEASER ?= goreleaser

.PHONY: help doctor tools tidy tidy-check fmt fmt-check fix lint test test-ci \
	test-race cover cover-html build clean check ci vuln test-scripts \
	release-surface release-check release-snapshot release release-verify

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

doctor: ## Show local tool availability
	@printf "Project: %s\n" "$(APP)"
	@printf "Go packages: %s\n\n" "$(PKG)"
	@command -v "$(GO)" >/dev/null 2>&1 && "$(GO)" version || { echo "go: missing"; exit 1; }
	@command -v "$(MISE)" >/dev/null 2>&1 && "$(MISE)" --version || echo "mise: missing"
	@command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1 && "$(GOLANGCI_LINT)" version || echo "golangci-lint: missing"
	@command -v "$(GOVULNCHECK)" >/dev/null 2>&1 && "$(GOVULNCHECK)" -version || echo "govulncheck: missing"
	@command -v "$(GORELEASER)" >/dev/null 2>&1 && "$(GORELEASER)" --version | sed -n '1,3p' || echo "goreleaser: missing"

tools: ## Install tools pinned in mise.toml
	@command -v "$(MISE)" >/dev/null 2>&1 || { echo "mise is required for tool installation" >&2; exit 127; }
	@echo "If mise has not trusted this repository yet, run: mise trust"
	$(MISE) install

tidy: ## Update go.mod and go.sum
	@set -e; \
	if [ -f go.mod ]; then \
		$(GO) mod tidy; \
	else \
		echo "No go.mod yet; skipping go mod tidy"; \
	fi

tidy-check: ## Check go.mod/go.sum are tidy and verified
	@set -e; \
	if [ -f go.mod ]; then \
		$(GO) mod tidy -diff; \
		$(GO) mod verify; \
	else \
		echo "No go.mod yet; skipping module validation"; \
	fi

fmt: ## Format Go source files
	@set -e; \
	if ! $(GIT) ls-files --cached --others --exclude-standard -- '*.go' | grep -q .; then \
		echo "No Go files yet; skipping gofmt"; \
	else \
		$(GIT) ls-files -z --cached --others --exclude-standard -- '*.go' | xargs -0 $(GOFMT) -s -w; \
	fi

fmt-check: ## Check Go source formatting
	@set -e; \
	if ! $(GIT) ls-files --cached --others --exclude-standard -- '*.go' | grep -q .; then \
		echo "No Go files yet; skipping gofmt check"; \
	else \
		unformatted="$$( $(GIT) ls-files -z --cached --others --exclude-standard -- '*.go' | xargs -0 $(GOFMT) -l -s )"; \
		if [ -n "$$unformatted" ]; then \
			echo "These .go files need formatting:"; \
			echo; \
			echo "$$unformatted"; \
			echo; \
			echo "To fix: make fmt"; \
			exit 1; \
		fi; \
	fi

fix: ## Format code and tidy module files
	@$(MAKE) fmt
	@$(MAKE) tidy

lint: ## Run golangci-lint through the repository wrapper
	sh ./scripts/run_golangci_lint.sh

test: ## Run Go tests
	@set -e; \
	if [ -f go.mod ]; then \
		$(GO) test $(PKG); \
	else \
		echo "No go.mod yet; skipping tests"; \
	fi

test-ci: ## Run uncached Go tests
	@set -e; \
	if [ -f go.mod ]; then \
		$(GO) test -count=1 $(PKG); \
	else \
		echo "No go.mod yet; skipping tests"; \
	fi

test-race: ## Run Go tests with the race detector
	@set -e; \
	if [ -f go.mod ]; then \
		$(GO) test -race $(PKG); \
	else \
		echo "No go.mod yet; skipping race tests"; \
	fi

cover: ## Run tests with coverage summary
	@set -e; \
	if [ -f go.mod ]; then \
		$(GO) test -race -coverprofile="$(COVERPROFILE)" $(PKG); \
		$(GO) tool cover -func="$(COVERPROFILE)"; \
	else \
		echo "No go.mod yet; skipping coverage"; \
	fi

cover-html: cover ## Generate HTML coverage report
	$(GO) tool cover -html="$(COVERPROFILE)" -o "$(COVERHTML)"

build: ## Build the hyard binary
	sh ./scripts/build_binaries.sh "$(BIN_DIR)"

clean: ## Remove local build and coverage artifacts
	rm -rf "$(DIST_DIR)" bin dist "$(COVERPROFILE)" "$(COVERHTML)" coverage/

check: ## Run fast local checks
	@$(MAKE) tidy-check
	@$(MAKE) fmt-check
	@$(MAKE) lint
	@$(MAKE) test

ci: ## Run full CI-local validation
	@$(MAKE) tidy-check
	@$(MAKE) fmt-check
	@$(MAKE) lint
	@$(MAKE) test-ci
	@$(MAKE) vuln
	@$(MAKE) test-scripts
	@$(MAKE) build

vuln: ## Run govulncheck
	@set -e; \
	if [ ! -f go.mod ]; then \
		echo "No go.mod yet; skipping vulnerability check"; \
	elif command -v "$(GOVULNCHECK)" >/dev/null 2>&1; then \
		"$(GOVULNCHECK)" $(PKG); \
	elif command -v "$(MISE)" >/dev/null 2>&1; then \
		"$(MISE)" exec -- "$(GOVULNCHECK)" $(PKG); \
	else \
		echo "govulncheck not found; run 'make tools' or install $(GOVULNCHECK)" >&2; \
		exit 127; \
	fi

test-scripts: ## Run shell validation tests
	sh ./scripts/test_run_golangci_lint.sh
	sh ./scripts/test_build_binaries.sh
	sh ./scripts/test_install_script.sh
	sh ./scripts/test_release_surface_hyard.sh

release-surface: ## Check the public release surface contract
	sh ./scripts/test_release_surface_hyard.sh

release-check: ## Validate GoReleaser configuration
	@command -v "$(GORELEASER)" >/dev/null 2>&1 || { echo "goreleaser not found; install goreleaser before releasing" >&2; exit 127; }
	$(GORELEASER) check

release-snapshot: ## Build a local GoReleaser snapshot
	@command -v "$(GORELEASER)" >/dev/null 2>&1 || { echo "goreleaser not found; install goreleaser before releasing" >&2; exit 127; }
	$(GORELEASER) release --snapshot --clean

release: ## Publish a release with GoReleaser
	@command -v "$(GORELEASER)" >/dev/null 2>&1 || { echo "goreleaser not found; install goreleaser before releasing" >&2; exit 127; }
	$(GORELEASER) release --clean

release-verify: ## Run maintainer release verification
	@$(MAKE) tidy-check
	@$(MAKE) fmt-check
	@$(MAKE) test-ci
	@$(MAKE) release-surface
	@$(MAKE) release-check
	@$(MAKE) release-snapshot

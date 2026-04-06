GO ?= go
BIN_DIR ?= bin
TINX_BIN ?= $(BIN_DIR)/tinx

TINX_HOME ?= $(CURDIR)/.tinx-home

ECHO_PROVIDER_DIR ?= testdata/echo-provider
ECHO_PROVIDER_REF ?= sourceplane/echo-provider
ECHO_PROVIDER_ALIAS ?= echo
ECHO_PROVIDER_DIST ?= $(ECHO_PROVIDER_DIR)/dist
ECHO_PROVIDER_OCI ?= $(ECHO_PROVIDER_DIR)/oci
ECHO_PROVIDER_INTENT ?= $(ECHO_PROVIDER_DIR)/intent.yaml
EXAMPLE_WORKSPACE ?= $(CURDIR)/.example-workspace

GHCR_OWNER ?= $(shell echo "$${GITHUB_REPOSITORY_OWNER:-$${USER}}" | tr '[:upper:]' '[:lower:]')
GHCR_REPO ?= ghcr.io/$(GHCR_OWNER)/tinx-echo-provider
GHCR_TAG ?= dev
GHCR_REF ?= $(GHCR_REPO):$(GHCR_TAG)

.PHONY: help tidy build test test-core release-example install-example run-example e2e-local ghcr-push ghcr-install-run clean

help:
	@echo "Targets:"
	@echo "  make tidy              - Run go mod tidy"
	@echo "  make build             - Build tinx CLI to $(TINX_BIN)"
	@echo "  make test              - Run go test ./..."
	@echo "  make test-core         - Run tinx command + OCI tests only"
	@echo "  make release-example   - Package test echo package into OCI layout"
	@echo "  make install-example   - Install test echo package from local OCI layout"
	@echo "  make run-example       - Run installed echo tool capability"
	@echo "  make e2e-local         - release+install+run using local OCI layout"
	@echo "  make ghcr-push         - Push example package to GHCR (requires auth)"
	@echo "  make ghcr-install-run  - Install from GHCR and run capability"
	@echo "  make clean             - Remove local build/test artifacts"

tidy:
	$(GO) mod tidy

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(TINX_BIN) ./cmd/tinx

test:
	$(GO) test ./...

test-core:
	$(GO) test ./internal/cmd ./internal/manifest ./internal/oci

release-example: build
	./$(TINX_BIN) release \
		--manifest $(ECHO_PROVIDER_DIR)/tinx.yaml \
		--main ./cmd/echo-provider \
		--dist $(ECHO_PROVIDER_DIST) \
		--output $(ECHO_PROVIDER_OCI)

install-example: release-example
	./$(TINX_BIN) --tinx-home $(TINX_HOME) install $(ECHO_PROVIDER_ALIAS) $(ECHO_PROVIDER_REF) --source $(ECHO_PROVIDER_OCI)

run-example: install-example
	rm -rf $(EXAMPLE_WORKSPACE)
	./$(TINX_BIN) --tinx-home $(TINX_HOME) init $(EXAMPLE_WORKSPACE) -p $(CURDIR)/$(ECHO_PROVIDER_OCI) as $(ECHO_PROVIDER_ALIAS)
	cd $(EXAMPLE_WORKSPACE) && $(CURDIR)/$(TINX_BIN) --tinx-home $(TINX_HOME) -- $(ECHO_PROVIDER_ALIAS) plan --intent $(CURDIR)/$(ECHO_PROVIDER_INTENT)

e2e-local: run-example

ghcr-push: build
	./$(TINX_BIN) release \
		--manifest $(ECHO_PROVIDER_DIR)/tinx.yaml \
		--main ./cmd/echo-provider \
		--dist $(ECHO_PROVIDER_DIST) \
		--output $(ECHO_PROVIDER_OCI) \
		--push $(GHCR_REF)

ghcr-install-run: build
	rm -rf $(EXAMPLE_WORKSPACE)
	./$(TINX_BIN) --tinx-home $(TINX_HOME) init $(EXAMPLE_WORKSPACE) -p $(GHCR_REF) as $(ECHO_PROVIDER_ALIAS)
	cd $(EXAMPLE_WORKSPACE) && $(CURDIR)/$(TINX_BIN) --tinx-home $(TINX_HOME) -- $(ECHO_PROVIDER_ALIAS) plan --intent $(CURDIR)/$(ECHO_PROVIDER_INTENT)

clean:
	rm -rf $(BIN_DIR)
	rm -rf $(TINX_HOME)
	rm -rf $(EXAMPLE_WORKSPACE)
	rm -rf $(ECHO_PROVIDER_DIST)
	rm -rf $(ECHO_PROVIDER_OCI)

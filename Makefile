GO ?= go
BIN_DIR ?= bin
KIOX_BIN ?= $(BIN_DIR)/kiox

KIOX_HOME ?= $(CURDIR)/.kiox-home

ECHO_PROVIDER_DIR ?= testdata/echo-provider
ECHO_PROVIDER_REF ?= sourceplane/echo-provider
ECHO_PROVIDER_ALIAS ?= echo
ECHO_PROVIDER_DIST ?= $(ECHO_PROVIDER_DIR)/dist
ECHO_PROVIDER_OCI ?= $(ECHO_PROVIDER_DIR)/oci

GHCR_OWNER ?= $(shell echo "$${GITHUB_REPOSITORY_OWNER:-$${USER}}" | tr '[:upper:]' '[:lower:]')
GHCR_REPO ?= ghcr.io/$(GHCR_OWNER)/kiox-echo-provider
GHCR_TAG ?= dev
GHCR_REF ?= $(GHCR_REPO):$(GHCR_TAG)

.PHONY: help tidy build test test-core release-example install-example run-example e2e-local ghcr-push ghcr-install-run clean

help:
	@echo "Targets:"
	@echo "  make tidy              - Run go mod tidy"
	@echo "  make build             - Build kiox CLI to $(KIOX_BIN)"
	@echo "  make test              - Run go test ./..."
	@echo "  make test-core         - Run kiox command + OCI tests only"
	@echo "  make release-example   - Package test echo provider into OCI layout"
	@echo "  make install-example   - Install test echo provider from local OCI layout"
	@echo "  make run-example       - Run installed echo provider capability"
	@echo "  make e2e-local         - release+install+run using local OCI layout"
	@echo "  make ghcr-push         - Push example provider package to GHCR (requires auth)"
	@echo "  make ghcr-install-run  - Install from GHCR and run capability"
	@echo "  make clean             - Remove local build/test artifacts"

tidy:
	$(GO) mod tidy

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(KIOX_BIN) ./cmd/kiox

test:
	$(GO) test ./...

test-core:
	$(GO) test ./internal/cmd ./internal/manifest ./internal/oci

release-example: build
	./$(KIOX_BIN) release \
		--manifest $(ECHO_PROVIDER_DIR)/kiox.yaml \
		--main ./cmd/echo-provider \
		--dist $(ECHO_PROVIDER_DIST) \
		--output $(ECHO_PROVIDER_OCI)

install-example: release-example
	./$(KIOX_BIN) --kiox-home $(KIOX_HOME) install $(ECHO_PROVIDER_ALIAS) $(ECHO_PROVIDER_REF) --source $(ECHO_PROVIDER_OCI)

run-example: install-example
	cd $(ECHO_PROVIDER_DIR) && ../../$(KIOX_BIN) --kiox-home $(KIOX_HOME) run $(ECHO_PROVIDER_ALIAS) plan --intent intent.yaml

e2e-local: run-example

ghcr-push: build
	./$(KIOX_BIN) release \
		--manifest $(ECHO_PROVIDER_DIR)/kiox.yaml \
		--main ./cmd/echo-provider \
		--dist $(ECHO_PROVIDER_DIST) \
		--output $(ECHO_PROVIDER_OCI) \
		--push $(GHCR_REF)

ghcr-install-run: build
	./$(KIOX_BIN) --kiox-home $(KIOX_HOME) install $(ECHO_PROVIDER_ALIAS) $(GHCR_REF)
	cd $(ECHO_PROVIDER_DIR) && ../../$(KIOX_BIN) --kiox-home $(KIOX_HOME) run $(ECHO_PROVIDER_ALIAS) plan --intent intent.yaml

clean:
	rm -rf $(BIN_DIR)
	rm -rf $(KIOX_HOME)
	rm -rf $(ECHO_PROVIDER_DIST)
	rm -rf $(ECHO_PROVIDER_OCI)

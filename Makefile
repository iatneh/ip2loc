# ip2loc Makefile
#
# The `help` target prints every documented target so a fresh checkout has a
# discoverable surface area. Targets prefixed with `--` are private.

GO          ?= go
GOOS        ?= linux
GOARCH      ?= amd64
CGO_ENABLED ?= 0
LDFLAGS     := -s -w -X main.version=$(VERSION)
BUILD_DIR    = build
BIN_NAME    = ip2loc

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

.PHONY: help
help: ## Show this help.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: all
all: tidy test build ## tidy + test + build

.PHONY: tidy
tidy: ## Sync go.mod/go.sum.
	$(GO) mod tidy

.PHONY: fmt
fmt: ## gofmt -s -w the tree.
	$(GO) fmt ./...

.PHONY: vet
vet: ## go vet the tree.
	$(GO) vet ./...

.PHONY: test
test: ## Run unit tests.
	$(GO) test -race -count=1 ./...

.PHONY: test-cover
test-cover: ## Run tests with coverage report.
	$(GO) test -race -count=1 -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

.PHONY: build
build: ## Build binary to ./build/ip2loc.
	@mkdir -p $(BUILD_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
		$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(BIN_NAME) ./cmd/ip2loc

.PHONY: run
run: ## Run the server with all defaults baked in.
	$(GO) run ./cmd/ip2loc

.PHONY: print-defaults
print-defaults: build ## Print the baked-in default config and exit.
	./$(BUILD_DIR)/$(BIN_NAME) -print-defaults

.PHONY: clean
clean: ## Remove build artefacts.
	rm -rf $(BUILD_DIR) coverage.out

.PHONY: docker
docker: ## Build the container image.
	docker build -t iatneh1900/$(BIN_NAME):$(VERSION) .

.PHONY: docker-run
docker-run: ## Run the container, mapping 8080.
	docker run --rm -p 8080:8080 -v $(PWD)/data:/opt/data iatneh1900/$(BIN_NAME):$(VERSION)

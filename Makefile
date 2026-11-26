# sloeval developer tasks. Run `make help` for a list.

GO           ?= go
BIN          := bin/sloe
PROM_VERSION ?= 3.5.0
TOOLS        := .tools
PROMTOOL     ?= $(shell command -v promtool 2>/dev/null || echo $(TOOLS)/promtool)

# Generated Prometheus rule files and their unit tests (empty until Phase 1).
RULE_FILES := $(shell find . -path ./.tools -prune -o -name '*.rules.yaml' -print)
TEST_FILES := $(shell find . -path ./.tools -prune -o -name '*.test.yaml' -print)

.PHONY: help build test lint golden promtool-check promtool fmt tidy clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n",$$1,$$2}'

build: ## Build the sloe CLI into bin/
	$(GO) build -o $(BIN) ./cmd/sloe

test: ## Run unit and golden tests with the race detector
	$(GO) test -race ./...

lint: ## Run golangci-lint
	golangci-lint run ./...

golden: ## Regenerate golden files (review the diff before committing)
	$(GO) test ./... -update

fmt: ## Format code
	golangci-lint fmt ./...

tidy: ## Tidy go.mod
	$(GO) mod tidy

promtool: ## Download pinned promtool into .tools/ (checksum-verified)
	./hack/install-promtool.sh $(PROM_VERSION) $(TOOLS)

promtool-check: ## promtool check + test every generated rule file
ifeq ($(strip $(RULE_FILES)),)
	@echo "promtool-check: no *.rules.yaml files yet, nothing to check"
else
	$(PROMTOOL) check rules $(RULE_FILES)
endif
ifneq ($(strip $(TEST_FILES)),)
	$(PROMTOOL) test rules $(TEST_FILES)
endif

clean: ## Remove build output
	rm -rf bin dist out

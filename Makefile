# Convenience wrappers around docker compose.
#
# Everything here is optional: the compose commands work directly, which is
# what Windows users without make will use. The targets exist so that the
# common operations have short names.

COMPOSE ?= docker compose
# Passed through to the image build so that generated files are owned by the
# invoking user on Linux. Harmless on Docker Desktop.
export UID  ?= $(shell id -u 2>/dev/null || echo 1000)
export GID  ?= $(shell id -g 2>/dev/null || echo 1000)

.PHONY: help build setup doctor test check record vendor deps shell clean reset native

help: ## Show this help
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  %-10s %s\n", $$1, $$2}'

build: ## Build the dev image
	$(COMPOSE) build

setup: ## Fetch dependencies and vendor the runtime (run once, after build)
	$(COMPOSE) run --rm setup

test: ## Run the test suite in the container
	$(COMPOSE) run --rm test

check: ## Run gofmt, vet, and tests (what CI gates on)
	$(COMPOSE) run --rm check

record: ## Re-record differential goldens from babel-preset-solid
	$(COMPOSE) run --rm record

vendor: ## Populate the embedded solid-js runtime
	$(COMPOSE) run --rm vendor

deps: ## Fetch or update the TypeScript compiler fork
	$(COMPOSE) run --rm deps

shell: ## Interactive shell in the dev environment
	$(COMPOSE) run --rm shell

doctor: ## Check that cache volumes are writable
	$(COMPOSE) run --rm doctor

reset: ## Discard volumes and rebuild from scratch
	$(COMPOSE) down -v
	$(COMPOSE) build --no-cache

clean: ## Remove containers and cache volumes
	$(COMPOSE) down -v

native: ## Run tests on the host toolchain, bypassing Docker
	go test -race -count=1 ./...

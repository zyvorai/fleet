SHELL := /bin/sh
BIN_DIR ?= bin
VERSION ?= 0.3.0

.PHONY: all build test test-race vet fmt check web-check live-smoke qualify demo docker-up docker-down clean deploy-remote help ci status
all: check build ## Format-check, vet, test, then build

build: ## Build fleetd, fleet-agent, and fleetctl
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BIN_DIR)/fleetd ./cmd/fleetd
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/zyvorai/fleet/internal/agent.Version=$(VERSION)" -o $(BIN_DIR)/fleet-agent ./cmd/fleet-agent
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BIN_DIR)/fleetctl ./cmd/fleetctl

test: ## Unit tests
	go test ./...

test-race: ## Tests with the race detector
	go test -race ./...

vet: ## go vet
	go vet ./...

fmt: ## Rewrite Go sources with gofmt
	gofmt -w cmd internal webui

web-check: ## Syntax-check the console JavaScript
	node --check webui/static/app.js

live-smoke: build
	python3 scripts/live-smoke.py

qualify: build
	python3 scripts/qualify-matrix.py

check: ## gofmt, vet, tests, and web syntax
	test -z "$$(gofmt -l cmd internal webui)"
	go vet ./...
	go test ./...
	node --check webui/static/app.js

demo: build
	ZYVOR_FLEET_ADMIN_PASSWORD=zyvor-fleet-demo ZYVOR_FLEET_SESSION_SECRET=local-demo-session-secret-change-me-1234567890 $(BIN_DIR)/fleetd --demo

docker-up:
	docker compose up --build

docker-down:
	docker compose down -v

clean:
	rm -rf $(BIN_DIR) data

# Remote deploy (SSH + rsync). Example: make deploy-remote H=10.0.0.5 U=sus
deploy-remote: ## Deploy: make deploy-remote H=<host> [U=user] [ARGS=--quick]
	@test -n "$(H)" || (echo "Usage: make deploy-remote H=<host> U=<user> [ARGS='--quick']"; exit 1)
	./scripts/deploy-remote.sh $(if $(U),$(U)@)$(H) $(ARGS)

ci: ## Local gate: gofmt, vet, race tests, web syntax, build
	test -z "$$(gofmt -l cmd internal webui)"
	$(MAKE) vet test-race web-check build

status: build ## fleetctl status (needs a running fleetd and login)
	$(BIN_DIR)/fleetctl status

help: ## Show targets
	@grep -E '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | sort | awk -F':.*## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

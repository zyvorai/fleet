SHELL := /bin/sh
BIN_DIR ?= bin
VERSION ?= 0.2.0

.PHONY: all build test test-race vet fmt check web-check live-smoke demo docker-up docker-down clean deploy-remote
all: check build

build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BIN_DIR)/fleetd ./cmd/fleetd
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/zyvorai/fleet/internal/agent.Version=$(VERSION)" -o $(BIN_DIR)/fleet-agent ./cmd/fleet-agent
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BIN_DIR)/fleetctl ./cmd/fleetctl

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal webui

web-check:
	node --check webui/static/app.js

live-smoke: build
	python3 scripts/live-smoke.py

check:
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
deploy-remote:
	@test -n "$(H)" || (echo "Usage: make deploy-remote H=<host> U=<user> [ARGS='--quick']"; exit 1)
	./scripts/deploy-remote.sh $(if $(U),$(U)@)$(H) $(ARGS)

# ai-proxy Makefile
#
# Works on Linux, macOS, and Windows (Git Bash / MSYS2).
# All docker and go commands run cross-platform.

IMAGE        ?= ghcr.io/jo-hoe/ai-proxy:latest
CONTAINER    ?= ai-proxy
PROXY_PORT   ?= 7655
MGMT_PORT    ?= 7656
PREFIX       ?= proxy-cli:http
TOKEN_PATH   ?= oauth2/token

.PHONY: help build up down logs status test vet push-token get-token run-local

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	/^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

## ── Docker ──────────────────────────────────────────────────────────────────

build: ## Build the Docker image locally
	docker build -t $(IMAGE) .

up: ## Start the container via docker compose (requires config.yaml)
	docker compose up -d

down: ## Stop and remove the container
	docker compose down

logs: ## Tail container logs
	docker logs -f $(CONTAINER)

status: ## Query the management API /status endpoint
	curl -s http://localhost:$(MGMT_PORT)/status | python3 -m json.tool 2>/dev/null || \
	curl -s http://localhost:$(MGMT_PORT)/status

## ── Token management ────────────────────────────────────────────────────────

get-token: ## Extract refresh token from Credential Manager to TOKEN_FILE (Windows only)
	go run ./cmd/get-token -prefix "$(PREFIX)" -output "$(TOKEN_FILE)"

push-token: ## Push token from Credential Manager to running container
	go run ./cmd/push-token \
		-prefix "$(PREFIX)" \
		-token-path "$(TOKEN_PATH)" \
		-url "http://localhost:$(MGMT_PORT)/token"

## ── Development ─────────────────────────────────────────────────────────────

test: ## Run all Go tests
	go test ./...

vet: ## Run go vet
	go vet ./...

run-local: ## Build and run the container locally (uses local image tag)
	IMAGE=proxy:latest $(MAKE) build
	IMAGE=proxy:latest docker compose up -d

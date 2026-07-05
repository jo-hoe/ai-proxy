# ai-proxy Makefile
#
# Works on Linux, macOS, and Windows (native cmd/PowerShell as well as
# Git Bash / MSYS2). All docker and go commands run cross-platform.

include help.mk

IMAGE               ?= ghcr.io/jo-hoe/ai-proxy:latest
CONTAINER           ?= ai-proxy
PROXY_PORT          ?= 7655
MGMT_PORT           ?= 7656
PREFIX              ?= proxy-cli:http
TOKEN_PATH          ?= oauth2/token

# k3d / Helm settings
IMAGE_NAME          := ai-proxy
IMAGE_VERSION       := latest
LOCAL_REGISTRY      := localhost:5000
LOCAL_REGISTRY_HELM := registry.localhost:5000

.PHONY: build up down logs status test vet push-token get-token run-local \
        start-cluster stop-k3d restart-k3d push-k3d start-k3d upgrade-k3d uninstall-k3d

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

## ── k3d / Helm ──────────────────────────────────────────────────────────────

start-cluster: ## Start k3d cluster and local registry
	k3d cluster create --config dev/clusterconfig.yaml

stop-k3d: ## Stop k3d cluster and local registry
	k3d cluster delete --config dev/clusterconfig.yaml

restart-k3d: stop-k3d start-k3d ## Restart k3d cluster and re-install chart

push-k3d: ## Build and push Docker image to local k3d registry
	docker build -t $(IMAGE_NAME):$(IMAGE_VERSION) .
	docker tag $(IMAGE_NAME):$(IMAGE_VERSION) $(LOCAL_REGISTRY)/$(IMAGE_NAME):$(IMAGE_VERSION)
	docker push $(LOCAL_REGISTRY)/$(IMAGE_NAME):$(IMAGE_VERSION)

start-k3d: start-cluster push-k3d ## Create cluster, push image, install Helm chart with dev values
	helm install $(IMAGE_NAME) charts/$(IMAGE_NAME) \
		--set image.repository=$(LOCAL_REGISTRY_HELM)/$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_VERSION) \
		-f dev/config.yaml

upgrade-k3d: push-k3d ## Rebuild image and upgrade Helm release
	helm upgrade $(IMAGE_NAME) charts/$(IMAGE_NAME) \
		--set image.repository=$(LOCAL_REGISTRY_HELM)/$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_VERSION) \
		-f dev/config.yaml

uninstall-k3d: ## Uninstall Helm release from k3d cluster
	helm uninstall $(IMAGE_NAME)

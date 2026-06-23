# ai-proxy

[![CI](https://github.com/jo-hoe/ai-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/jo-hoe/ai-proxy/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/jo-hoe/ai-proxy)](https://goreportcard.com/report/github.com/jo-hoe/ai-proxy)
[![Go Version](https://img.shields.io/github/go-mod/go-version/jo-hoe/ai-proxy)](go.mod)
[![License](https://img.shields.io/github/license/jo-hoe/ai-proxy)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/jo-hoe/ai-proxy)](https://github.com/jo-hoe/ai-proxy/releases/latest)

A Docker container that acts as an injecting reverse proxy for OIDC-protected LLM APIs.
It handles token exchange and automatic rotation — clients connect without credentials.

The refresh token is obtained once on the Windows machine where login was completed
and pushed to the running container via the management API. It is never baked into the
image or passed as an environment variable.

## How it works

1. The container starts and waits for a token (or reads one from a mounted secret).
2. `push-token` extracts a refresh token from Windows Credential Manager and POSTs it to the management API.
3. The container exchanges it for an OIDC access token, then reverse-proxies all incoming requests to `upstream_url` with `Authorization: Bearer <token>` injected.
4. The token is automatically rotated every 50 minutes with zero downtime.

## Quick start

```powershell
# Build the Go tools (Windows)
go build -o get-token.exe  ./cmd/get-token
go build -o push-token.exe ./cmd/push-token
go build -o run.exe        ./cmd/run

# Start the container with your config mounted
.\run.exe

# Push your OIDC refresh token into the running container
# Adjust -prefix to match your credential manager entries
.\push-token.exe -prefix "my-cli:http"
```

Proxy available at `http://localhost:7655/`
Management API at `http://localhost:7656/`

## Configuration

Copy `config.example.yaml` to `config.yaml` and fill in your values.
Mount it at runtime — it is **not** baked into the image:

```bash
docker run -v /path/to/config.yaml:/config.yaml:ro ...
```

```yaml
oidc:
  endpoint: "https://your-oidc-server/oauth2/token"
  client_id: "your-client-id"

proxy:
  port: 7655
  upstream_url: "https://your-upstream-llm-api"
```

## Tools

### `get-token` — extract and save a token to a file

Reads a refresh token from Windows Credential Manager and writes it to a file.

```
get-token.exe [flags]

Flags:
  -prefix  string   credential target prefix  (default: proxy-cli:http)
  -exclude string   comma-separated substrings to exclude  (default: proxy-api-key)
  -output  string   output file path  (default: .token beside the executable)
```

> **Note:** `-prefix` must match the target prefix your CLI tool uses when storing
> credentials. For example, if entries are stored as `my-cli:http://...`,
> pass `-prefix my-cli:http`.

### `push-token` — extract and push a token to a running container

Reads a refresh token from Windows Credential Manager and POSTs it to the management
API. The OIDC endpoint and client ID are parsed directly from the credential target
(format: `<prefix>:<base_url>/<client_id>`), so no extra flags are needed for them.

```
push-token.exe [flags]

Flags:
  -url         string   management API token endpoint  (default: http://localhost:7656/token)
  -prefix      string   credential target prefix  (default: proxy-cli:http)
  -exclude     string   comma-separated substrings to exclude  (default: proxy-api-key)
  -token-path  string   OIDC token path appended to the base URL  (default: oauth2/token)
```

```powershell
# Local container
.\push-token.exe -prefix "my-cli:http"

# Remote container
.\push-token.exe -prefix "my-cli:http" -url http://my-server:7656/token
```

### `run` — build and start the container

Builds the Docker image (if needed) and starts the proxy container.

```
run.exe [flags]

Flags:
  -image      string   Docker image name  (default: proxy:latest)
  -container  string   container name  (default: proxy)
  -proxy-port string   proxy port  (default: from config.yaml)
  -mgmt-port  string   management API port  (default: 7656)
  -token-file string   path to an existing token file (overrides Credential Manager lookup)
  -build               force image rebuild
```

`TOKEN_FILE` environment variable is also honoured as a fallback for `-token-file`.

## Run on a remote machine

```bash
# 1. Pull the pre-built image on the remote machine
docker pull ghcr.io/jo-hoe/ai-proxy:latest

# 2. Run with your config mounted
docker run -d \
  --name ai-proxy \
  -p 7655:7655 \
  -p 7656:7656 \
  -v /path/to/config.yaml:/config.yaml:ro \
  --restart unless-stopped \
  ghcr.io/jo-hoe/ai-proxy:latest

# 3. Push your token from Windows
push-token.exe -prefix "my-cli:http" -url http://remote-host:7656/token
```

## Management API

| Method | Path | Body | Description |
|--------|------|------|-------------|
| `POST` | `/token` | form field `token=<refresh_token>` | Validate via OIDC exchange and hot-swap the access token |
| `GET` | `/status` | — | Returns `running`, `token_expires_at`, `last_refreshed_at`, `uptime_seconds` |

## Docker image

Pre-built images are published to the GitHub Container Registry on every tagged release:

```bash
docker pull ghcr.io/jo-hoe/ai-proxy:latest
```

## Files

| Path | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage container image definition |
| `main.go` / `*.go` | Go management binary (reverse proxy + token rotation) |
| `cmd/get-token/` | Extract refresh token from Credential Manager to a file |
| `cmd/push-token/` | Extract token and POST it to the management API |
| `cmd/run/` | Build and start the container |
| `internal/wincred/` | Win32 Credential Manager bindings |
| `config.example.yaml` | Configuration template |

## Stop

```bash
docker rm -f ai-proxy
```

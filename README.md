# proxy-docker

Runs an AI proxy inside a Docker container, accessible on any machine.

The refresh token is obtained once on the Windows machine where login was completed
and mounted into the container as a read-only file secret at runtime. It is never baked
into the image or passed as an environment variable.

## Quick start

```powershell
# Build the Go tools (Windows)
go build -o get-token.exe  ./cmd/get-token
go build -o push-token.exe ./cmd/push-token
go build -o run.exe        ./cmd/run

# Extract token and start container
.\get-token.exe
.\run.exe
```

Proxy available at `http://localhost:7655/openai/v1/`
Management API at `http://localhost:7656/`

## Tools

### `get-token` — extract and save a token

Reads a refresh token from Windows Credential Manager and writes it to a file.

```
get-token.exe [flags]

Flags:
  -prefix  string   credential target prefix  (default: proxy-cli:http)
  -exclude string   comma-separated substrings to exclude  (default: proxy-api-key)
  -output  string   output file path  (default: .token beside the executable)
```

> **Note:** The `-prefix` must match the credential target prefix used by the CLI tool
> that performed login. For example, if the tool stores credentials as `my-cli:http://...`,
> pass `-prefix my-cli:http`.

### `push-token` — extract and push a token to a running container

Reads a refresh token from Windows Credential Manager and POSTs it directly to the
management API of a running container. Works whether the proxy is already running (token
rotation) or has not yet started (initial provisioning after a tokenless container start).

```
push-token.exe [flags]

Flags:
  -url     string   management API token endpoint  (default: http://localhost:7656/token)
  -prefix  string   credential target prefix  (default: proxy-cli:http)
  -exclude string   comma-separated substrings to exclude  (default: proxy-api-key)
```

Example:
```powershell
.\push-token.exe -url http://my-server:7656/token
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

The `TOKEN_FILE` environment variable is also honoured as a fallback for `-token-file`.

## Run on a remote machine

```bash
# 1. Save the token to a file
get-token.exe -output token

# 2. Copy image and token to the remote machine
docker save proxy:latest | ssh user@remote docker load
scp token user@remote:~/token

# 3. On the remote machine
run.exe -token-file ~/token
```

## Management API

| Method | Path | Description |
|--------|------|-------------|
| `POST /token` | form field `token=<refresh_token>` | Push a new refresh token; validates via OIDC exchange and restarts the proxy (blue/green) |
| `GET /status` | — | Returns `running`, `token_expires_at`, `last_refreshed_at`, `uptime_seconds` |

## Swapping the proxy binary

The container image is built for **Linux/amd64**. Any replacement binary must also be a
Linux/amd64 executable.

The binary's CLI interface, startup args, setup command, and access token env var are all
configurable in `config.yaml` — no rebuild required for the management binary.

**Option 1 — env var (binary already inside the image):**
```bash
docker run ... -e PROXY_BIN=/usr/local/bin/my-tool ...
```

**Option 2 — volume mount:**
```bash
docker run ... -v /path/to/my-tool:/proxy:ro ...
```

The default binary slot is `/proxy` (linux/amd64). The archive expected by `run.exe` is
`proxy-linux.amd64.tar.gz`, which must unpack a single file named `proxy`.

## Configuration

`config.yaml` is baked into the image at build time. All `proxy.*` fields are optional.

```yaml
oidc:
  endpoint: "https://your-oidc-server/oauth2/token"  # OIDC token endpoint
  client_id: "your-client-id"

proxy:
  port: 7655                                          # external proxy port
  bin: /proxy                                         # path to the proxy binary
  start_args: "proxy start --headless --use-keyring=false"  # args for starting the proxy
  token_env: PROXY_OIDC_TOKEN                         # env var the binary reads for the access token
```
## Files

| Path | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage container image definition |
| `main.go` / `*.go` | Go management binary (API server + proxy supervisor) |
| `cmd/get-token/` | Extract refresh token from Credential Manager to a file |
| `cmd/push-token/` | Extract token and POST it to the management API |
| `cmd/run/` | Build and start the container |
| `internal/wincred/` | Win32 Credential Manager bindings |
| `config.yaml` | Non-secret configuration (OIDC endpoint, port) |
| `proxy-linux.amd64.tar.gz` | Proxy binary archive (linux/amd64) |

## Stop

```bash
docker rm -f proxy
```

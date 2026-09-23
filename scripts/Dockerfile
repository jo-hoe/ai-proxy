# Builds the standalone docs refresh tool from the isolated scripts/ module.
# This is NOT the proxy service image — the service has its own Dockerfile at the
# repo root and shares none of this code.
#
# Build (from repo root):
#   docker build -f scripts/Dockerfile -t ai-proxy-refresh .
# Run (mount docs/ so edits land back on the host):
#   docker run --rm -v "$PWD/docs:/docs" ai-proxy-refresh \
#     -config /providers.yaml -doc /docs/providers-and-models.md
FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY scripts/go.mod scripts/go.sum ./
RUN go mod download
COPY scripts/ .
RUN go build -trimpath -ldflags="-s -w" -o /refresh-models ./refresh-models

FROM alpine:3.24
RUN apk add --no-cache ca-certificates
COPY --from=builder /refresh-models /refresh-models
COPY scripts/providers.yaml /providers.yaml
ENTRYPOINT ["/refresh-models"]

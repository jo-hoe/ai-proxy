# Stage 1: build the Go management binary
FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN go build -trimpath -ldflags="-s -w" -o mgmt .

# Stage 2: minimal runtime image
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /src/mgmt /mgmt
RUN chmod +x /mgmt

COPY config.yaml /config.yaml

# PROXY_PORT: external proxy port  (default: from config.yaml)
# MGMT_PORT:  management API port  (default: 7656)
ENV PROXY_PORT="" MGMT_PORT=""

EXPOSE 7655 7656

ENTRYPOINT ["/mgmt"]

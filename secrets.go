package main

import (
	"log/slog"
	"os"
	"strings"
)

// Default secret file paths. Overridable via variables below for testing.
const (
	defaultSecretEndpoint = "/run/secrets/oidc-endpoint"
	defaultSecretClientID = "/run/secrets/oidc-client-id"
	defaultSecretToken    = "/run/secrets/refresh-token"
)

// Mutable path variables — tests override these to point at temp files.
var (
	secretEndpointOverride = defaultSecretEndpoint
	secretClientIDOverride = defaultSecretClientID
	secretTokenOverride    = defaultSecretToken
)

// loadTokenFromSecrets reads OIDC credentials from well-known secret file paths
// and activates the proxy on startup. No-ops silently if any file is absent.
func loadTokenFromSecrets(sup supervisorIface) {
	endpoint := readSecret(secretEndpointOverride)
	clientID := readSecret(secretClientIDOverride)
	token := readSecret(secretTokenOverride)
	if endpoint == "" || clientID == "" || token == "" {
		return
	}
	if err := sup.UpdateToken(endpoint, clientID, token); err != nil {
		slog.Error("startup: failed to load token from secrets", "err", err)
		return
	}
	slog.Info("startup: token loaded from secret files")
}

func readSecret(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

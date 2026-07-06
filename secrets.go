package main

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"syscall"
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
	endpoint, clientID, token, missing := readSecretFiles()
	if missing != "" {
		slog.Debug("startup: skipping secret-based token load", "missing", missing)
		return
	}
	if err := sup.UpdateToken(endpoint, clientID, token); err != nil {
		slog.Error("startup: failed to load token from secrets", "err", err)
		return
	}
	slog.Info("startup: token loaded from secret files")
}

// readSecretFiles reads the three OIDC secret files and returns the first
// missing field name, or an empty string when all are present.
func readSecretFiles() (endpoint, clientID, token, missing string) {
	endpoint = readSecret(secretEndpointOverride)
	if endpoint == "" {
		return "", "", "", "oidc-endpoint"
	}
	clientID = readSecret(secretClientIDOverride)
	if clientID == "" {
		return "", "", "", "oidc-client-id"
	}
	token = readSecret(secretTokenOverride)
	if token == "" {
		return "", "", "", "refresh-token"
	}
	return endpoint, clientID, token, ""
}

func readSecret(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && !isDirectory(err) {
			slog.Warn("startup: could not read secret file", "path", path, "err", err)
		}
		return ""
	}
	return strings.TrimSpace(string(b))
}

// isDirectory reports whether an error from os.ReadFile was caused by the
// path being a directory — expected when optional secret volumes are mounted
// but the Secret does not exist yet (kubelet creates empty dir placeholders).
func isDirectory(err error) bool {
	var pe *os.PathError
	return errors.As(err, &pe) && errors.Is(pe.Err, syscall.EISDIR)
}

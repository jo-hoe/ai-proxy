package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// Standard in-cluster ServiceAccount paths, mounted by kubelet.
// Overridable via variables below for testing.
const (
	defaultSAToken     = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	defaultSACA        = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	defaultSANamespace = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	defaultAPIServer   = "https://kubernetes.default.svc"

	envPersistSecretName      = "PERSIST_SECRET_NAME"
	envPersistSecretNamespace = "PERSIST_SECRET_NAMESPACE"

	contentTypeStrategicMergePatch = "application/strategic-merge-patch+json"
	contentTypeJSON                = "application/json"
)

// Mutable paths for testing.
var (
	saTokenPath     = defaultSAToken
	saCAPath        = defaultSACA
	saNamespacePath = defaultSANamespace
	apiServerURL    = defaultAPIServer
)

// SecretPatcher patches a single k8s Secret from inside the cluster using
// only stdlib HTTP. Nil-safe — callers should tolerate a nil patcher.
type SecretPatcher struct {
	apiURL    string
	namespace string
	secret    string
	token     string
	client    *http.Client
}

// NewSecretPatcher constructs a patcher from the in-cluster ServiceAccount
// files. Returns (nil, nil) when:
//   - PERSIST_SECRET_NAME env is unset (feature disabled), or
//   - the ServiceAccount files are absent (not running in a cluster).
//
// Returns an error only when the environment claims a Secret name but the
// SA files can't be read — that's a misconfiguration worth surfacing.
func NewSecretPatcher() (*SecretPatcher, error) {
	secret := os.Getenv(envPersistSecretName)
	if secret == "" {
		return nil, nil
	}

	tokenBytes, err := os.ReadFile(saTokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("kube: not in a cluster, secret persistence disabled")
			return nil, nil
		}
		return nil, fmt.Errorf("read SA token: %w", err)
	}

	caBytes, err := os.ReadFile(saCAPath)
	if err != nil {
		return nil, fmt.Errorf("read SA CA: %w", err)
	}

	namespace, err := resolveNamespace()
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, errors.New("parse SA CA: no PEM blocks found")
	}

	return &SecretPatcher{
		apiURL:    apiServerURL,
		namespace: namespace,
		secret:    secret,
		token:     strings.TrimSpace(string(tokenBytes)),
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: caPool, MinVersion: tls.VersionTLS12},
			},
		},
	}, nil
}

// resolveNamespace returns the pod namespace from the env override or the
// mounted ServiceAccount file.
func resolveNamespace() (string, error) {
	if ns := os.Getenv(envPersistSecretNamespace); ns != "" {
		slog.Debug("kube: using namespace from env var", "namespace", ns)
		return ns, nil
	}
	nsBytes, err := os.ReadFile(saNamespacePath)
	if err != nil {
		return "", fmt.Errorf("read SA namespace: %w", err)
	}
	return strings.TrimSpace(string(nsBytes)), nil
}

// PatchRefreshToken updates the `refresh-token` key of the Secret via a
// strategic merge patch. Values in Secret.data are base64-encoded per
// the k8s API contract.
func (p *SecretPatcher) PatchRefreshToken(ctx context.Context, refreshToken string) error {
	if p == nil {
		return nil
	}

	body, err := json.Marshal(map[string]any{
		"data": map[string]string{
			"refresh-token": base64.StdEncoding.EncodeToString([]byte(refreshToken)),
		},
	})
	if err != nil {
		return fmt.Errorf("marshal patch body: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/namespaces/%s/secrets/%s", p.apiURL, p.namespace, p.secret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Content-Type", contentTypeStrategicMergePatch)
	req.Header.Set("Accept", contentTypeJSON)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("patch secret: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("patch secret: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}

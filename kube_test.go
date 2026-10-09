package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewSecretPatcher_disabledWhenNoEnv(t *testing.T) {
	t.Setenv("PERSIST_SECRET_NAME", "")
	p, err := NewSecretPatcher()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil patcher when PERSIST_SECRET_NAME is unset")
	}
}

func TestNewSecretPatcher_disabledWhenNotInCluster(t *testing.T) {
	t.Setenv("PERSIST_SECRET_NAME", "my-secret")
	withKubePaths(t, "/nonexistent/token", "/nonexistent/ca.crt", "/nonexistent/ns")
	p, err := NewSecretPatcher()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil patcher when SA files are absent")
	}
}

func TestPatchCredentials_success(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotContentType string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := &SecretPatcher{
		apiURL:    srv.URL,
		namespace: "default",
		secret:    "ai-proxy-token",
		token:     "sa-token-xyz",
		client:    srv.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.PatchCredentials(ctx, "https://oidc.example/token", "client-123", "new-refresh-token"); err != nil {
		t.Fatalf("PatchCredentials: %v", err)
	}

	if gotMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/api/v1/namespaces/default/secrets/ai-proxy-token" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer sa-token-xyz" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotContentType != "application/strategic-merge-patch+json" {
		t.Errorf("content-type = %q", gotContentType)
	}

	var parsed struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	for key, raw := range map[string]string{
		"oidc-endpoint":  "https://oidc.example/token",
		"oidc-client-id": "client-123",
		"refresh-token":  "new-refresh-token",
	} {
		want := base64.StdEncoding.EncodeToString([]byte(raw))
		if parsed.Data[key] != want {
			t.Errorf("data.%s = %q, want %q", key, parsed.Data[key], want)
		}
	}
}

func TestPatchCredentials_omitsEmptyFields(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := &SecretPatcher{
		apiURL:    srv.URL,
		namespace: "default",
		secret:    "ai-proxy-token",
		token:     "sa-token",
		client:    srv.Client(),
	}
	// Only the refresh token is set; endpoint and client id are empty and must
	// be omitted so the patch never nulls existing keys in the Secret.
	if err := p.PatchCredentials(context.Background(), "", "", "rt-only"); err != nil {
		t.Fatalf("PatchCredentials: %v", err)
	}

	var parsed struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := parsed.Data["oidc-endpoint"]; ok {
		t.Error("expected oidc-endpoint to be omitted when empty")
	}
	if _, ok := parsed.Data["oidc-client-id"]; ok {
		t.Error("expected oidc-client-id to be omitted when empty")
	}
	if parsed.Data["refresh-token"] != base64.StdEncoding.EncodeToString([]byte("rt-only")) {
		t.Errorf("refresh-token = %q", parsed.Data["refresh-token"])
	}
}

func TestPatchCredentials_errorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"kind":"Status","message":"forbidden"}`))
	}))
	defer srv.Close()

	p := &SecretPatcher{
		apiURL:    srv.URL,
		namespace: "default",
		secret:    "ai-proxy-token",
		token:     "sa-token",
		client:    srv.Client(),
	}
	err := p.PatchCredentials(context.Background(), "ep", "cid", "rt")
	if err == nil {
		t.Fatal("expected error on 403 response")
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("error = %v, want HTTP 403", err)
	}
}

func TestPatchCredentials_nilPatcher(t *testing.T) {
	// Nil patcher must be a safe no-op — supervisor callers rely on this.
	var p *SecretPatcher
	if err := p.PatchCredentials(context.Background(), "ep", "cid", "rt"); err != nil {
		t.Errorf("nil patcher returned error: %v", err)
	}
}

// withKubePaths overrides the ServiceAccount file paths for a test.
func withKubePaths(t *testing.T, token, ca, ns string) {
	t.Helper()
	origToken, origCA, origNS := saTokenPath, saCAPath, saNamespacePath
	t.Cleanup(func() {
		saTokenPath = origToken
		saCAPath = origCA
		saNamespacePath = origNS
	})
	saTokenPath = token
	saCAPath = ca
	saNamespacePath = ns
}

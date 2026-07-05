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

func TestPatchRefreshToken_success(t *testing.T) {
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
	if err := p.PatchRefreshToken(ctx, "new-refresh-token"); err != nil {
		t.Fatalf("PatchRefreshToken: %v", err)
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
	want := base64.StdEncoding.EncodeToString([]byte("new-refresh-token"))
	if parsed.Data["refresh-token"] != want {
		t.Errorf("data.refresh-token = %q, want %q", parsed.Data["refresh-token"], want)
	}
}

func TestPatchRefreshToken_errorOnNon2xx(t *testing.T) {
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
	err := p.PatchRefreshToken(context.Background(), "rt")
	if err == nil {
		t.Fatal("expected error on 403 response")
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("error = %v, want HTTP 403", err)
	}
}

func TestPatchRefreshToken_nilPatcher(t *testing.T) {
	// Nil patcher must be a safe no-op — supervisor callers rely on this.
	var p *SecretPatcher
	if err := p.PatchRefreshToken(context.Background(), "rt"); err != nil {
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

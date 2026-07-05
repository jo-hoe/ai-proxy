package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTokenFromSecrets_allPresent(t *testing.T) {
	dir := t.TempDir()
	write := func(name, val string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(val), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Trailing newline verifies TrimSpace behaviour.
	write("oidc-endpoint", "https://idp.example.com/token\n")
	write("oidc-client-id", "my-client\n")
	write("refresh-token", "rt-abc123\n")

	calls := 0
	sup := &mockSupervisor{
		updateFn: func(ep, cid, rt string) error {
			calls++
			if ep != "https://idp.example.com/token" {
				t.Errorf("endpoint = %q, want trimmed", ep)
			}
			if cid != "my-client" {
				t.Errorf("clientID = %q, want trimmed", cid)
			}
			if rt != "rt-abc123" {
				t.Errorf("refreshToken = %q, want trimmed", rt)
			}
			return nil
		},
	}

	withSecretPaths(t, dir, "oidc-endpoint", "oidc-client-id", "refresh-token")
	loadTokenFromSecrets(sup)
	if calls != 1 {
		t.Errorf("UpdateToken called %d times, want 1", calls)
	}
}

func TestLoadTokenFromSecrets_missingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "oidc-endpoint"), []byte("ep"), 0600); err != nil {
		t.Fatal(err)
	}
	// Only endpoint file exists; the other two are missing.

	calls := 0
	sup := &mockSupervisor{updateFn: func(_, _, _ string) error { calls++; return nil }}

	withSecretPaths(t, dir, "oidc-endpoint", "oidc-client-id", "refresh-token")
	loadTokenFromSecrets(sup)
	if calls != 0 {
		t.Errorf("UpdateToken called %d times, want 0 when files are missing", calls)
	}
}

func TestLoadTokenFromSecrets_emptyFile(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"oidc-endpoint", "oidc-client-id", "refresh-token"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("   \n"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	calls := 0
	sup := &mockSupervisor{updateFn: func(_, _, _ string) error { calls++; return nil }}

	withSecretPaths(t, dir, "oidc-endpoint", "oidc-client-id", "refresh-token")
	loadTokenFromSecrets(sup)
	if calls != 0 {
		t.Errorf("UpdateToken called %d times, want 0 when files are whitespace-only", calls)
	}
}

// withSecretPaths overrides the secret file paths for the duration of a test
// and restores them via t.Cleanup.
func withSecretPaths(t *testing.T, dir, endpointFile, clientIDFile, tokenFile string) {
	t.Helper()
	origEndpoint := secretEndpointOverride
	origClientID := secretClientIDOverride
	origToken := secretTokenOverride
	t.Cleanup(func() {
		secretEndpointOverride = origEndpoint
		secretClientIDOverride = origClientID
		secretTokenOverride = origToken
	})
	secretEndpointOverride = filepath.Join(dir, endpointFile)
	secretClientIDOverride = filepath.Join(dir, clientIDFile)
	secretTokenOverride = filepath.Join(dir, tokenFile)
}

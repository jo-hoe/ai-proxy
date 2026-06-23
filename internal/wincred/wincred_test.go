package wincred_test

import (
	"testing"

	"github.com/jo-hoe/ai-proxy/internal/wincred"
)

// mockStore implements wincred.Store for testing without Win32.
type mockStore struct {
	creds []wincred.Credential
	err   error
}

func (m *mockStore) FindByPrefix(_ string) ([]wincred.Credential, error) {
	return m.creds, m.err
}

func TestFilter_ExcludesMatches(t *testing.T) {
	creds := []wincred.Credential{
		{Target: "proxy-cli:https://example.com", Token: "tok1"},
		{Target: "proxy-cli:https://example.com/proxy-api-key", Token: "tok2"},
		{Target: "proxy-cli:https://other.com", Token: "tok3"},
	}
	got := wincred.Filter(creds, []string{"proxy-api-key"})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	for _, c := range got {
		if c.Target == "proxy-cli:https://example.com/proxy-api-key" {
			t.Error("excluded credential should not appear in result")
		}
	}
}

func TestFilter_NoExclusions(t *testing.T) {
	creds := []wincred.Credential{
		{Target: "a", Token: "1"},
		{Target: "b", Token: "2"},
	}
	got := wincred.Filter(creds, nil)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestFilter_AllExcluded(t *testing.T) {
	creds := []wincred.Credential{
		{Target: "proxy-cli:api-key", Token: "1"},
	}
	got := wincred.Filter(creds, []string{"api-key"})
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestFilter_EmptyInput(t *testing.T) {
	got := wincred.Filter(nil, []string{"anything"})
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestMockStore_FindByPrefix(t *testing.T) {
	store := &mockStore{creds: []wincred.Credential{
		{Target: "proxy-cli:http://host", Token: "mytoken"},
	}}
	got, err := store.FindByPrefix("proxy-cli:http")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Token != "mytoken" {
		t.Errorf("token = %q, want mytoken", got[0].Token)
	}
}

func TestParseTarget_Valid(t *testing.T) {
	meta, err := wincred.ParseTarget("hai-cli:https://auth.example.com/bdd1034d-faa4-41f5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.BaseURL != "https://auth.example.com" {
		t.Errorf("BaseURL = %q, want https://auth.example.com", meta.BaseURL)
	}
	if meta.ClientID != "bdd1034d-faa4-41f5" {
		t.Errorf("ClientID = %q, want bdd1034d-faa4-41f5", meta.ClientID)
	}
}

func TestParseTarget_DeepPath(t *testing.T) {
	meta, err := wincred.ParseTarget("proxy-cli:https://auth.example.com/path/to/client-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.BaseURL != "https://auth.example.com/path/to" {
		t.Errorf("BaseURL = %q", meta.BaseURL)
	}
	if meta.ClientID != "client-id" {
		t.Errorf("ClientID = %q", meta.ClientID)
	}
}

func TestParseTarget_NoColon(t *testing.T) {
	_, err := wincred.ParseTarget("nocolon")
	if err == nil {
		t.Fatal("expected error for missing colon")
	}
}

func TestParseTarget_NoSlash(t *testing.T) {
	_, err := wincred.ParseTarget("prefix:noslash")
	if err == nil {
		t.Fatal("expected error for missing slash")
	}
}

func TestTokenEndpoint(t *testing.T) {
	meta := wincred.OIDCMeta{BaseURL: "https://auth.example.com"}
	got := meta.TokenEndpoint("oauth2/token")
	want := "https://auth.example.com/oauth2/token"
	if got != want {
		t.Errorf("TokenEndpoint = %q, want %q", got, want)
	}
}

func TestTokenEndpoint_TrailingSlash(t *testing.T) {
	meta := wincred.OIDCMeta{BaseURL: "https://auth.example.com/"}
	got := meta.TokenEndpoint("/oauth2/token")
	want := "https://auth.example.com/oauth2/token"
	if got != want {
		t.Errorf("TokenEndpoint = %q, want %q", got, want)
	}
}

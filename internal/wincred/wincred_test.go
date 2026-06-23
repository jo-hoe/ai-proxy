package wincred_test

import (
	"testing"

	"github.com/oidc-proxy/oidc-proxy/internal/wincred"
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

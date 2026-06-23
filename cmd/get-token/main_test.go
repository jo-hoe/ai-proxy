package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jo-hoe/ai-proxy/internal/wincred"
)

// stubStore implements wincred.Store for tests.
type stubStore struct {
	creds []wincred.Credential
	err   error
}

func (s *stubStore) FindByPrefix(_ string) ([]wincred.Credential, error) {
	return s.creds, s.err
}

func TestRun_NoCredentials(t *testing.T) {
	store := &stubStore{}
	err := run([]string{}, store)
	if err == nil || !strings.Contains(err.Error(), "no credentials found") {
		t.Errorf("expected 'no credentials found' error, got: %v", err)
	}
}

func TestRun_StoreError(t *testing.T) {
	store := &stubStore{err: errors.New("access denied")}
	err := run([]string{}, store)
	if err == nil || !strings.Contains(err.Error(), "credential lookup") {
		t.Errorf("expected 'credential lookup' error, got: %v", err)
	}
}

func TestRun_EmptyToken(t *testing.T) {
	store := &stubStore{creds: []wincred.Credential{
		{Target: "proxy-cli:http://host", Token: ""},
	}}
	err := run([]string{}, store)
	if err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Errorf("expected 'empty token' error, got: %v", err)
	}
}

func TestRun_WritesTokenFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "token")
	store := &stubStore{creds: []wincred.Credential{
		{Target: "proxy-cli:http://host", Token: "mytoken"},
	}}
	if err := run([]string{"--output", out}, store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(data) != "mytoken" {
		t.Errorf("token = %q, want mytoken", string(data))
	}
}

func TestRun_ExcludesApiKey(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "token")
	store := &stubStore{creds: []wincred.Credential{
		{Target: "proxy-cli:http://host/proxy-api-key", Token: "apikey"},
		{Target: "proxy-cli:http://host", Token: "realtoken"},
	}}
	if err := run([]string{"--output", out}, store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(out)
	if string(data) != "realtoken" {
		t.Errorf("token = %q, want realtoken", string(data))
	}
}

func TestRun_CustomPrefix(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "token")
	store := &stubStore{creds: []wincred.Credential{
		{Target: "custom-cli:http://host", Token: "tok"},
	}}
	err := run([]string{"--prefix", "custom-cli:http", "--output", out}, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ", []string{"a", "b"}},
	}
	for _, tc := range cases {
		got := splitCSV(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestWriteTokenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tok")
	if err := writeTokenFile(path, "secret"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "secret" {
		t.Errorf("got %q, want secret", string(data))
	}
	// Permission enforcement is OS-specific; just verify the file exists and is readable.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("stat token file: %v", err)
	}
}



package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jo-hoe/ai-proxy/internal/wincred"
)

type stubStore struct {
	creds []wincred.Credential
	err   error
}

func (s *stubStore) FindByPrefix(_ string) ([]wincred.Credential, error) {
	return s.creds, s.err
}

func TestRun_PostsToken(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		received = r.FormValue("token")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	store := &stubStore{creds: []wincred.Credential{
		{Target: "proxy-cli:http://host", Token: "mytoken"},
	}}
	if err := run([]string{"--url", srv.URL}, store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "mytoken" {
		t.Errorf("received token = %q, want mytoken", received)
	}
}

func TestRun_NoCredentials(t *testing.T) {
	err := run([]string{"--url", "http://unused"}, &stubStore{})
	if err == nil || !strings.Contains(err.Error(), "no credentials found") {
		t.Errorf("expected 'no credentials found', got: %v", err)
	}
}

func TestRun_StoreError(t *testing.T) {
	err := run([]string{"--url", "http://unused"}, &stubStore{err: errors.New("access denied")})
	if err == nil || !strings.Contains(err.Error(), "credential lookup") {
		t.Errorf("expected 'credential lookup', got: %v", err)
	}
}

func TestRun_EmptyToken(t *testing.T) {
	store := &stubStore{creds: []wincred.Credential{
		{Target: "proxy-cli:http://host", Token: ""},
	}}
	err := run([]string{"--url", "http://unused"}, store)
	if err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Errorf("expected 'empty token', got: %v", err)
	}
}

func TestRun_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer srv.Close()

	store := &stubStore{creds: []wincred.Credential{
		{Target: "proxy-cli:http://host", Token: "badtoken"},
	}}
	err := run([]string{"--url", srv.URL}, store)
	if err == nil || !strings.Contains(err.Error(), "HTTP 422") {
		t.Errorf("expected HTTP 422 error, got: %v", err)
	}
}

func TestRun_ExcludesApiKey(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		received = r.FormValue("token")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	store := &stubStore{creds: []wincred.Credential{
		{Target: "proxy-cli:http://host/proxy-api-key", Token: "apikey"},
		{Target: "proxy-cli:http://host", Token: "realtoken"},
	}}
	if err := run([]string{"--url", srv.URL}, store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "realtoken" {
		t.Errorf("received = %q, want realtoken", received)
	}
}

func TestRun_CustomPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	store := &stubStore{creds: []wincred.Credential{
		{Target: "custom-cli:http://host", Token: "tok"},
	}}
	err := run([]string{"--url", srv.URL, "--prefix", "custom-cli:http"}, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b", []string{"a", "b"}},
		{" a , b ", []string{"a", "b"}},
	}
	for _, tc := range cases {
		got := splitCSV(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

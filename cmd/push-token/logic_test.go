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

// validTarget encodes a base URL and client_id in the credential target format.
const validTarget = "proxy-cli:https://auth.example.com/my-client-id"

func TestRun_PostsAllFields(t *testing.T) {
	var gotEndpoint, gotClientID, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotEndpoint = r.FormValue("endpoint")
		gotClientID = r.FormValue("client_id")
		gotToken = r.FormValue("token")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	store := &stubStore{creds: []wincred.Credential{
		{Target: validTarget, Token: "mytoken"},
	}}
	if err := run([]string{"--url", srv.URL}, store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotEndpoint != "https://auth.example.com/oauth2/token" {
		t.Errorf("endpoint = %q", gotEndpoint)
	}
	if gotClientID != "my-client-id" {
		t.Errorf("client_id = %q", gotClientID)
	}
	if gotToken != "mytoken" {
		t.Errorf("token = %q", gotToken)
	}
}

func TestRun_CustomTokenPath(t *testing.T) {
	var gotEndpoint string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotEndpoint = r.FormValue("endpoint")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	store := &stubStore{creds: []wincred.Credential{
		{Target: validTarget, Token: "tok"},
	}}
	if err := run([]string{"--url", srv.URL, "--token-path", "v1/token"}, store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotEndpoint != "https://auth.example.com/v1/token" {
		t.Errorf("endpoint = %q, want https://auth.example.com/v1/token", gotEndpoint)
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
		{Target: validTarget, Token: ""},
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
		{Target: validTarget, Token: "badtoken"},
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
		{Target: "proxy-cli:https://auth.example.com/proxy-api-key", Token: "apikey"},
		{Target: validTarget, Token: "realtoken"},
	}}
	if err := run([]string{"--url", srv.URL}, store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "realtoken" {
		t.Errorf("received = %q, want realtoken", received)
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

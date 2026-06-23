package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOIDCClient_Exchange_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		assertFormValue(t, r, "grant_type", "refresh_token")
		assertFormValue(t, r, "client_id", "test-client")
		assertFormValue(t, r, "refresh_token", "old-refresh")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	result, err := NewOIDCClient(srv.URL, "test-client").Exchange("old-refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q, want new-access", result.AccessToken)
	}
	if result.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken = %q, want new-refresh", result.RefreshToken)
	}
	if result.ExpiresAt.Before(time.Now().Add(3500 * time.Second)) {
		t.Error("ExpiresAt should be ~1 hour from now")
	}
}

func TestOIDCClient_Exchange_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	_, err := NewOIDCClient(srv.URL, "test-client").Exchange("bad-token")
	if err == nil {
		t.Fatal("expected error for HTTP 401")
	}
}

func TestOIDCClient_Exchange_EmptyAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"token_type": "bearer"})
	}))
	defer srv.Close()

	_, err := NewOIDCClient(srv.URL, "test-client").Exchange("some-token")
	if err == nil {
		t.Fatal("expected error for missing access_token")
	}
}

func TestOIDCClient_Exchange_DefaultExpiry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok"})
	}))
	defer srv.Close()

	result, err := NewOIDCClient(srv.URL, "cid").Exchange("rt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExpiresAt.Before(time.Now().Add(3500 * time.Second)) {
		t.Error("ExpiresAt should default to ~1 hour")
	}
}

func assertFormValue(t *testing.T, r *http.Request, field, want string) {
	t.Helper()
	if got := r.FormValue(field); got != want {
		t.Errorf("%s = %q, want %q", field, got, want)
	}
}

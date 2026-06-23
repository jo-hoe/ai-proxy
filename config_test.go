package main

import (
	"testing"
)

func TestParseConfig_Valid(t *testing.T) {
	src := `
oidc:
  endpoint: "https://example.com/token"
  client_id: "my-client"

proxy:
  port: 8080
  upstream_url: "https://api.example.com"
`
	cfg, err := parseConfig(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDC.Endpoint != "https://example.com/token" {
		t.Errorf("endpoint = %q, want https://example.com/token", cfg.OIDC.Endpoint)
	}
	if cfg.OIDC.ClientID != "my-client" {
		t.Errorf("client_id = %q, want my-client", cfg.OIDC.ClientID)
	}
	if cfg.Proxy.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Proxy.Port)
	}
	if cfg.Proxy.UpstreamURL != "https://api.example.com" {
		t.Errorf("upstream_url = %q, want https://api.example.com", cfg.Proxy.UpstreamURL)
	}
}

func TestParseConfig_DefaultPort(t *testing.T) {
	src := `
oidc:
  endpoint: "https://example.com/token"
  client_id: "my-client"
`
	cfg, err := parseConfig(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Proxy.Port != 7655 {
		t.Errorf("port = %d, want default 7655", cfg.Proxy.Port)
	}
}

func TestParseConfig_MissingEndpoint(t *testing.T) {
	src := `
oidc:
  client_id: "my-client"
`
	_, err := parseConfig(src)
	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}
}

func TestParseConfig_MissingClientID(t *testing.T) {
	src := `
oidc:
  endpoint: "https://example.com/token"
`
	_, err := parseConfig(src)
	if err == nil {
		t.Fatal("expected error for missing client_id")
	}
}

func TestParseConfig_InvalidPort(t *testing.T) {
	src := `
oidc:
  endpoint: "https://example.com/token"
  client_id: "my-client"

proxy:
  port: notanumber
`
	_, err := parseConfig(src)
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

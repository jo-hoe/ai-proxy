package main

import (
	"testing"
)

func TestParseConfig_Valid(t *testing.T) {
	src := `
proxy:
  port: 8080
  upstream_url: "https://api.example.com"
`
	cfg, err := parseConfig(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
proxy:
  upstream_url: "https://api.example.com"
`
	cfg, err := parseConfig(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Proxy.Port != 7655 {
		t.Errorf("port = %d, want default 7655", cfg.Proxy.Port)
	}
}

func TestParseConfig_InvalidPort(t *testing.T) {
	_, err := parseConfig("proxy:\n  port: notanumber\n")
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestParseConfig_Empty(t *testing.T) {
	cfg, err := parseConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Proxy.Port != 7655 {
		t.Errorf("port = %d, want default 7655", cfg.Proxy.Port)
	}
}

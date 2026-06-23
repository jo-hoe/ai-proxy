package main

import (
	"reflect"
	"testing"
)

func TestParseConfig_Valid(t *testing.T) {
	src := `
oidc:
  endpoint: "https://example.com/token"
  client_id: "my-client"

proxy:
  port: 8080
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

func TestParseConfig_ProxyFields(t *testing.T) {
	src := `
oidc:
  endpoint: "https://example.com/token"
  client_id: "my-client"

proxy:
  port: 7655
  bin: /usr/local/bin/my-proxy
  start_args: "proxy start --headless --use-keyring=false"
  token_env: MY_TOKEN
`
	cfg, err := parseConfig(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Proxy.Bin != "/usr/local/bin/my-proxy" {
		t.Errorf("bin = %q, want /usr/local/bin/my-proxy", cfg.Proxy.Bin)
	}
	wantStart := []string{"proxy", "start", "--headless", "--use-keyring=false"}
	if !reflect.DeepEqual(cfg.Proxy.StartArgs, wantStart) {
		t.Errorf("start_args = %v, want %v", cfg.Proxy.StartArgs, wantStart)
	}
	if cfg.Proxy.TokenEnv != "MY_TOKEN" {
		t.Errorf("token_env = %q, want MY_TOKEN", cfg.Proxy.TokenEnv)
	}
}

func TestParseConfig_DefaultStartArgs(t *testing.T) {
	src := `
oidc:
  endpoint: "https://example.com/token"
  client_id: "my-client"
`
	cfg, err := parseConfig(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"proxy", "start", "--headless", "--use-keyring=false"}
	if !reflect.DeepEqual(cfg.Proxy.StartArgs, want) {
		t.Errorf("StartArgs = %v, want %v", cfg.Proxy.StartArgs, want)
	}
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"proxy start --headless", []string{"proxy", "start", "--headless"}},
		{"configure claude-code", []string{"configure", "claude-code"}},
		{`"my arg" plain`, []string{"my arg", "plain"}},
		{"'quoted arg' second", []string{"quoted arg", "second"}},
		{"  spaced  args  ", []string{"spaced", "args"}},
		{"single", []string{"single"}},
	}
	for _, tc := range cases {
		got := parseArgs(tc.input)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseArgs(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

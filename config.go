package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds the full application configuration.
type Config struct {
	OIDC  OIDCConfig
	Proxy ProxyConfig
}

// OIDCConfig holds OIDC token endpoint settings.
type OIDCConfig struct {
	Endpoint string
	ClientID string
}

// ProxyConfig holds proxy process settings.
type ProxyConfig struct {
	Port        int
	UpstreamURL string // upstream LLM API base URL
}

// LoadConfig reads and parses the YAML config file at path.
// Only the keys used by this application are extracted; an external YAML
// library is intentionally avoided to keep the binary dependency-free.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	return parseConfig(string(data))
}

func parseConfig(src string) (*Config, error) {
	flat := make(map[string]string)
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := cutKV(line)
		if !ok || v == "" {
			continue
		}
		flat[k] = strings.Trim(v, `"`)
	}

	cfg := &Config{}
	var err error

	if cfg.OIDC.Endpoint, err = require(flat, "endpoint"); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.OIDC.ClientID, err = require(flat, "client_id"); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	if raw, ok := flat["port"]; ok {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil {
			return nil, fmt.Errorf("config: invalid proxy.port %q: %w", raw, convErr)
		}
		cfg.Proxy.Port = n
	}
	if cfg.Proxy.Port == 0 {
		cfg.Proxy.Port = 7655
	}

	cfg.Proxy.UpstreamURL = flat["upstream_url"]

	return cfg, nil
}

func cutKV(line string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(line, ":")
	if !ok {
		return
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

func require(m map[string]string, key string) (string, error) {
	v, ok := m[key]
	if !ok || v == "" {
		return "", fmt.Errorf("missing required key %q", key)
	}
	return v, nil
}

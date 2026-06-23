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
	Bin         string // path to the proxy binary (overrides PROXY_BIN env var)
	StartArgs   []string
	TokenEnv    string
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

	cfg.Proxy.Bin = flat["bin"]

	if raw, ok := flat["start_args"]; ok && raw != "" {
		cfg.Proxy.StartArgs = parseArgs(raw)
	}
	if len(cfg.Proxy.StartArgs) == 0 {
		cfg.Proxy.StartArgs = []string{"proxy", "start", "--headless", "--use-keyring=false"}
	}

	cfg.Proxy.TokenEnv = flat["token_env"]
	if cfg.Proxy.TokenEnv == "" {
		cfg.Proxy.TokenEnv = "PROXY_OIDC_TOKEN"
	}

	return cfg, nil
}

// parseArgs splits a whitespace-separated argument string into a slice.
// Quoted tokens (single or double) are preserved as one argument.
func parseArgs(s string) []string {
	var args []string
	var cur strings.Builder
	inSingle, inDouble := false, false

	for _, ch := range s {
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case (ch == ' ' || ch == '\t') && !inSingle && !inDouble:
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
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

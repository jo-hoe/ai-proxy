package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultRotationMargin = 10 * time.Minute

// defaultClientVersion is the client version reported to the upstream API.
// The upstream rejects callers below its minimum with HTTP 426; bump this when
// that minimum rises. Must be valid semver.
const defaultClientVersion = "1.4.5"

// Config holds the full application configuration.
type Config struct {
	Proxy ProxyConfig
}

// ProxyConfig holds proxy process settings.
type ProxyConfig struct {
	Port           int
	UpstreamURL    string        // upstream LLM API base URL
	RotationMargin time.Duration // how early to rotate before token expiry
	ClientVersion  string        // client version reported to the upstream API
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

	cfg.Proxy.ClientVersion = defaultClientVersion
	if raw, ok := flat["client_version"]; ok && raw != "" {
		cfg.Proxy.ClientVersion = raw
	}

	cfg.Proxy.RotationMargin = defaultRotationMargin
	if raw, ok := flat["rotation_margin_seconds"]; ok {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil {
			return nil, fmt.Errorf("config: invalid rotation_margin_seconds %q: %w", raw, convErr)
		}
		if n <= 0 {
			return nil, fmt.Errorf("config: rotation_margin_seconds must be positive, got %d", n)
		}
		cfg.Proxy.RotationMargin = time.Duration(n) * time.Second
	}

	return cfg, nil
}

func cutKV(line string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(line, ":")
	if !ok {
		return
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

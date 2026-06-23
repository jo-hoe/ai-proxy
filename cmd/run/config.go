package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// proxyConfig holds the proxy port read from config.yaml.
type proxyConfig struct {
	Proxy struct{ Port int }
}

// parseConfigFile reads the proxy port from a config.yaml path.
func parseConfigFile(path string) (*proxyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &proxyConfig{}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		if strings.TrimSpace(k) == "port" {
			n, err := strconv.Atoi(strings.Trim(strings.TrimSpace(v), `"`))
			if err == nil {
				cfg.Proxy.Port = n
			}
		}
	}
	if cfg.Proxy.Port == 0 {
		cfg.Proxy.Port = 7655
	}
	return cfg, nil
}

// toWindowsPath converts a Unix-style path to a Windows absolute path
// for use in Docker Desktop volume mounts on Git Bash / MSYS2.
// /c/Users/... → C:\Users\...
// Absolute Windows paths are returned as-is.
func toWindowsPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	// Already a Windows path (e.g. C:\...)
	if len(path) >= 3 && path[1] == ':' {
		return path, nil
	}
	// MSYS2/Git Bash Unix path: /c/... → C:\...
	if len(path) >= 3 && path[0] == '/' && path[2] == '/' {
		drive := strings.ToUpper(string(path[1]))
		rest := strings.ReplaceAll(path[2:], "/", `\`)
		return drive + ":" + rest, nil
	}
	// Relative or other path — return as-is; Docker will handle it
	return strings.ReplaceAll(path, "/", `\`), nil
}

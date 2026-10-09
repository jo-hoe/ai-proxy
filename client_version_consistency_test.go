package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestClientVersionConsistency guards against drift between config.go's
// defaultClientVersion const and the files that mirror it. If this fails, run:
//
//	go run ./cmd/client-version -set <version>
func TestClientVersionConsistency(t *testing.T) {
	mirrors := []struct {
		path    string
		pattern *regexp.Regexp
	}{
		{"config.example.yaml", regexp.MustCompile(`(?m)^\s*client_version:\s*"(\d+\.\d+\.\d+)"`)},
		{filepath.Join("charts", "ai-proxy", "values.yaml"), regexp.MustCompile(`(?m)^\s*clientVersion:\s*"(\d+\.\d+\.\d+)"`)},
		{filepath.Join("charts", "ai-proxy", "README.md"), regexp.MustCompile("(?m)^\\| config.clientVersion \\| string \\| `\"(\\d+\\.\\d+\\.\\d+)\"`")},
	}
	for _, m := range mirrors {
		data, err := os.ReadFile(m.path)
		if err != nil {
			t.Fatalf("read %s: %v", m.path, err)
		}
		match := m.pattern.FindSubmatch(data)
		if match == nil {
			t.Fatalf("%s: no client-version line matched the expected pattern", m.path)
		}
		if got := string(match[1]); got != defaultClientVersion {
			t.Errorf("%s has client version %q, want %q (run: go run ./cmd/client-version -set %s)",
				m.path, got, defaultClientVersion, defaultClientVersion)
		}
	}
}

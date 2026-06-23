// Package wincred reads credentials from Windows Credential Manager.
// The Store interface abstracts the Win32 layer so callers can be tested
// on any OS using a mock.
package wincred

import (
	"fmt"
	"strings"
)

// Credential holds a single Credential Manager entry.
type Credential struct {
	Target string
	Token  string
}

// OIDCMeta holds the OIDC parameters extracted from a credential target.
type OIDCMeta struct {
	BaseURL  string // e.g. https://host.example.com
	ClientID string // e.g. bdd1034d-a514-4fbd-9e99-1725eefcb9d1
}

// TokenEndpoint builds the full OIDC token endpoint URL by appending
// tokenPath to BaseURL (e.g. "oauth2/token" → "https://host.example.com/oauth2/token").
func (m OIDCMeta) TokenEndpoint(tokenPath string) string {
	base := strings.TrimRight(m.BaseURL, "/")
	path := strings.TrimLeft(tokenPath, "/")
	return base + "/" + path
}

// ParseTarget extracts OIDCMeta from a credential target of the form
// "<prefix>:<base_url>/<client_id>". The prefix (everything up to the first
// colon) is stripped, then the last path segment is taken as the client_id
// and the remainder as the base URL.
func ParseTarget(target string) (OIDCMeta, error) {
	// Strip the prefix (everything up to and including the first colon).
	_, rest, ok := strings.Cut(target, ":")
	if !ok {
		return OIDCMeta{}, fmt.Errorf("wincred: target %q has no prefix separator", target)
	}
	// rest is e.g. https://host.example.com/client-id

	slashIdx := strings.LastIndexByte(rest, '/')
	if slashIdx < 0 {
		return OIDCMeta{}, fmt.Errorf("wincred: target %q: cannot split base URL and client_id", target)
	}
	baseURL := rest[:slashIdx]
	clientID := rest[slashIdx+1:]
	if baseURL == "" || clientID == "" {
		return OIDCMeta{}, fmt.Errorf("wincred: target %q: empty base URL or client_id", target)
	}
	return OIDCMeta{BaseURL: baseURL, ClientID: clientID}, nil
}

// Store abstracts access to the credential store.
type Store interface {
	// FindByPrefix returns all credentials whose target starts with prefix.
	FindByPrefix(prefix string) ([]Credential, error)
}

// Filter returns the subset of credentials whose target does not contain
// any of the excluded substrings.
func Filter(creds []Credential, exclude []string) []Credential {
	out := make([]Credential, 0, len(creds))
	for _, c := range creds {
		if !containsAny(c.Target, exclude) {
			out = append(out, c)
		}
	}
	return out
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

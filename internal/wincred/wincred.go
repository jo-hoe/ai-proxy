// Package wincred reads credentials from Windows Credential Manager.
// The Store interface abstracts the Win32 layer so callers can be tested
// on any OS using a mock.
package wincred

import "strings"

// Credential holds a single Credential Manager entry.
type Credential struct {
	Target string
	Token  string
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

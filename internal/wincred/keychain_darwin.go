//go:build darwin

package wincred

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

// KeychainStore implements Store using the macOS Keychain via the `security`
// command-line tool. It is the darwin counterpart to WindowsStore.
//
// The hai CLI (and compatible clients) store OIDC refresh tokens as generic
// password items whose *service* is the credential prefix (e.g. "hai-cli") and
// whose *account* encodes the OIDC base URL and client id
// (e.g. "https://auth.example.com/<client-id>"). To keep the rest of the
// package OS-agnostic, FindByPrefix reconstructs a target in the same
// "<service>:<account>" form that WindowsStore produces, so ParseTarget and
// Filter work identically on both platforms.
type KeychainStore struct{}

// FindByPrefix returns all generic-password items whose target starts with
// prefix. The prefix is split on its first colon: the part before the colon is
// treated as the Keychain service name, and FindByPrefix enumerates every
// account under that service. Targets are returned as "<service>:<account>".
//
// Examples of prefix values and the service they select:
//
//	"hai-cli:http"       -> service "hai-cli"
//	"proxy-cli:http"     -> service "proxy-cli"
//	"hai-cli"            -> service "hai-cli"
func (KeychainStore) FindByPrefix(prefix string) ([]Credential, error) {
	service, _, _ := strings.Cut(prefix, ":")
	if service == "" {
		return nil, fmt.Errorf("wincred: empty service in prefix %q", prefix)
	}

	accounts, err := keychainAccounts(service)
	if err != nil {
		return nil, err
	}

	var results []Credential
	for _, acct := range accounts {
		target := service + ":" + acct
		// Apply the prefix filter before fetching the secret to avoid
		// unnecessary subprocess spawns and Keychain prompts for non-matching
		// accounts.
		if !strings.HasPrefix(target, prefix) {
			continue
		}
		token, err := keychainSecret(service, acct)
		if err != nil {
			return nil, err
		}
		results = append(results, Credential{Target: target, Token: token})
	}
	return results, nil
}

// keychainAccounts returns the account names of every generic-password item
// for the given service. It parses the attribute dump produced by
// `security dump-keychain` scoped to the login keychain to avoid prompting
// for locked system keychains.
func keychainAccounts(service string) ([]string, error) {
	// Scope to the login keychain only to avoid triggering unlock prompts for
	// system or other keychains. We pass the path explicitly so that locked
	// system keychains are never touched.
	loginKeychain, err := runSecurity("login-keychain")
	if err != nil {
		return nil, err
	}
	keychainPath := strings.Trim(strings.TrimSpace(string(loginKeychain)), `"`)

	// `security find-generic-password` only returns a single (most recent)
	// match, so enumerate via dump-keychain and parse the attribute blocks.
	out, err := runSecurity("dump-keychain", keychainPath)
	if err != nil {
		return nil, err
	}
	return parseAccountsForService(out, service), nil
}

// parseAccountsForService scans `security dump-keychain` output and returns the
// "acct" values of every generic-password item whose "svce" equals service.
// Items are separated by blank lines within a keychain block; the "keychain:"
// header line appears once per keychain file, not once per item.
func parseAccountsForService(dump []byte, service string) []string {
	var accounts []string
	var curAcct string
	var curSvce string

	flush := func() {
		if curSvce == service && curAcct != "" {
			accounts = append(accounts, curAcct)
		}
		curAcct, curSvce = "", ""
	}

	for _, line := range strings.Split(string(dump), "\n") {
		// Normalize CRLF in case output ever has Windows line endings.
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			// Blank line separates items within the same keychain block.
			flush()
		case strings.HasPrefix(trimmed, "keychain:"):
			// New keychain file header — also flush any pending item.
			flush()
		case strings.HasPrefix(trimmed, `"acct"<blob>=`):
			if v, ok := parseBlobValue(trimmed); ok {
				curAcct = v
			}
		case strings.HasPrefix(trimmed, `"svce"<blob>=`):
			if v, ok := parseBlobValue(trimmed); ok {
				curSvce = v
			}
		}
	}
	flush()
	return accounts
}

// parseBlobValue extracts the quoted value from a dump-keychain attribute line
// of the form `"acct"<blob>="the value"`. It returns ("", false) for NULL or
// non-string (hex) values, which cannot be matched reliably.
// The value is delimited by the outermost pair of quotes: the first `"` after
// `=` and the last `"` in the line. This correctly handles values that contain
// embedded `=` characters (e.g. URLs with query strings).
func parseBlobValue(line string) (string, bool) {
	_, rhs, ok := strings.Cut(line, "=")
	if !ok {
		return "", false
	}
	rhs = strings.TrimSpace(rhs)
	if !strings.HasPrefix(rhs, `"`) {
		// <NULL> or hex (0x...) — not a plain string value.
		return "", false
	}
	// Strip the leading quote; the value ends at the trailing quote.
	// Use Index (not LastIndex) on the remainder so that an embedded `\"`
	// sequence doesn't cause us to find a quote mid-value.
	// dump-keychain escapes embedded double-quotes as `\"`, so we scan for
	// the first unescaped closing quote.
	inner := rhs[1:]
	end := strings.LastIndex(inner, `"`)
	if end < 0 {
		return inner, true
	}
	// Walk back past any backslash-escaped quotes to find the real end.
	for end > 0 && inner[end-1] == '\\' {
		prev := strings.LastIndex(inner[:end-1], `"`)
		if prev < 0 {
			return inner, true
		}
		end = prev
	}
	return inner[:end], true
}

// keychainSecret returns the secret (token) for a specific service/account
// generic-password item.
func keychainSecret(service, account string) (string, error) {
	out, err := runSecurity("find-generic-password", "-s", service, "-a", account, "-w")
	if err != nil {
		return "", fmt.Errorf("wincred: read secret for %s/%s: %w", service, account, err)
	}
	// -w prints the raw secret followed by a trailing newline.
	raw := strings.TrimRight(string(out), "\r\n")
	return decodeKeyringValue(raw), nil
}

// keyringBase64Prefix is the marker the go-keyring library (used by the hai
// CLI) prepends when it base64-encodes a secret before storing it in the
// Keychain. The actual secret is the base64 payload following the prefix.
const keyringBase64Prefix = "go-keyring-base64:"

// decodeKeyringValue unwraps a value stored by go-keyring. When the value
// carries the "go-keyring-base64:" marker, the payload after it is
// base64-decoded to recover the original secret. Values without the marker are
// returned unchanged, so this is safe for tokens written by other clients
// (e.g. the Windows Credential Manager, which stores the raw token).
func decodeKeyringValue(v string) string {
	payload, ok := strings.CutPrefix(v, keyringBase64Prefix)
	if !ok {
		return v
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		// Not valid base64 despite the marker — fall back to the raw value
		// rather than losing the secret entirely.
		return v
	}
	return string(decoded)
}

// runSecurity executes the macOS `security` tool and returns its stdout.
// If the command exits non-zero, stderr is included in the error. If the
// command exits zero but wrote to stderr (e.g. partial-dump warnings for
// locked items), the warning is surfaced as an error so callers never
// silently receive an incomplete result.
func runSecurity(args ...string) ([]byte, error) {
	cmd := exec.Command("security", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	stderrMsg := strings.TrimSpace(stderr.String())
	if err != nil {
		if stderrMsg != "" {
			return nil, fmt.Errorf("security %s: %s: %w", strings.Join(args, " "), stderrMsg, err)
		}
		return nil, fmt.Errorf("security %s: %w", strings.Join(args, " "), err)
	}
	if stderrMsg != "" {
		return nil, fmt.Errorf("security %s: %s", strings.Join(args, " "), stderrMsg)
	}
	return stdout.Bytes(), nil
}

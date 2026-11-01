//go:build darwin

package wincred

import (
	"encoding/base64"
	"testing"
)

const sampleDump = `keychain: "/Users/x/Library/Keychains/login.keychain-db"

class: "genp"
attributes:
    0x00000007 <blob>="hai-cli"
    "acct"<blob>="proxy-api-key"
    "svce"<blob>="hai-cli"

class: "genp"
attributes:
    0x00000007 <blob>="hai-cli"
    "acct"<blob>="https://hyperspace.accounts.ondemand.com/bdd1034d-a514-4fbd-9e99-1725eefcb9d1"
    "svce"<blob>="hai-cli"

class: "genp"
attributes:
    "acct"<blob>="someone"
    "svce"<blob>="unrelated-service"

class: "genp"
attributes:
    "acct"<blob>=<NULL>
    "svce"<blob>="MetadataKeychain"
`

func TestParseAccountsForService(t *testing.T) {
	got := parseAccountsForService([]byte(sampleDump), "hai-cli")
	want := []string{
		"proxy-api-key",
		"https://hyperspace.accounts.ondemand.com/bdd1034d-a514-4fbd-9e99-1725eefcb9d1",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d accounts %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("account[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseAccountsForService_NoMatch(t *testing.T) {
	got := parseAccountsForService([]byte(sampleDump), "does-not-exist")
	if len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

func TestParseAccountsForService_CRLF(t *testing.T) {
	// Simulate dump output with CRLF line endings — service names must not
	// gain a trailing \r that breaks the equality check.
	crlfDump := "keychain: \"/x/login.keychain-db\"\r\n\r\nclass: \"genp\"\r\nattributes:\r\n    \"acct\"<blob>=\"myacct\"\r\n    \"svce\"<blob>=\"mysvc\"\r\n"
	got := parseAccountsForService([]byte(crlfDump), "mysvc")
	if len(got) != 1 || got[0] != "myacct" {
		t.Fatalf("got %v, want [myacct]", got)
	}
}

func TestParseBlobValue(t *testing.T) {
	cases := []struct {
		line    string
		want    string
		wantOK  bool
	}{
		{`    "acct"<blob>="hello"`, "hello", true},
		{`    "svce"<blob>="hai-cli"`, "hai-cli", true},
		{`    "acct"<blob>="https://h/id"`, "https://h/id", true},
		{`    "acct"<blob>="url?a=b"`, "url?a=b", true},
		{`    "acct"<blob>=<NULL>`, "", false},
		{`    "gena"<blob>=0xDEADBEEF`, "", false},
	}
	for _, tc := range cases {
		got, ok := parseBlobValue(tc.line)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("parseBlobValue(%q) = (%q,%v), want (%q,%v)", tc.line, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestDecodeKeyringValue(t *testing.T) {
	const token = "me2-a-realistic-opaque-refresh-token-value-E"
	wrapped := keyringBase64Prefix + base64.StdEncoding.EncodeToString([]byte(token))

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"go-keyring base64 wrapper is decoded", wrapped, token},
		{"raw token without marker is unchanged", token, token},
		{"empty string", "", ""},
		{"marker with invalid base64 falls back to raw", keyringBase64Prefix + "!!!notb64", keyringBase64Prefix + "!!!notb64"},
		{"windows-style raw token unchanged", "AQAAAraw-token", "AQAAAraw-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeKeyringValue(tc.in); got != tc.want {
				t.Errorf("decodeKeyringValue() = %q, want %q", got, tc.want)
			}
		})
	}
}

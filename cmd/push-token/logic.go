package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jo-hoe/ai-proxy/internal/wincred"
)

const defaultManagementURL = "http://localhost:7656/token"
const defaultTokenPath = "oauth2/token"

func run(args []string, store wincred.Store) error {
	fs := flag.NewFlagSet("push-token", flag.ContinueOnError)
	apiURL := fs.String("url", defaultManagementURL, "management API token endpoint URL")
	tokenPath := fs.String("token-path", defaultTokenPath, "OIDC token endpoint path appended to the base URL from the credential target")
	prefix := fs.String("prefix", "proxy-cli:http", "credential target prefix to search for")
	exclude := fs.String("exclude", "proxy-api-key", "comma-separated substrings to exclude from results")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cred, err := selectCredential(store, *prefix, splitCSV(*exclude))
	if err != nil {
		return err
	}

	meta, err := wincred.ParseTarget(cred.Target)
	if err != nil {
		return fmt.Errorf("parse credential target: %w", err)
	}
	endpoint := meta.TokenEndpoint(*tokenPath)

	fmt.Printf("Using credential: %s\n", cred.Target)
	fmt.Printf("OIDC endpoint:    %s\n", endpoint)
	fmt.Printf("Client ID:        %s\n", meta.ClientID)

	if err := postToken(*apiURL, endpoint, meta.ClientID, cred.Token); err != nil {
		return err
	}

	fmt.Printf("Token posted to %s\n", *apiURL)
	return nil
}

// selectCredential finds and returns the single matching credential.
func selectCredential(store wincred.Store, prefix string, exclude []string) (wincred.Credential, error) {
	creds, err := store.FindByPrefix(prefix)
	if err != nil {
		return wincred.Credential{}, fmt.Errorf("credential lookup: %w", err)
	}
	creds = wincred.Filter(creds, exclude)
	if len(creds) == 0 {
		return wincred.Credential{}, fmt.Errorf("no credentials found matching prefix %q — ensure SSO login has been completed", prefix)
	}
	if strings.TrimSpace(creds[0].Token) == "" {
		return wincred.Credential{}, fmt.Errorf("credential %q has an empty token — re-run SSO login", creds[0].Target)
	}
	return creds[0], nil
}

// postToken sends the endpoint, client ID and refresh token to the management API.
func postToken(apiURL, endpoint, clientID, token string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	body := url.Values{
		"endpoint":  {endpoint},
		"client_id": {clientID},
		"token":     {token},
	}
	resp, err := client.PostForm(apiURL, body)
	if err != nil {
		return fmt.Errorf("post token to %s: %w", apiURL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("management API returned HTTP %d: %s", resp.StatusCode, raw)
	}
	return nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

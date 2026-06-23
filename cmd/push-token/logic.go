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

func run(args []string, store wincred.Store) error {
	fs := flag.NewFlagSet("push-token", flag.ContinueOnError)
	apiURL := fs.String("url", defaultManagementURL, "management API token endpoint URL")
	prefix := fs.String("prefix", "proxy-cli:http", "credential target prefix to search for")
	exclude := fs.String("exclude", "proxy-api-key", "comma-separated substrings to exclude from results")

	if err := fs.Parse(args); err != nil {
		return err
	}

	token, target, err := extractToken(store, *prefix, splitCSV(*exclude))
	if err != nil {
		return err
	}
	fmt.Printf("Using credential: %s\n", target)

	if err := postToken(*apiURL, token); err != nil {
		return err
	}

	fmt.Printf("Token posted to %s\n", *apiURL)
	return nil
}

// extractToken finds and returns the first matching credential token.
func extractToken(store wincred.Store, prefix string, exclude []string) (token, target string, err error) {
	creds, err := store.FindByPrefix(prefix)
	if err != nil {
		return "", "", fmt.Errorf("credential lookup: %w", err)
	}
	creds = wincred.Filter(creds, exclude)
	if len(creds) == 0 {
		return "", "", fmt.Errorf("no credentials found matching prefix %q — ensure SSO login has been completed", prefix)
	}
	if strings.TrimSpace(creds[0].Token) == "" {
		return "", "", fmt.Errorf("credential %q has an empty token — re-run SSO login", creds[0].Target)
	}
	return creds[0].Token, creds[0].Target, nil
}

// postToken sends the token to the management API via POST form.
func postToken(apiURL, token string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	body := url.Values{"token": {token}}
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

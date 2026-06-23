package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jo-hoe/ai-proxy/internal/wincred"
)

func run(args []string, store wincred.Store) error {
	fs := flag.NewFlagSet("get-token", flag.ContinueOnError)
	prefix := fs.String("prefix", "proxy-cli:http", "credential target prefix to search for")
	exclude := fs.String("exclude", "proxy-api-key", "comma-separated substrings to exclude from results")
	output := fs.String("output", "", "path to write the token file (default: .token beside the executable)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	outPath := *output
	if outPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}
		outPath = filepath.Join(filepath.Dir(exe), ".token")
	}

	creds, err := store.FindByPrefix(*prefix)
	if err != nil {
		return fmt.Errorf("credential lookup: %w", err)
	}
	creds = wincred.Filter(creds, splitCSV(*exclude))

	if len(creds) == 0 {
		return fmt.Errorf("no credentials found matching prefix %q — ensure SSO login has been completed", *prefix)
	}

	chosen, err := selectCredential(creds)
	if err != nil {
		return err
	}

	if strings.TrimSpace(chosen.Token) == "" {
		return fmt.Errorf("credential %q has an empty token — re-run SSO login", chosen.Target)
	}

	if err := writeTokenFile(outPath, chosen.Token); err != nil {
		return err
	}

	fmt.Printf("Token saved to: %s\n", outPath)
	fmt.Printf("Mount it with: -v %s:/run/secrets/refresh-token:ro\n", outPath)
	return nil
}

// selectCredential returns the single credential, or prompts when multiple exist.
func selectCredential(creds []wincred.Credential) (wincred.Credential, error) {
	if len(creds) == 1 {
		fmt.Printf("Using credential: %s\n", creds[0].Target)
		return creds[0], nil
	}

	fmt.Println("Multiple credentials found. Select one:")
	for i, c := range creds {
		fmt.Printf("  [%d] %s\n", i, c.Target)
	}

	fmt.Print("Enter number: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return wincred.Credential{}, fmt.Errorf("no input received")
	}
	idx, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || idx < 0 || idx >= len(creds) {
		return wincred.Credential{}, fmt.Errorf("invalid selection %q", scanner.Text())
	}
	return creds[idx], nil
}

// writeTokenFile writes token to path with owner-only permissions.
func writeTokenFile(path, token string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create token file: %w", err)
	}
	_, writeErr := fmt.Fprint(f, token)
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("write token file: %w", writeErr)
	}
	return closeErr
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

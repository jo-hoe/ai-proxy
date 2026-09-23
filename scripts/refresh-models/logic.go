package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultBase = "https://ai-proxy.johoe.duckdns.org"

// Provider describes one auto-refreshed vendor: where to fetch its models, how to
// parse the response, and which markdown section to rewrite.
type Provider struct {
	Name    string `yaml:"name"`
	Slug    string `yaml:"slug"`
	Path    string `yaml:"path"`
	Format  string `yaml:"format"` // anthropic | openai | gemini
	Heading string `yaml:"heading"`
}

// Config is scripts/providers.yaml.
type Config struct {
	Providers  []Provider `yaml:"providers"`
	Candidates []string   `yaml:"candidates"`
}

// Model is the common shape parsed from every provider format. Gemini-only fields
// stay zero/empty for the other formats.
type Model struct {
	ID           string
	DisplayName  string
	InputTokens  int
	OutputTokens int
	Methods      []string
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("refresh-models", flag.ContinueOnError)
	base := fs.String("base", defaultBase, "proxy base URL")
	configPath := fs.String("config", "scripts/providers.yaml", "path to providers.yaml")
	docPath := fs.String("doc", "docs/providers-and-models.md", "path to the markdown doc to update")
	check := fs.Bool("check", false, "do not write; exit non-zero if the doc would change")
	timeout := fs.Duration("timeout", 20*time.Second, "per-request HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}

	docBytes, err := os.ReadFile(*docPath)
	if err != nil {
		return fmt.Errorf("read doc: %w", err)
	}
	// Normalize to LF for matching; restore the original ending style on write so
	// we don't rewrite every line of a CRLF file.
	crlf := strings.Contains(string(docBytes), "\r\n")
	original := string(docBytes)
	doc := strings.ReplaceAll(original, "\r\n", "\n")
	normalized := doc

	client := &http.Client{Timeout: *timeout}
	today := time.Now().Format("2006-01-02")

	for _, p := range cfg.Providers {
		models, err := fetchModels(client, *base, p)
		if err != nil {
			return fmt.Errorf("provider %s: %w", p.Name, err)
		}
		doc, err = spliceModelsTable(doc, p, models, today)
		if err != nil {
			return fmt.Errorf("provider %s: %w", p.Name, err)
		}
	}

	newVendors := probeCandidates(client, *base, cfg.Candidates)
	if len(newVendors) > 0 {
		doc = dropFromNotExposed(doc, newVendors)
		for _, v := range newVendors {
			fmt.Fprintf(stdout, "NEW VENDOR DETECTED: %q returns 2xx — add it to %s so its models get refreshed\n", v, *configPath)
		}
	}

	if doc == normalized {
		fmt.Fprintln(stdout, "docs/providers-and-models.md is up to date")
		return nil
	}

	if *check {
		return fmt.Errorf("doc is out of date; re-run without -check to update %s", *docPath)
	}

	out := doc
	if crlf {
		out = strings.ReplaceAll(doc, "\n", "\r\n")
	}
	if err := os.WriteFile(*docPath, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write doc: %w", err)
	}
	fmt.Fprintf(stdout, "updated %s\n", *docPath)
	return nil
}

func loadConfig(path string) (Config, error) {
	var cfg Config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func fetchModels(client *http.Client, base string, p Provider) ([]Model, error) {
	body, err := httpGet(client, base+p.Path)
	if err != nil {
		return nil, err
	}
	models, err := parseModels(p.Format, body)
	if err != nil {
		return nil, err
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func httpGet(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func parseModels(format string, body []byte) ([]Model, error) {
	switch format {
	case "anthropic":
		var r struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		out := make([]Model, 0, len(r.Data))
		for _, m := range r.Data {
			out = append(out, Model{ID: m.ID, DisplayName: m.DisplayName})
		}
		return out, nil
	case "openai":
		var r struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		out := make([]Model, 0, len(r.Data))
		for _, m := range r.Data {
			out = append(out, Model{ID: m.ID})
		}
		return out, nil
	case "gemini":
		var r struct {
			Models []struct {
				BaseModelID                string   `json:"baseModelId"`
				Name                       string   `json:"name"`
				InputTokenLimit            int      `json:"inputTokenLimit"`
				OutputTokenLimit           int      `json:"outputTokenLimit"`
				SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, err
		}
		out := make([]Model, 0, len(r.Models))
		for _, m := range r.Models {
			id := m.BaseModelID
			if id == "" {
				id = strings.TrimPrefix(m.Name, "models/")
			}
			out = append(out, Model{
				ID:           id,
				InputTokens:  m.InputTokenLimit,
				OutputTokens: m.OutputTokenLimit,
				Methods:      m.SupportedGenerationMethods,
			})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
}

// renderTable produces the markdown table body (header + rows) for a provider,
// matching the column layout already used in the doc for that format.
func renderTable(format string, models []Model) string {
	var b strings.Builder
	switch format {
	case "anthropic":
		b.WriteString("| Model ID | Display name |\n|---|---|\n")
		for _, m := range models {
			fmt.Fprintf(&b, "| `%s` | %s |\n", m.ID, m.DisplayName)
		}
	case "openai":
		b.WriteString("| Model ID |\n|---|\n")
		for _, m := range models {
			fmt.Fprintf(&b, "| `%s` |\n", m.ID)
		}
	case "gemini":
		b.WriteString("| Model ID | Input tokens | Output tokens | Methods |\n|---|---|---|---|\n")
		for _, m := range models {
			in := formatInt(m.InputTokens)
			out := "—"
			if m.OutputTokens > 0 {
				out = formatInt(m.OutputTokens)
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", m.ID, in, out, strings.Join(m.Methods, ", "))
		}
	}
	return b.String()
}

func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}

// modelsBlockRe matches a provider's "### Models (as of <date>)" heading plus the
// contiguous markdown table that immediately follows it (blank lines allowed
// between heading and table). It intentionally stops at the first table so the
// Anthropic alias table below is left untouched.
var modelsBlockRe = regexp.MustCompile(`(?m)^### Models \(as of \d{4}-\d{2}-\d{2}\)\n+((?:\|.*\n)+)`)

// spliceModelsTable finds the provider's section (by heading), then replaces the
// "### Models (as of …)" date and table within that section only.
func spliceModelsTable(doc string, p Provider, models []Model, today string) (string, error) {
	start, end, err := sectionBounds(doc, p.Heading)
	if err != nil {
		return "", err
	}
	section := doc[start:end]

	loc := modelsBlockRe.FindStringSubmatchIndex(section)
	if loc == nil {
		return "", fmt.Errorf("no \"### Models (as of …)\" block found in section")
	}
	newBlock := fmt.Sprintf("### Models (as of %s)\n\n%s", today, renderTable(p.Format, models))
	newSection := section[:loc[0]] + newBlock + section[loc[1]:]
	return doc[:start] + newSection + doc[end:], nil
}

// sectionBounds returns the [start,end) byte range of the "## <heading>" section,
// i.e. from its heading line up to (but not including) the next "## " heading.
func sectionBounds(doc, heading string) (int, int, error) {
	marker := "## " + heading
	start := strings.Index(doc, marker)
	if start < 0 {
		return 0, 0, fmt.Errorf("section heading %q not found", heading)
	}
	rest := doc[start+len(marker):]
	next := regexp.MustCompile(`(?m)^## `).FindStringIndex(rest)
	end := len(doc)
	if next != nil {
		end = start + len(marker) + next[0]
	}
	return start, end, nil
}

func probeCandidates(client *http.Client, base string, candidates []string) []string {
	var live []string
	for _, c := range candidates {
		resp, err := client.Get(base + "/" + c + "/v1/models")
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			live = append(live, c)
		}
	}
	return live
}

// dropFromNotExposed removes any newly-live vendor names from the comma-separated
// "Providers not currently exposed" list. Matching is case-insensitive and also
// checks the slug against the human names in the list.
func dropFromNotExposed(doc string, live []string) string {
	const marker = "## Providers not currently exposed"
	idx := strings.Index(doc, marker)
	if idx < 0 {
		return doc
	}
	// The list is the free-text paragraph after the intro sentence; we only strip
	// tokens, leaving the surrounding prose intact.
	for _, v := range live {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(v) + `\b,?\s*`)
		section := doc[idx:]
		section = re.ReplaceAllString(section, "")
		doc = doc[:idx] + section
	}
	return doc
}

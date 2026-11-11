package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultBase = "https://ai-proxy.johoe.duckdns.org"

// Provider describes one vendor: where to fetch its models, how to parse the
// response, and the prose rendered above its model table.
type Provider struct {
	Name        string   `yaml:"name"`
	Slug        string   `yaml:"slug"`
	Path        string   `yaml:"path"`
	Format      string   `yaml:"format"` // anthropic | openai | gemini
	Heading     string   `yaml:"heading"`
	Description string   `yaml:"description"`
	Endpoints   []string `yaml:"endpoints"`
}

// Config is scripts/providers.yaml.
type Config struct {
	Intro      string     `yaml:"intro"`
	Footer     string     `yaml:"footer"`
	Providers  []Provider `yaml:"providers"`
	Candidates []string   `yaml:"candidates"`
}

// Model is the common shape parsed from every provider format.
type Model struct {
	ID string
}

// section is one rendered provider block passed to the doc template.
type section struct {
	Provider Provider
	Table    string
}

// docData is the full template input.
type docData struct {
	Intro      string
	Footer     string
	Date       string
	Sections   []section
	NotExposed []string
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("refresh-models", flag.ContinueOnError)
	base := fs.String("base", defaultBase, "proxy base URL")
	configPath := fs.String("config", "scripts/providers.yaml", "path to providers.yaml")
	docPath := fs.String("doc", "docs/providers-and-models.md", "path to the markdown doc to write")
	check := fs.Bool("check", false, "do not write; exit non-zero if the doc would change")
	timeout := fs.Duration("timeout", 20*time.Second, "per-request HTTP timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: *timeout}
	today := time.Now().Format("2006-01-02")

	data := docData{
		Intro:  strings.TrimRight(cfg.Intro, "\n"),
		Footer: strings.TrimRight(cfg.Footer, "\n"),
		Date:   today,
	}

	for _, p := range cfg.Providers {
		models, err := fetchModels(client, *base, p)
		if err != nil {
			return fmt.Errorf("provider %s: %w", p.Name, err)
		}
		data.Sections = append(data.Sections, section{
			Provider: p,
			Table:    renderTable(models),
		})
	}

	live := probeCandidates(client, *base, cfg.Candidates)
	liveSet := make(map[string]bool, len(live))
	for _, v := range live {
		liveSet[v] = true
		fmt.Fprintf(stdout, "NEW VENDOR DETECTED: %q returns 2xx — add it to %s so its models get refreshed\n", v, *configPath) //nolint:errcheck // best-effort stdout notice
	}
	for _, c := range cfg.Candidates {
		if !liveSet[c] {
			data.NotExposed = append(data.NotExposed, c)
		}
	}

	rendered, err := renderDoc(data)
	if err != nil {
		return err
	}

	existing, err := os.ReadFile(*docPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read doc: %w", err)
	}
	// Preserve the file's existing line-ending style so we don't rewrite every
	// line of a CRLF-checked-out file.
	crlf := strings.Contains(string(existing), "\r\n")
	normalizedExisting := strings.ReplaceAll(string(existing), "\r\n", "\n")

	if rendered == normalizedExisting {
		fmt.Fprintf(stdout, "%s is up to date\n", *docPath) //nolint:errcheck // best-effort stdout notice
		return nil
	}

	if *check {
		return fmt.Errorf("doc is out of date; re-run without -check to update %s", *docPath)
	}

	out := rendered
	if crlf {
		out = strings.ReplaceAll(rendered, "\n", "\r\n")
	}
	if err := os.WriteFile(*docPath, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write doc: %w", err)
	}
	fmt.Fprintf(stdout, "updated %s\n", *docPath) //nolint:errcheck // best-effort stdout notice
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
	resp, err := client.Get(url) //nolint:noctx // standalone CLI tool, no request context to thread
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
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
				BaseModelID string `json:"baseModelId"`
				Name        string `json:"name"`
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
			out = append(out, Model{ID: id})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
}

// renderTable produces a single-column markdown table of model IDs.
func renderTable(models []Model) string {
	var b strings.Builder
	b.WriteString("| Model ID |\n|---|\n")
	for _, m := range models {
		fmt.Fprintf(&b, "| `%s` |\n", m.ID)
	}
	return b.String()
}

// docTmpl renders the full providers-and-models.md. text/template (not
// html/template) so backticks and pipes in the markdown are not escaped.
const docTmpl = `# API surface

{{ .Intro }}
{{ range .Sections }}
## {{ .Provider.Heading }}

{{ .Provider.Description }}
{{ range .Provider.Endpoints }}- {{ . }}
{{ end }}
### Models (as of {{ $.Date }})

{{ .Table }}{{ end }}
## Providers not currently exposed

The following vendor prefixes returned a non-2xx status at generation time and
are not documented above. If one starts responding, it is added automatically on
the next ` + "`make generate-docs`" + `:

{{ range .NotExposed }}- ` + "`{{ . }}`" + `
{{ end }}
{{ .Footer }}
`

func renderDoc(data docData) (string, error) {
	t, err := template.New("doc").Parse(docTmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}
	// Collapse any run of 3+ newlines to a blank-line separator and ensure a
	// single trailing newline, so template whitespace stays tidy.
	out := buf.String()
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimRight(out, "\n") + "\n", nil
}

func probeCandidates(client *http.Client, base string, candidates []string) []string {
	var live []string
	for _, c := range candidates {
		resp, err := client.Get(base + "/" + c + "/v1/models") //nolint:noctx // standalone CLI tool, no request context to thread
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			live = append(live, c)
		}
	}
	return live
}

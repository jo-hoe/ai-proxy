package main

import (
	"strings"
	"testing"
)

func TestParseModels_Anthropic(t *testing.T) {
	body := []byte(`{"data":[{"id":"anthropic--claude-4.5-haiku","display_name":"Claude 4.5 Haiku"},{"id":"anthropic--claude-4.8-opus","display_name":"Claude 4.8 Opus"}]}`)
	got, err := parseModels("anthropic", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d models, want 2", len(got))
	}
	if got[0].ID != "anthropic--claude-4.5-haiku" {
		t.Errorf("unexpected first model: %+v", got[0])
	}
}

func TestParseModels_OpenAI(t *testing.T) {
	body := []byte(`{"object":"list","data":[{"id":"gpt-5"},{"id":"gpt-5-mini"}]}`)
	got, err := parseModels("openai", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "gpt-5" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestParseModels_Gemini(t *testing.T) {
	body := []byte(`{"models":[{"baseModelId":"gemini-2.5-flash"},{"name":"models/gemini-embedding"}]}`)
	got, err := parseModels("gemini", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d models, want 2", len(got))
	}
	if got[0].ID != "gemini-2.5-flash" {
		t.Errorf("unexpected first model: %+v", got[0])
	}
	// Falls back to name when baseModelId is absent.
	if got[1].ID != "gemini-embedding" {
		t.Errorf("expected id from name fallback, got %q", got[1].ID)
	}
}

func TestParseModels_UnknownFormat(t *testing.T) {
	if _, err := parseModels("bogus", []byte(`{}`)); err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestRenderTable_SingleColumn(t *testing.T) {
	models := []Model{{ID: "gpt-5"}, {ID: "gpt-5-mini"}}
	got := renderTable(models)
	want := "| Model ID |\n|---|\n| `gpt-5` |\n| `gpt-5-mini` |\n"
	if got != want {
		t.Errorf("renderTable mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderDoc(t *testing.T) {
	data := docData{
		Intro:  "Intro prose here.",
		Footer: "## Refreshing\n\nRun `make generate-docs`.",
		Date:   "2026-11-11",
		Sections: []section{
			{
				Provider: Provider{
					Heading:     "OpenAI — `/openai/v1/...`",
					Description: "Native OpenAI API.",
					Endpoints:   []string{"Models: `GET /openai/v1/models`"},
				},
				Table: renderTable([]Model{{ID: "gpt-5"}}),
			},
		},
		NotExposed: []string{"mistral", "cohere"},
	}
	out, err := renderDoc(data)
	if err != nil {
		t.Fatalf("renderDoc: %v", err)
	}

	for _, want := range []string{
		"# API surface",
		"Intro prose here.",
		"## OpenAI — `/openai/v1/...`",
		"Native OpenAI API.",
		"- Models: `GET /openai/v1/models`",
		"### Models (as of 2026-11-11)",
		"| `gpt-5` |",
		"## Providers not currently exposed",
		"- `mistral`",
		"- `cohere`",
		"Run `make generate-docs`.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered doc missing %q:\n%s", want, out)
		}
	}

	// No runs of 3+ newlines should survive the collapse step.
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("doc contains a triple newline:\n%s", out)
	}
	// Exactly one trailing newline.
	if !strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") {
		t.Errorf("doc should end with exactly one newline")
	}
}

func TestRenderDoc_LiveCandidateExcluded(t *testing.T) {
	// A candidate that is live must not appear in NotExposed. This mirrors how
	// run() builds NotExposed from candidates minus the live set.
	data := docData{
		Date:       "2026-11-11",
		NotExposed: []string{"mistral"}, // cohere omitted as if live
	}
	out, err := renderDoc(data)
	if err != nil {
		t.Fatalf("renderDoc: %v", err)
	}
	if strings.Contains(out, "`cohere`") {
		t.Errorf("live candidate cohere should not be listed:\n%s", out)
	}
	if !strings.Contains(out, "`mistral`") {
		t.Errorf("dead candidate mistral should be listed:\n%s", out)
	}
}

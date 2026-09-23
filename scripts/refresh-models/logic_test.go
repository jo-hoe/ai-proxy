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
	if got[0].ID != "anthropic--claude-4.5-haiku" || got[0].DisplayName != "Claude 4.5 Haiku" {
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
	body := []byte(`{"models":[{"baseModelId":"gemini-2.5-flash","inputTokenLimit":1000000,"outputTokenLimit":65536,"supportedGenerationMethods":["generateContent","countTokens"]},{"name":"models/gemini-embedding","inputTokenLimit":1048576,"supportedGenerationMethods":["embedContent"]}]}`)
	got, err := parseModels("gemini", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d models, want 2", len(got))
	}
	if got[0].ID != "gemini-2.5-flash" || got[0].InputTokens != 1000000 {
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

func TestFormatInt(t *testing.T) {
	cases := map[int]string{
		0:       "0",
		100:     "100",
		1000:    "1,000",
		65536:   "65,536",
		1000000: "1,000,000",
		1048576: "1,048,576",
	}
	for in, want := range cases {
		if got := formatInt(in); got != want {
			t.Errorf("formatInt(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderTable_Gemini_EmbeddingHasNoOutput(t *testing.T) {
	models := []Model{{ID: "gemini-embedding", InputTokens: 1048576, OutputTokens: 0, Methods: []string{"embedContent"}}}
	got := renderTable("gemini", models)
	if !strings.Contains(got, "| `gemini-embedding` | 1,048,576 | — | embedContent |") {
		t.Errorf("unexpected gemini row:\n%s", got)
	}
}

const sampleDoc = "# API surface\n\n" +
	"## OpenAI — `/openai/v1/...`\n\n" +
	"Native OpenAI API.\n\n" +
	"- Models: `GET /openai/v1/models`\n\n" +
	"### Models (as of 2026-07-16)\n\n" +
	"| Model ID |\n|---|\n| `gpt-4.1` |\n| `gpt-5` |\n\n" +
	"## Google Gemini — `/gemini/v1beta/...`\n\n" +
	"prose stays.\n"

func TestSpliceModelsTable_ReplacesOnlyTargetTable(t *testing.T) {
	p := Provider{Format: "openai", Heading: "OpenAI — `/openai/v1/...`"}
	models := []Model{{ID: "gpt-5"}, {ID: "gpt-5.6-luna"}}
	out, err := spliceModelsTable(sampleDoc, p, models, "2026-09-23")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "### Models (as of 2026-09-23)") {
		t.Error("date was not updated")
	}
	if !strings.Contains(out, "| `gpt-5.6-luna` |") {
		t.Error("new model missing from table")
	}
	if strings.Contains(out, "| `gpt-4.1` |") {
		t.Error("stale model should have been removed")
	}
	// Surrounding prose and the next section must be preserved.
	if !strings.Contains(out, "Native OpenAI API.") || !strings.Contains(out, "prose stays.") {
		t.Error("surrounding prose was disturbed")
	}
	if !strings.Contains(out, "## Google Gemini — `/gemini/v1beta/...`") {
		t.Error("next section heading was disturbed")
	}
}

func TestSpliceModelsTable_MissingSection(t *testing.T) {
	p := Provider{Format: "openai", Heading: "Nonexistent"}
	if _, err := spliceModelsTable(sampleDoc, p, nil, "2026-09-23"); err == nil {
		t.Fatal("expected error for missing section")
	}
}

func TestDropFromNotExposed(t *testing.T) {
	doc := "## Providers not currently exposed\n\nThe following returned 404:\n\n" +
		"Azure OpenAI, AWS Bedrock, Mistral, Cohere, DeepSeek.\n"
	out := dropFromNotExposed(doc, []string{"mistral"})
	if strings.Contains(strings.ToLower(out), "mistral") {
		t.Errorf("mistral should have been removed:\n%s", out)
	}
	if !strings.Contains(out, "Cohere") || !strings.Contains(out, "DeepSeek") {
		t.Errorf("other vendors should remain:\n%s", out)
	}
}

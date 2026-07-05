# API surface

ai-proxy is a transparent reverse proxy: whatever path you send is forwarded
verbatim to the configured `upstream_url`. The paths below are the ones the
current upstream exposes. Each provider preserves its native API shape, so
you can point existing SDKs at the proxy by only changing the base URL.

The **paths are stable** — the `/<provider>/<version>/...` prefix does not
change. The **model catalogue does change** over time; the tables below are
snapshots. Re-run the listed `curl` commands to get the current set.

In the examples below, `$BASE` is the proxy's base URL (whatever host it is
deployed to).

## Anthropic — `/anthropic/v1/...`

Native Anthropic Messages API. Point Anthropic SDKs at `$BASE/anthropic` as
their base URL.

- Models: `GET /anthropic/v1/models`
- Messages: `POST /anthropic/v1/messages`

### Models (as of 2026-07-05)

| Model ID | Display name |
|---|---|
| `anthropic--claude-4-sonnet` | Claude 4 Sonnet |
| `anthropic--claude-4.5-haiku` | Claude 4.5 Haiku |
| `anthropic--claude-4.5-sonnet` | Claude 4.5 Sonnet |
| `anthropic--claude-4.5-opus` | Claude 4.5 Opus |
| `anthropic--claude-4.6-sonnet` | Claude 4.6 Sonnet |
| `anthropic--claude-4.6-opus` | Claude 4.6 Opus |
| `anthropic--claude-4.7-opus` | Claude 4.7 Opus |

The Messages endpoint also accepts short aliases that resolve to a dated
snapshot in the response (as of 2026-07-05):

| Alias | Resolves to |
|---|---|
| `claude-haiku-4-5` | `claude-haiku-4-5-20251001` |
| `claude-sonnet-4-5` | `claude-sonnet-4-5-20250929` |
| `claude-opus-4-5` | `claude-opus-4-5-20251101` |
| `claude-sonnet-4-6` | `claude-sonnet-4-6` |
| `claude-opus-4-6` | `claude-opus-4-6` |

## OpenAI — `/openai/v1/...`

Native OpenAI API. Point OpenAI SDKs at `$BASE/openai/v1` as their base URL.

- Models: `GET /openai/v1/models`
- Chat: `POST /openai/v1/chat/completions`
- Embeddings: `POST /openai/v1/embeddings`

### Models (as of 2026-07-05)

| Model ID |
|---|
| `gpt-4.1` |
| `gpt-4.1-mini` |
| `gpt-5` |
| `gpt-5-mini` |
| `gpt-5.4` |
| `gpt-5.5` |
| `text-embedding-3-large` |
| `text-embedding-3-small` |

## Google Gemini — `/gemini/v1beta/...`

Native Google Generative Language API. Note the version is `v1beta`, not `v1`.

- Models: `GET /gemini/v1beta/models`
- Generate: `POST /gemini/v1beta/models/<model>:generateContent`
- Token count: `POST /gemini/v1beta/models/<model>:countTokens`
- Embed: `POST /gemini/v1beta/models/<model>:embedContent`

### Models (as of 2026-07-05)

| Model ID | Input tokens | Output tokens | Methods |
|---|---|---|---|
| `gemini-2.5-flash` | 1,000,000 | 65,536 | generateContent, countTokens, createCachedContent |
| `gemini-2.5-flash-lite` | 1,000,000 | 65,536 | generateContent, countTokens, createCachedContent |
| `gemini-2.5-pro` | 1,000,000 | 65,536 | generateContent, countTokens, createCachedContent |
| `gemini-3.1-flash-lite` | 1,000,000 | 65,536 | generateContent, countTokens, createCachedContent |
| `gemini-embedding` | 1,048,576 | — | embedContent |

## Refreshing this document

There is no cross-provider catalogue endpoint on the upstream; each provider
must be queried on its own path. To refresh the tables above:

```bash
curl -s $BASE/anthropic/v1/models  | jq
curl -s $BASE/openai/v1/models     | jq
curl -s $BASE/gemini/v1beta/models | jq
```

Update the "as of" date at the top of each provider section when you refresh.

## Providers not currently exposed

The following provider prefixes were probed on 2026-07-05 and returned 404,
i.e. they are **not** available through the upstream:

Azure OpenAI, AWS Bedrock, Vertex AI, Mistral, Cohere, Meta / Llama,
AI21, Aleph Alpha, NVIDIA / NIM, watsonx / IBM, DeepSeek, xAI / Grok,
Databricks, Fireworks, Groq, Together, Perplexity.

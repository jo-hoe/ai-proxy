# API surface

ai-proxy is a transparent reverse proxy: whatever path you send is forwarded
verbatim to the configured `upstream_url`. The paths below are the ones the
current upstream exposes. Each provider preserves its native API shape, so
you can point existing SDKs at the proxy by only changing the base URL.

The **paths are stable** — the `/<provider>/<version>/...` prefix does not
change. The **model catalogue does change** over time; the tables below are
snapshots.

In the examples below, `$BASE` is the proxy's base URL (whatever host it is
deployed to).

## Anthropic — `/anthropic/v1/...`

Native Anthropic Messages API. Point Anthropic SDKs at `$BASE/anthropic` as
their base URL.

- Models: `GET /anthropic/v1/models`
- Messages: `POST /anthropic/v1/messages`

### Models (as of 2026-11-11)

| Model ID |
|---|
| `claude-haiku-4-5` |
| `claude-opus-4-5` |
| `claude-opus-4-6` |
| `claude-opus-4-7` |
| `claude-opus-4-8` |
| `claude-sonnet-4-5` |
| `claude-sonnet-4-6` |
| `claude-sonnet-4-6[1m]` |

## OpenAI — `/openai/v1/...`

Native OpenAI API. Point OpenAI SDKs at `$BASE/openai/v1` as their base URL.

- Models: `GET /openai/v1/models`
- Chat: `POST /openai/v1/chat/completions`
- Embeddings: `POST /openai/v1/embeddings`

### Models (as of 2026-11-11)

| Model ID |
|---|
| `gpt-4.1` |
| `gpt-4.1-mini` |
| `gpt-5` |
| `gpt-5-mini` |
| `gpt-5.4` |
| `gpt-5.5` |
| `gpt-5.6-luna` |
| `gpt-5.6-sol` |
| `gpt-5.6-terra` |
| `gpt-6-astra` |
| `gpt-6-luna` |
| `gpt-6.1-sol` |
| `text-embedding-3-large` |
| `text-embedding-3-small` |

## Google Gemini — `/gemini/v1beta/...`

Native Google Generative Language API. Note the version is `v1beta`, not `v1`.

- Models: `GET /gemini/v1beta/models`
- Generate: `POST /gemini/v1beta/models/<model>:generateContent`
- Token count: `POST /gemini/v1beta/models/<model>:countTokens`
- Embed: `POST /gemini/v1beta/models/<model>:embedContent`

### Models (as of 2026-11-11)

| Model ID |
|---|
| `gemini-2.5-flash` |
| `gemini-2.5-flash-lite` |
| `gemini-2.5-pro` |
| `gemini-3.1-flash-lite` |
| `gemini-3.5-flash` |
| `gemini-3.8-flash` |
| `gemini-embedding` |

## Providers not currently exposed

The following vendor prefixes returned a non-2xx status at generation time and
are not documented above. If one starts responding, it is added automatically on
the next `make generate-docs`:

- `azure`
- `bedrock`
- `vertex`
- `mistral`
- `cohere`
- `meta`
- `ai21`
- `alephalpha`
- `nvidia`
- `watsonx`
- `deepseek`
- `xai`
- `databricks`
- `fireworks`
- `groq`
- `together`
- `perplexity`

## Refreshing this document

This document is generated. Do not edit it by hand — edit `scripts/providers.yaml`
and run:

```bash
make generate-docs
```

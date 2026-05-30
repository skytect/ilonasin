# OpenRouter API Surface

Accessed: 2026-05-30. Primary evidence: official OpenRouter documentation, the official OpenAPI document, and sanitized live tests. No secrets, full response IDs, prompts, completions, credit balances, account identifiers, or bearer headers are included.

## Sources

- OpenAPI: https://openrouter.ai/openapi.json
- API overview: https://openrouter.ai/docs/api/reference/overview
- Authentication: https://openrouter.ai/docs/api/reference/authentication
- Parameters: https://openrouter.ai/docs/api/reference/parameters
- Streaming: https://openrouter.ai/docs/api/reference/streaming
- Errors and debugging: https://openrouter.ai/docs/api/reference/errors-and-debugging
- Limits: https://openrouter.ai/docs/api/reference/limits
- Chat completions: https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request
- Responses API: https://openrouter.ai/docs/api/reference/responses/overview
- Provider routing: https://openrouter.ai/docs/guides/routing/provider-selection
- Model fallbacks: https://openrouter.ai/docs/guides/routing/model-fallbacks
- Structured outputs: https://openrouter.ai/docs/guides/features/structured-outputs
- Tool calling: https://openrouter.ai/docs/guides/features/tool-calling
- Reasoning tokens: https://openrouter.ai/docs/guides/best-practices/reasoning-tokens
- Prompt caching: https://openrouter.ai/docs/guides/best-practices/prompt-caching
- Response caching: https://openrouter.ai/docs/guides/features/response-caching

## Base URLs and Auth

| Surface | Base URL | Notes |
| --- | --- | --- |
| Production API | `https://openrouter.ai/api/v1` | Official OpenAPI server and OpenAI SDK base URL. |
| EU in-region API | `https://eu.openrouter.ai/api/v1` | Enterprise EU-only routing surface, available by request. |

Auth is `Authorization: Bearer <redacted>`. Some discovery endpoints are publicly readable, but authenticated requests are required for inference, credits, key telemetry, and account management.

Optional app attribution headers:

- `HTTP-Referer`: app URL.
- `X-Title` or `X-OpenRouter-Title`: app display name.
- `X-OpenRouter-Categories`: comma-separated app categories.

## Endpoint Inventory

The official OpenAPI document currently exposes these endpoint families.

| Area | Endpoints |
| --- | --- |
| Chat | `POST /chat/completions`, `POST /presets/{slug}/chat/completions` |
| Responses | `POST /responses`, `POST /presets/{slug}/responses` |
| Anthropic Messages | `POST /messages`, `POST /presets/{slug}/messages` |
| Models/providers | `GET /models`, `GET /models/count`, `GET /models/user`, `GET /models/{author}/{slug}/endpoints`, `GET /providers`, `GET /endpoints/zdr` |
| Embeddings/rerank | `POST /embeddings`, `GET /embeddings/models`, `POST /rerank` |
| Audio/video | `POST /audio/speech`, `POST /audio/transcriptions`, `POST /videos`, `GET /videos/models`, `GET /videos/{jobId}`, `GET /videos/{jobId}/content` |
| Usage/billing | `GET /credits`, `GET /activity`, `GET /generation`, `GET /generation/content`, `GET /key` |
| API keys and OAuth-style key creation | `GET /keys`, `POST /keys`, `GET /keys/{hash}`, `PATCH /keys/{hash}`, `DELETE /keys/{hash}`, `POST /auth/keys`, `POST /auth/keys/code` |
| BYOK | `GET /byok`, `POST /byok`, `GET /byok/{id}`, `PATCH /byok/{id}`, `DELETE /byok/{id}` |
| Guardrails | `GET /guardrails`, `POST /guardrails`, `GET /guardrails/{id}`, `PATCH /guardrails/{id}`, `DELETE /guardrails/{id}`, guardrail assignment endpoints |
| Observability | `GET /observability/destinations`, `POST /observability/destinations`, `GET/PATCH/DELETE /observability/destinations/{id}` |
| Organization/workspaces | `GET /organization/members`, `GET/POST /workspaces`, `GET/PATCH/DELETE /workspaces/{id}`, workspace member add/remove endpoints |
| Rankings | `GET /datasets/rankings-daily` |

The OpenAPI document does not list a legacy `POST /completions` endpoint. OpenAI-compatible text clients should use Chat Completions or Responses.

## Model Discovery and Naming

`GET /models` returns `data[]` with fields such as:

- `id`: OpenRouter model slug, for example `openai/gpt-5.1-chat` or `deepseek/deepseek-v4-flash:free`.
- `name`, `description`, `created`.
- `architecture`, including input/output modalities.
- `context_length`.
- `pricing`.
- `top_provider`.
- `supported_parameters`.
- `per_request_limits`.

Model IDs are OpenRouter slugs, not native provider IDs. Some model IDs include suffixes such as `:free`. The `openrouter/auto` router and `models: [...]` fallback list can cause the actual model returned in the response to differ from a generic route selection. A router should record both the requested model and the resolved response model.

`GET /models/{author}/{slug}/endpoints` exposes provider-specific endpoints for one model. `GET /providers` exposes provider metadata. `GET /models/user` filters model visibility by the caller's provider preferences, privacy settings, and guardrails.

## Chat Completions

`POST /chat/completions` is the main OpenAI-compatible inference endpoint.

Common OpenAI-style request fields:

- `model`, `messages`
- `temperature`, `top_p`, `stop`
- `top_k`, `min_p`, `top_a`, `repetition_penalty`, `logit_bias`
- `max_tokens`, `max_completion_tokens`
- `prediction`, `user`, `verbosity`
- `stream`, `stream_options`
- `tools`, `tool_choice`, `parallel_tool_calls`
- `response_format`
- `logprobs`, `top_logprobs`
- `seed`
- `presence_penalty`, `frequency_penalty`

OpenRouter-specific request fields include:

- `models`: fallback model list.
- `route`: OpenRouter routing mode such as fallback routing.
- `provider`: provider routing/privacy/capability preferences.
- `reasoning` and legacy `include_reasoning`.
- `plugins`: OpenRouter-side features such as file parsing, web/search integrations, response healing, or context compression.
- `cache_control`, `metadata`, `session_id`, `service_tier`, `modalities`, `image_config`, `web_search_options`, `stop_server_tools_when`, and tracing/debug fields.

OpenRouter supports final assistant-message prefill. Direct DeepSeek's closest
equivalent is beta chat prefix completion on `https://api.deepseek.com/beta`
with `prefix: true`; a router must translate or reject prefill explicitly.

Response shape is OpenAI-like:

- `object: "chat.completion"` for non-streaming.
- `object: "chat.completion.chunk"` for streaming chunks.
- `choices[].message` for non-streaming.
- `choices[].delta` for streaming.
- `usage` for token and cost telemetry.

OpenRouter can add provider/routing fields such as `provider`, `native_finish_reason`, `service_tier`, detailed cost usage, and reasoning fields.

OpenRouter may ignore unsupported parameters for a routed provider. Use `provider.require_parameters: true` when a feature such as tools, JSON schema, or logprobs must be honored rather than silently downgraded.

## Streaming

Set `stream: true`. OpenRouter streams server-sent events:

- Data frames use `data: {...}`.
- The stream terminates with `data: [DONE]`.
- SSE comments such as `: OPENROUTER PROCESSING` can appear and should be ignored.
- Streaming errors can arrive as HTTP `200` SSE objects with a top-level `error` and `choices[0].finish_reason: "error"`.
- For chat streaming, docs state usage is returned in a final chunk before `[DONE]`.
- OpenRouter documents `X-Generation-Id` for correlating inference requests with
  `/generation`; direct DeepSeek has no comparable generation lookup endpoint.

Clients should tolerate comments, empty lines, final usage chunks with empty `choices`, provider-specific delta fields, and mid-stream top-level errors.

## Responses API

`POST /responses` is documented as beta and stateless. It is intended for OpenAI Responses-compatible clients, but callers must send full context rather than relying on persisted conversation state.

Documented request concepts include:

- `model`
- string or structured `input`
- `instructions`
- `tools`, `tool_choice`, `parallel_tool_calls`
- `reasoning`
- `stream`
- `max_output_tokens`
- `previous_response_id`
- `prompt_cache_key`
- `text`, `truncation`, `modalities`
- routing fields such as `provider`, `models`, plugins, and service controls

The local smoke script did not test `/responses`; this path is docs-only in this repository pass.

## Tools and Function Calling

OpenRouter accepts OpenAI-style function tools:

```json
{
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather for a city.",
        "parameters": {
          "type": "object",
          "properties": { "city": { "type": "string" } },
          "required": ["city"]
        }
      }
    }
  ],
  "tool_choice": "auto"
}
```

For multi-turn tool use, pass the assistant message containing `tool_calls` and then a `role: "tool"` result message. For non-OpenAI providers, OpenRouter transforms tool schemas to provider-native formats when possible.

Router implication: if tool support is required, either select a model whose `supported_parameters` includes `tools` or send `provider.require_parameters: true`.

Cross-provider caveat: direct DeepSeek strict tool mode is narrower than
OpenRouter-routed tool support. DeepSeek strict mode requires the beta base URL,
`function.strict: true`, all object properties in `required`,
`additionalProperties: false`, and a limited JSON Schema subset.

## Structured Outputs

OpenRouter documents both JSON mode and schema mode through `response_format`.

JSON mode:

```json
{ "type": "json_object" }
```

JSON schema mode:

```json
{
  "type": "json_schema",
  "json_schema": {
    "name": "example",
    "strict": true,
    "schema": {
      "type": "object",
      "properties": { "ok": { "type": "boolean" } },
      "required": ["ok"],
      "additionalProperties": false
    }
  }
}
```

Use `supported_parameters` and `provider.require_parameters: true` for reliable routing. Without that, a provider may ignore or downgrade unsupported structure controls.

The current OpenAPI schema also includes OpenRouter-specific structured-output
variants such as `grammar` and `python`. Direct DeepSeek chat only documents
`text` and `json_object`; DeepSeek schema strictness is through beta strict tool
calls, not chat `response_format`.

## Reasoning

OpenRouter normalizes reasoning controls across providers with a `reasoning` object. Official docs describe controls such as:

```json
{
  "reasoning": {
    "effort": "high",
    "max_tokens": 2000,
    "exclude": false,
    "enabled": true
  }
}
```

Reasoning support and returned fields vary by model/provider. Responses may include fields such as `message.reasoning`, `reasoning_details`, or token details under `usage`. Some upstream models expose reasoning token budgets but do not return raw reasoning text.

Router guidance: normalize OpenRouter `reasoning` / `reasoning_details` and
DeepSeek `reasoning_content` into internal reasoning fields. Do not assume
OpenRouter `reasoning.effort` maps one-to-one to DeepSeek `thinking` plus
`reasoning_effort`, and do not replay provider reasoning as normal assistant
content across providers.

## Multimodal, Audio, Video, Embeddings, and Rerank

Chat messages support content parts for multimodal-capable models, including text and images. The OpenAPI surface also contains dedicated endpoints for:

- Embeddings: `POST /embeddings`, `GET /embeddings/models`.
- Rerank: `POST /rerank`.
- Audio speech/transcription: `POST /audio/speech`, `POST /audio/transcriptions`.
- Video generation: `POST /videos`, `GET /videos/{jobId}`, `GET /videos/{jobId}/content`, `GET /videos/models`.

Support is model- and provider-dependent. Use endpoint-specific model lists and `supported_parameters` rather than assuming every OpenRouter model supports every modality.

## Provider Routing, Fallbacks, and Privacy

The `provider` object controls routing. Documented concepts include:

- `order`: ordered provider slugs.
- `only` and `ignore`: provider allow/block lists.
- `allow_fallbacks`: default fallback behavior.
- `sort`: route by criteria such as price, latency, or throughput.
- `require_parameters`: require support for supplied parameters.
- `max_price`: cap prompt/completion/request/media prices.
- `data_collection: "deny"`: prefer providers that do not collect user data.
- `zdr: true`: require Zero Data Retention endpoints.
- `quantizations` and other provider-specific filters.

`models: [...]` enables model-level fallbacks. `openrouter/auto` delegates model choice to OpenRouter. A generic router should not hide this resolution; it should expose requested model, resolved model, provider, and fallback status where possible.

## Caching

Prompt caching is provider-dependent. Usage can include fields such as:

- `prompt_tokens_details.cached_tokens`
- `prompt_tokens_details.cache_write_tokens`

OpenRouter also documents response caching. When enabled, cache-related headers such as `X-OpenRouter-Cache-Status`, `X-OpenRouter-Cache-Age`, and `X-OpenRouter-Cache-TTL` can appear. Cached responses can change billing behavior, so a router should keep cache telemetry distinct from normal provider token usage.

## Usage, Billing, and Management

Relevant endpoints:

- `GET /credits`: remaining credits and usage totals.
- `GET /key`: current key telemetry, limits, usage buckets, BYOK usage, and free-tier state.
- `GET /activity`: grouped user activity.
- `GET /generation`: metadata for a generation.
- `GET /generation/content`: stored prompt/completion content for a generation, if retained and accessible.
- `GET/POST/PATCH/DELETE /keys`: key management.
- `GET/POST/PATCH/DELETE /byok`: bring-your-own-key provider credentials.
- Workspaces, organization members, guardrails, and observability destinations for account administration.

Do not log raw credit values, account identifiers, key hashes, generation content, or prompt/completion content in normal router logs.

## Errors and Rate Limits

Documented error shape:

```json
{
  "error": {
    "code": 401,
    "message": "..."
  }
}
```

The `error` object may also contain `metadata`. Documented statuses include `400`, `401`, `402`, `403`, `408`, `429`, `502`, and `503`. `Retry-After` may appear on `429` and `503`.

Free models and routed upstream providers can have separate limits. OpenRouter can return upstream/provider error metadata. A router should normalize `status`, `code`, `message`, `retry_after`, `provider`, and `model`, while keeping redacted raw error shape for debugging.

## OpenAI Compatibility Gaps

- Base URL is OpenAI-compatible, but model IDs are OpenRouter slugs.
- The API is a router, so requested model/provider may differ from resolved model/provider.
- Unsupported parameters may be ignored unless `provider.require_parameters: true`.
- `/responses` is beta and stateless.
- Streaming can include comments and mid-stream error objects.
- OpenRouter adds provider/routing, cost, caching, guardrail, plugin, BYOK, workspace, and observability surfaces that plain OpenAI clients do not model.
- `GET /generation` metadata is tied to OpenRouter generation tracking, not necessarily the same as the chat response `id`.

## Minimal Examples

Chat:

```bash
curl https://openrouter.ai/api/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <redacted>" \
  -H "HTTP-Referer: https://example.com" \
  -H "X-Title: Example App" \
  -d '{
    "model": "openai/gpt-5.1-chat",
    "messages": [{"role": "user", "content": "Hello"}],
    "max_completion_tokens": 16
  }'
```

OpenAI SDK:

```python
from openai import OpenAI

client = OpenAI(
    api_key="<redacted>",
    base_url="https://openrouter.ai/api/v1",
)

resp = client.chat.completions.create(
    model="openai/gpt-5.1-chat",
    messages=[{"role": "user", "content": "Hello"}],
    max_completion_tokens=16,
    extra_headers={
        "HTTP-Referer": "https://example.com",
        "X-Title": "Example App",
    },
)
print(resp.choices[0].message.content)
```

Structured output with routing guard:

```bash
curl https://openrouter.ai/api/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <redacted>" \
  -d '{
    "model": "openai/gpt-5.1-chat",
    "messages": [{"role": "user", "content": "Return {\"ok\": true}"}],
    "response_format": {"type": "json_object"},
    "provider": {"require_parameters": true},
    "max_completion_tokens": 16
  }'
```

## Live-Test Matrix

Date: 2026-05-30. `OPENROUTER_API_KEY` was present. All local generation probes used token caps of 16 or less. Outputs below are sanitized schema/status observations only.

| Test | Result |
| --- | --- |
| Fake auth `GET /credits` | `401`; `error` object observed. |
| `GET /models` | `200`; schema key `data`. |
| `GET /credits` | `200`; schema key `data`; credit values redacted. |
| Non-stream chat, `openai/gpt-3.5-turbo-0613` | `200`; OpenAI-like keys plus `provider`, `service_tier`, `usage`. |
| Streaming chat, `openai/gpt-3.5-turbo-0613` | `200`; SSE event shapes observed. |
| JSON output, `openai/gpt-5.1-chat` | `200`; chat completion schema observed with `provider.require_parameters: true`. |
| Forced tool call, `openai/gpt-5.1-chat` | `200`; chat completion schema observed with `provider.require_parameters: true`. |
| Auto-selected free model `deepseek/deepseek-v4-flash:free` | Non-stream and stream returned `429`; JSON/tool probes returned `404` in this run. Treat as route/model availability behavior, not a global OpenRouter capability failure. |
| `GET /providers`, `GET /key`, `POST /responses` | Docs-only in the local parent smoke run. |
| Audio/video/embeddings/rerank/BYOK/guardrails/workspaces/observability | Docs-only. |

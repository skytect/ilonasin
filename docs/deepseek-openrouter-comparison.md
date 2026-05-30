# DeepSeek and OpenRouter API Comparison

Accessed: 2026-05-30. Primary evidence: official DeepSeek docs, official OpenRouter docs/OpenAPI, and sanitized live smoke tests. This comparison is written for a generic LLM router that accepts multiple credential types and exposes an OpenAI-compatible backend.

## Sources

- DeepSeek quick start: https://api-docs.deepseek.com/
- DeepSeek chat completions: https://api-docs.deepseek.com/api/create-chat-completion
- DeepSeek models: https://api-docs.deepseek.com/api/list-models
- DeepSeek JSON mode: https://api-docs.deepseek.com/guides/json_mode
- DeepSeek tool calling: https://api-docs.deepseek.com/guides/function_calling
- DeepSeek rate limits: https://api-docs.deepseek.com/quick_start/rate_limit
- DeepSeek errors: https://api-docs.deepseek.com/quick_start/error_codes
- OpenRouter OpenAPI: https://openrouter.ai/openapi.json
- OpenRouter overview: https://openrouter.ai/docs/api/reference/overview
- OpenRouter chat completions: https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request
- OpenRouter provider routing: https://openrouter.ai/docs/guides/routing/provider-selection
- OpenRouter structured outputs: https://openrouter.ai/docs/guides/features/structured-outputs
- OpenRouter streaming: https://openrouter.ai/docs/api/reference/streaming
- OpenRouter errors/limits: https://openrouter.ai/docs/api/reference/errors-and-debugging

## Compatibility Matrix

| Area | DeepSeek | OpenRouter | Router implication |
| --- | --- | --- | --- |
| Base URL | `https://api.deepseek.com`; beta at `/beta`; Anthropic-compatible surface at `/anthropic`. | `https://openrouter.ai/api/v1`; EU enterprise base exists by request. | Store base URL per credential/provider. Do not infer auth or path layout from model name alone. |
| Auth | Bearer API key. | Bearer API key; optional app attribution headers. Also has key-management and BYOK endpoints. | Keep user provider keys separate from router-owned admin credentials. Never forward one provider key to another provider. |
| Primary OpenAI-compatible path | `POST /chat/completions`. | `POST /chat/completions`. | Chat can share a common request path after provider base URL selection. |
| Other inference APIs | FIM `POST /completions` on beta; Anthropic compatibility documented. | Responses, Anthropic Messages, embeddings, rerank, audio, video, presets. | Expose non-chat capabilities as provider-specific feature flags, not as universal OpenAI-compatible promises. |
| Model names | Native IDs such as `deepseek-v4-flash`, `deepseek-v4-pro`. Legacy aliases are documented separately. | Slugs such as `openai/gpt-5.1-chat`, `deepseek/deepseek-v4-flash:free`, and routes such as `openrouter/auto`. | Use internal namespaces such as `deepseek:deepseek-v4-flash` and `openrouter:openai/gpt-5.1-chat`. Store requested and resolved model. |
| Model discovery | `GET /models`. | `GET /models`, `/models/user`, `/models/{author}/{slug}/endpoints`, `/providers`. | Build a model registry with provider-specific metadata and capability flags. |
| Standard chat fields | `model`, `messages`, `max_tokens`, `stream`, `stream_options`, `temperature`, `top_p`, `stop`, `tools`, `tool_choice`, `response_format`, logprobs. | Similar OpenAI chat fields plus broad provider pass-through. | Maintain a per-provider/model allowlist. Unknown fields should be rejected, warned, or forwarded only through explicit provider escape hatches. |
| Provider extensions | `thinking`, `reasoning_effort`, `user_id`, beta prefix completion, strict tool mode. | `provider`, `models`, `route`, `plugins`, `reasoning`, `cache_control`, app attribution headers, BYOK, guardrails. | Keep extension fields namespaced, for example `provider_options.deepseek` and `provider_options.openrouter`. |
| Reasoning | `thinking` controls mode; `reasoning_content` and `completion_tokens_details.reasoning_tokens` can appear. | `reasoning` normalizes effort/budget; output fields vary by model/provider. | Normalize to internal `reasoning_text` and `reasoning_tokens`, but retain redacted provider details. |
| Structured output | `response_format: {"type": "json_object"}`. No official JSON Schema mode found; strict tool mode can enforce schemas. | `json_object`, `json_schema`, and OpenRouter-specific grammar/python variants are exposed. | Represent JSON object, JSON Schema, and provider-specific grammar modes separately. Do not send JSON Schema to DeepSeek unless separately verified. |
| Tool calls | OpenAI-style tools; strict mode is beta with a limited schema subset. Live parent test succeeded with thinking disabled. | OpenAI-style tools across compatible routed models; `require_parameters` prevents routing to unsupported providers. | Tool support must be model-specific. Tool schema acceptance is not portable across providers. |
| Assistant prefill | Chat prefix completion beta requires `/beta` and `prefix: true`. | Final assistant-message prefill is supported. | Prefill needs explicit translation or rejection; it is not a plain pass-through feature. |
| Streaming | SSE ending in `[DONE]`; can include `delta.reasoning_content`; usage chunk documented with `stream_options.include_usage`. | SSE ending in `[DONE]`; can include comments, final usage chunk, provider-specific fields, and mid-stream error objects. | Parser must ignore comments/empty lines, support top-level stream errors, handle final usage chunks, and not assume every delta has content. |
| Errors | Docs list `400`, `401`, `402`, `422`, `429`, `500`, `503`; fake key returned OpenAI-like `error` object. | Docs list `400`, `401`, `402`, `403`, `408`, `429`, `502`, `503`; error object may include metadata. | Normalize status, code, message, retry-after, provider, model, and redacted raw shape. |
| Usage/billing | Standard token fields, cache hit/miss tokens, reasoning tokens; `/user/balance`. | Usage can include token/cost/provider/cache details; `/credits`, `/key`, `/activity`, `/generation`. | Store common usage fields plus `provider_usage` JSON. Redact account, credit, and generation identifiers by default. |
| Rate limits | Account/model concurrency limits; `429` on exceeded concurrency. | Limits vary by account, key, model, provider, and free-tier route; `Retry-After` may appear. | Implement provider rate buckets. Do not auto-switch accounts to bypass provider terms. |
| Privacy/user isolation | `user_id` supports safety/cache/scheduling isolation and must not contain private info. | Provider routing supports data-collection and ZDR constraints; BYOK and region controls exist. | Model privacy controls as explicit routing policy, not hidden metadata. |

## Router Design Implications

Credential registry:

- Store credential type, provider, base URL, owner, allowed models, billing owner, and feature policy.
- Support API-key credentials for DeepSeek and OpenRouter.
- For future OAuth/subscription credentials, store refresh state and scopes separately from provider API keys.
- Keep router admin credentials separate from user-supplied provider credentials.

Model namespace:

- Use a stable internal prefix: `deepseek:<model>` and `openrouter:<slug>`.
- Track aliases separately from canonical model IDs.
- Record `requested_model`, `provider`, `resolved_model`, and `resolved_upstream` when available.

Request normalization:

- Keep an OpenAI-compatible common core for `messages`, `tools`, `tool_choice`, `response_format`, sampling, token limits, and streaming.
- Maintain per-provider translations:
  - DeepSeek reasoning: `reasoning` intent maps to `thinking` and `reasoning_effort`.
  - OpenRouter reasoning: `reasoning` can pass through to OpenRouter's normalized object.
  - DeepSeek JSON Schema should not be sent as `response_format.json_schema`.
  - OpenRouter feature-sensitive requests should use `provider.require_parameters: true`.
  - DeepSeek `user_id` is not equivalent to OpenAI/OpenRouter `user`; map it
    only through an explicit privacy and isolation policy.
- Provide explicit provider escape hatches instead of silently forwarding arbitrary fields.

Streaming normalization:

- Treat SSE comments and empty lines as non-events.
- Stop on `[DONE]`.
- Parse both `choices[].delta` content and provider-specific reasoning deltas.
- Accept final usage chunks with empty choices.
- Accept top-level stream errors.
- Surface partial output plus normalized error when a stream fails mid-flight.

Tool-call normalization:

- Normalize to OpenAI `tool_calls` internally.
- Validate tool-call arguments as JSON before invoking tools.
- Preserve tool call IDs when present.
- For DeepSeek strict mode, route to the beta base URL only when the caller explicitly requests strict mode and the schema satisfies DeepSeek's subset.
- For OpenRouter, check model `supported_parameters` or require `provider.require_parameters: true`.

Structured-output normalization:

- `json_object`: supported by both surfaces, but prompts still need to ask for JSON.
- `json_schema`: supported by OpenRouter for compatible models; not found in DeepSeek official chat docs.
- Strict function/tool schemas can emulate schema-constrained output on DeepSeek beta.

Usage and rate-limit normalization:

- Common fields: prompt tokens, completion tokens, total tokens, reasoning tokens, cached tokens, finish reason, elapsed time, status.
- Provider extras: cost, provider, route, cache write tokens, BYOK usage, balance/credits, concurrency bucket.
- Redact account balances, account IDs, key hashes, request IDs, and generation content in normal logs.
- Respect `Retry-After` when present.
- Avoid account rotation policies that are designed to evade rate limits or payment limits.

Fallback policy:

- OpenRouter has native model and provider fallback controls.
- DeepSeek does not expose provider fallback; fallback would be router-side across models or credentials.
- Router-side fallback should be explicit and auditable. Users should be able to see when a request left the requested provider/model.
- Never fallback from a privacy-constrained route to a weaker privacy route unless the user policy permits it.

## Live Probe Summary

Live-tested locally on 2026-05-30 using `scripts/api-surface/smoke.py`. Both provider keys were present. Token caps were 16 or less. Raw prompts, completions, balances, keys, account IDs, and request IDs were not printed.

| Provider | Test | Result |
| --- | --- | --- |
| DeepSeek | fake-key `GET /models` | `401`, error object observed. |
| DeepSeek | `GET /models` | `200`, keys `data`, `object`. |
| DeepSeek | `GET /user/balance` | `200`, keys `balance_infos`, `is_available`; values redacted. |
| DeepSeek | non-stream chat | `200`, OpenAI-like chat completion schema. |
| DeepSeek | stream chat | `200`, SSE event shapes observed. |
| DeepSeek | JSON object | `200`, chat completion schema observed. |
| DeepSeek | forced tool call | `200`, chat completion schema observed. |
| OpenRouter | fake-key `GET /credits` | `401`, error object observed. |
| OpenRouter | `GET /models` | `200`, key `data`. |
| OpenRouter | `GET /credits` | `200`, key `data`; values redacted. |
| OpenRouter | non-stream chat | `200` with `openai/gpt-3.5-turbo-0613`. |
| OpenRouter | stream chat | `200` with `openai/gpt-3.5-turbo-0613`. |
| OpenRouter | JSON object | `200` with `openai/gpt-5.1-chat` and `provider.require_parameters: true`. |
| OpenRouter | forced tool call | `200` with `openai/gpt-5.1-chat` and `provider.require_parameters: true`. |

Docs-only in this local parent run:

- DeepSeek Anthropic compatibility.
- DeepSeek embeddings/files/batches/fine-tuning/images/audio, because no official endpoints were found.
- OpenRouter Responses, Messages, embeddings, rerank, audio/video, providers, key telemetry, generation metadata, BYOK, guardrails, workspaces, observability.

## Safe Testing Strategy

Use a smoke script with these constraints:

- Read only `DEEPSEEK_API_KEY` and `OPENROUTER_API_KEY`.
- Print only whether each key is present.
- Use fixed harmless prompts and token caps of 16 or less.
- Run one fake-key auth probe only against endpoints where invalid auth does not risk lockout.
- Sanitize output to status code, top-level schema keys, selected header names, and safe timing.
- Redact keys, bearer headers, IDs, account IDs, user IDs, request IDs, generation IDs, prompts, completions, tool arguments, balances, and credit totals.
- Keep expensive or content-revealing endpoints behind explicit opt-in flags.

## Risks

- Accidentally logging prompts, completions, API keys, request IDs, account IDs, key hashes, balances, or generation content.
- Confusing user-supplied provider API keys with router-owned admin credentials.
- Hiding routed provider identity, resolved model, fallback behavior, or BYOK status from users.
- Routing around rate limits or credit limits in ways that violate provider terms.
- Silently forwarding unsupported fields and letting users believe a feature was honored.
- Treating OpenAI-compatible as OpenAI-identical, especially for reasoning, streaming, usage, structured output, and routed provider behavior.
- Falling back from a privacy-constrained route to a cheaper or faster route that does not satisfy the user's privacy policy.

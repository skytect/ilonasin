# DeepSeek API Surface

Accessed: 2026-05-30. Primary evidence: official DeepSeek documentation plus sanitized live tests. No secrets, full response IDs, balances, prompts, completions, or account identifiers are included.

## Sources

- Quick start: https://api-docs.deepseek.com/
- Chat completion reference: https://api-docs.deepseek.com/api/create-chat-completion
- Models endpoint: https://api-docs.deepseek.com/api/list-models
- FIM completions endpoint: https://api-docs.deepseek.com/api/create-completion
- Balance endpoint: https://api-docs.deepseek.com/api/get-user-balance
- Models/pricing/context: https://api-docs.deepseek.com/quick_start/pricing
- Token usage: https://api-docs.deepseek.com/quick_start/token_usage
- Rate limit/isolation: https://api-docs.deepseek.com/quick_start/rate_limit
- Error codes: https://api-docs.deepseek.com/quick_start/error_codes
- Thinking mode: https://api-docs.deepseek.com/guides/reasoning_model
- JSON output: https://api-docs.deepseek.com/guides/json_mode
- Tool/function calling: https://api-docs.deepseek.com/guides/function_calling
- Context caching: https://api-docs.deepseek.com/guides/kv_cache
- Anthropic compatibility: https://api-docs.deepseek.com/guides/anthropic_api

## Base URLs and Auth

| Surface | Base URL | Notes |
| --- | --- | --- |
| OpenAI-compatible API | `https://api.deepseek.com` | Official quick-start base URL. |
| OpenAI-compatible beta API | `https://api.deepseek.com/beta` | Required for beta features such as FIM, chat prefix completion, and strict tool mode. |
| Anthropic-compatible API | `https://api.deepseek.com/anthropic` | Documented compatibility surface; not live-tested. |
| `/v1` alias | `https://api.deepseek.com/v1` | Live-tested by a subagent for `/models` and `/chat/completions`; not the current advertised base URL. |

Auth is `Authorization: Bearer <redacted>` with `Content-Type: application/json`. The OpenAI SDK works by setting `base_url="https://api.deepseek.com"` and passing the API key.

DeepSeek-specific request identity belongs in `user_id`, not the OpenAI `user`
field. It affects content-safety isolation, KV-cache privacy isolation, and
scheduling isolation. A router should not blindly forward private internal user
identifiers into this field or reuse one end-user identifier across providers
without an explicit privacy policy.

## Endpoint Inventory

| Endpoint | Method | Status |
| --- | ---: | --- |
| `/chat/completions` | POST | Documented and live-tested. |
| `/completions` | POST | Documented FIM beta endpoint on `/beta`; live-tested by subagent. |
| `/models` | GET | Documented and live-tested. |
| `/user/balance` | GET | Documented and live-tested with redacted account values. |
| Anthropic messages surface | POST | Documented via `/anthropic`; docs-only in this pass. |

No official API reference entries were found for embeddings, files, batches, hosted fine-tuning jobs, images, audio, Assistants, Responses API, or vector stores.

Anthropic compatibility has model-mapping behavior: DeepSeek documents Claude
model prefixes and unsupported Anthropic model names as mapping to DeepSeek
models. A router should treat the returned/resolved model as provider-specific
and should not assume the Anthropic `model` string was honored exactly.

## Models

Current documented model IDs:

- `deepseek-v4-flash`
- `deepseek-v4-pro`

Legacy aliases scheduled for deprecation on 2026-07-24:

- `deepseek-chat`: maps to non-thinking mode of `deepseek-v4-flash`.
- `deepseek-reasoner`: maps to thinking mode of `deepseek-v4-flash`.

Docs show both current models as the available `/models` output. The quick start lists a 1M context and 384K maximum output in current model material. Thinking mode is enabled by default for current chat examples; non-thinking mode is selected with `thinking: {"type": "disabled"}`.

These direct DeepSeek IDs are not portable to OpenRouter. OpenRouter uses
namespaced slugs such as `deepseek/deepseek-v4-flash` and can expose variants
such as `:free` with different routing, limits, and supported parameters.

## Chat Completions

`POST /chat/completions` is OpenAI Chat Completions compatible with DeepSeek extensions.

Request fields:

- Required: `model`, `messages`.
- Message roles: `system`, `user`, `assistant`, `tool`.
- Generation controls: `max_tokens`, `stop`, `stream`, `stream_options.include_usage`, `temperature`, `top_p`.
- Tool calls: `tools`, `tool_choice`.
- Structured output: `response_format`.
- Logprobs: `logprobs`, `top_logprobs`.
- DeepSeek extension: `thinking`, `reasoning_effort`, `user_id`.
- Deprecated/no-op: `frequency_penalty`, `presence_penalty`.

Response shape:

- `object: "chat.completion"` for non-streaming.
- `choices[].message.role/content`.
- `choices[].message.reasoning_content` in thinking mode.
- `choices[].message.tool_calls` for function calls.
- `usage.prompt_tokens`, `completion_tokens`, `total_tokens`.
- Context-cache usage: `prompt_cache_hit_tokens`, `prompt_cache_miss_tokens`.
- Reasoning usage: `completion_tokens_details.reasoning_tokens`.

Finish reasons:

- `stop`
- `length`
- `content_filter`
- `tool_calls`
- `insufficient_system_resource`

## Streaming

Set `stream: true`. DeepSeek returns data-only SSE ending with `data: [DONE]`.

Chunk shape:

- `object: "chat.completion.chunk"`.
- `choices[].delta.role`.
- `choices[].delta.content`.
- `choices[].delta.reasoning_content` in thinking mode.
- `choices[].finish_reason`.
- With `stream_options.include_usage: true`, docs say a final usage chunk appears before `[DONE]` and normal chunks have `usage: null`.

DeepSeek can emit empty lines for pending non-streaming requests and SSE comments such as `: keep-alive` for pending streaming requests. A parser should ignore both.

Live note: the local smoke test saw successful streaming chunks. A subagent reported no final usage chunk in one low-token stream even though `include_usage` was set, so clients should handle both documented and absent usage chunks.

## Thinking Mode

Request controls:

```json
{
  "thinking": { "type": "enabled" },
  "reasoning_effort": "high"
}
```

- `thinking.type`: `enabled` or `disabled`; default is `enabled`.
- `reasoning_effort`: documented values `high` and `max`; `low` and `medium` map to `high`, and `xhigh` maps to `max`.
- OpenAI SDK callers may need `extra_body={"thinking": {"type": "enabled"}}`.
- In thinking mode, docs state `temperature`, `top_p`, `presence_penalty`, and `frequency_penalty` do not take effect.
- `reasoning_content` appears beside final `content`.
- Reasoning-mode multi-turn clients should not blindly replay prior `reasoning_content` in normal messages unless the feature path explicitly requires it.

Router translation note: DeepSeek direct uses `thinking`, `reasoning_effort`,
and `reasoning_content`. OpenRouter uses a `reasoning` request object and may
return `reasoning` or `reasoning_details`. Normalize these into separate
internal reasoning fields instead of replaying them as normal assistant text.

## Tool / Function Calling

Tool calling uses OpenAI-style `tools` and `tool_choice`.

- Only tool type `function` is documented.
- Up to 128 functions.
- Function names: letters, digits, underscore, dash, max length 64.
- `tool_choice`: `none`, `auto`, `required`, or a named function selector.
- Tool call arguments are a JSON string. Docs warn they may be invalid JSON or contain hallucinated parameters, so callers must validate.
- Strict tool mode is beta and requires `https://api.deepseek.com/beta` plus `function.strict: true`.

Strict schema subset:

- Supported: `object`, `string`, `number`, `integer`, `boolean`, `array`, `enum`, `anyOf`.
- Every object property must be listed in `required`.
- `additionalProperties` must be `false`.
- Unsupported examples include string `minLength`/`maxLength` and array `minItems`/`maxItems`.

## JSON Output

JSON mode uses:

```json
"response_format": { "type": "json_object" }
```

Rules and gaps:

- Prompt must explicitly ask for JSON and should include an example.
- Set enough `max_tokens` to avoid truncation.
- Docs warn JSON mode may occasionally return empty content.
- Chat `response_format.type` supports `text` and `json_object`.
- No official OpenAI-style `response_format: {"type": "json_schema"}` support was found. Schema-constrained output is documented through strict tool/function calling instead.

Router consequence: a generic `json_schema` structured-output request should be
rejected for direct DeepSeek, downgraded to `json_object`, or translated to beta
strict tool mode only when that behavior is explicit to the caller.

## Beta Prefix and FIM

Chat prefix completion requires `https://api.deepseek.com/beta` and a final assistant message with `prefix: true`. The beta path can also accept `reasoning_content` for thinking-mode continuation.

FIM uses `POST /completions` on `https://api.deepseek.com/beta`.

FIM request fields:

- `model`
- `prompt`
- `suffix`
- `max_tokens`
- `stream`
- `stream_options.include_usage`
- `echo`
- `logprobs`
- `stop`
- `temperature`
- `top_p`
- Deprecated/no-op: `frequency_penalty`, `presence_penalty`

The response object is `text_completion`. The FIM reference schema lists `deepseek-v4-pro`; subagent live tests reported `deepseek-v4-flash` also returned `text_completion`.

## Context Caching and `user_id`

Context caching is documented as enabled by default with no request changes. Usage includes cache-hit and cache-miss prompt token fields.

`user_id` can support content-safety isolation, KV-cache isolation, and scheduling isolation. It must match `[a-zA-Z0-9\-_]+`, max length 512, and must not contain private user information. Concurrency limits are account-level by default, regardless of API key; for accounts with increased quotas, per-`user_id` limits can apply.

DeepSeek cache telemetry uses fields such as `prompt_cache_hit_tokens` and
`prompt_cache_miss_tokens`. OpenRouter exposes cache data through different
usage fields and, for response caching, cache headers. Normalize prompt cache
hit, prompt cache miss, cache write, and response-cache status separately.

## Account / Billing

`GET /user/balance` returns:

- `is_available`
- `balance_infos[]`
- `currency`
- `total_balance`
- `granted_balance`
- `topped_up_balance`

The local smoke test confirmed status `200` and this schema, with balance values redacted.

## Errors and Rate Limits

Documented status codes:

- `400`: invalid request body format.
- `401`: authentication failed.
- `402`: insufficient balance.
- `422`: invalid parameters.
- `429`: rate limit reached.
- `500`: server error.
- `503`: server overloaded.

Fake-auth live test returned `401` with an `error` object. No stable rate-limit headers were observed in sanitized success/fake-auth results.

DeepSeek documents concurrency limits rather than RPM/TPM quotas:

- `deepseek-v4-pro`: 500 concurrent requests per account.
- `deepseek-v4-flash`: 2500 concurrent requests per account.
- A request counts from send until response completion.
- Exceeding concurrency returns HTTP `429`.

OpenRouter can fall back across providers/models unless constrained; DeepSeek
does not expose equivalent provider fallback. Router-side fallback from DeepSeek
to another credential or provider must be explicit and auditable, not hidden
account/key rotation.

## OpenAI Compatibility Gaps

- No Responses API documented.
- No embeddings endpoint documented.
- No files, batches, fine-tuning jobs, images, or audio endpoints documented.
- Chat message content is documented as text strings, not OpenAI multimodal content arrays.
- `response_format` supports `text` and `json_object`, not `json_schema`.
- `thinking`, `reasoning_effort`, and `user_id` are provider-specific extensions.
- `reasoning_content` is returned and may need special handling.
- `presence_penalty` and `frequency_penalty` are deprecated/no-op.
- In thinking mode, `temperature` and `top_p` are no-op.
- Anthropic compatibility is documented separately and does not imply complete Anthropic feature parity.
- OpenRouter attribution headers such as `HTTP-Referer` and `X-Title` are not
  part of DeepSeek's documented API surface.

## Minimal Examples

Chat:

```bash
curl https://api.deepseek.com/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <redacted>" \
  -d '{
    "model": "deepseek-v4-flash",
    "messages": [{"role": "user", "content": "Hello"}],
    "thinking": {"type": "disabled"},
    "max_tokens": 16
  }'
```

Streaming:

```bash
curl -N https://api.deepseek.com/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <redacted>" \
  -d '{
    "model": "deepseek-v4-flash",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true,
    "stream_options": {"include_usage": true},
    "max_tokens": 16
  }'
```

OpenAI SDK:

```python
from openai import OpenAI

client = OpenAI(api_key="<redacted>", base_url="https://api.deepseek.com")

resp = client.chat.completions.create(
    model="deepseek-v4-flash",
    messages=[{"role": "user", "content": "Hello"}],
    max_tokens=16,
    extra_body={"thinking": {"type": "disabled"}},
)
print(resp.choices[0].message.content)
```

## Live-Test Matrix

Date: 2026-05-30. `DEEPSEEK_API_KEY` was present. All local generation probes used `max_tokens <= 16`. Outputs below are sanitized schema/status observations only.

| Test | Result |
| --- | --- |
| Fake auth `GET /models` | `401`; error object observed. |
| `GET /models` | `200`; schema keys `data`, `object`. |
| `GET /user/balance` | `200`; schema keys `balance_infos`, `is_available`; balance values redacted. |
| Non-stream chat | `200`; schema keys `choices`, `created`, `id`, `model`, `object`, `system_fingerprint`, `usage`. |
| Streaming chat | `200`; SSE event shapes observed. |
| JSON output | `200`; chat completion schema observed. |
| Forced tool call | `200`; chat completion schema observed. |
| Subagent `/v1/models` and `/v1/chat/completions` | `200`; `/v1` alias works live. |
| Subagent beta FIM | `200` for `deepseek-v4-pro` and `deepseek-v4-flash`; `text_completion` shape observed. |
| Anthropic API | Docs-only. |
| Embeddings/files/batches/fine-tuning | No official endpoint found; not tested. |

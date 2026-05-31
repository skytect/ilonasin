# 090 multimodal efforts tools

## Goal

Add full request-shape support for:

- multimodal chat input,
- Codex reasoning effort, reasoning summary, verbosity, and fast service tier,
- Codex function-tool request translation,
- stronger complex-tool smoke coverage across providers.

This keeps the local API strict and OpenAI-compatible while translating only
documented or source-confirmed provider behavior.

## Ground truth

- `docs/ilonasin-architecture.md` says unsupported fields must fail clearly,
  provider escape hatches must be explicit and namespaced, and request bodies,
  tool arguments, tool results, raw provider payloads, full request IDs, and
  full account IDs must not be stored or logged.
- `docs/openrouter-api.md` documents OpenAI-style tools, tool messages,
  multimodal inputs, `modalities`, `image_config`, and `reasoning`.
- `docs/deepseek-api.md` documents text-string chat content, OpenAI-style
  function tools, and DeepSeek-specific `thinking` and `reasoning_effort`.
  It does not document image/audio chat input.
- Local Codex 0.135 source confirms Responses requests use:
  - `input[].content[].type` values `input_text`, `input_image`,
    `output_text`,
  - `reasoning.effort` values `none`, `minimal`, `low`, `medium`, `high`,
    `xhigh`,
  - `reasoning.summary` values `auto`, `concise`, `detailed`, with local
    `none` represented by omitting `reasoning.summary`,
  - `text.verbosity` values `low`, `medium`, `high`,
  - `service_tier` values `priority`, `flex`, with `default` as the explicit
    standard-routing sentinel that is omitted from the upstream request,
  - function tools as Responses tools:
    `{"type":"function","name":...,"description":...,"strict":...,"parameters":...}`,
  - previous function calls and tool results as `function_call` and
    `function_call_output` input items.

## In scope

1. Chat message content parsing.
   - Accept existing string content unchanged.
   - Accept OpenAI chat content arrays for user messages:
     `{"type":"text","text":"..."}` and
     `{"type":"image_url","image_url":{"url":"...","detail":"auto|low|high"}}`.
   - Accept data URLs and remote URLs as opaque provider input. Do not fetch,
     inspect, log, or store them.
   - Reject unsupported content item types, malformed `image_url`, and invalid
     `detail` values before provider HTTP.
   - Keep system, assistant, and tool message content text-only unless already
     allowed by existing tool-call validation.

2. Provider behavior.
   - OpenRouter forwards valid user content arrays unchanged to
     `/chat/completions`.
   - OpenRouter multimodal smoke must use an image-capable model discovered
     from metadata, or a provider route with `provider.require_parameters:true`
     and an explicitly image-capable model. If neither is available, fake smoke
     remains mandatory and real multimodal smoke is reported as skipped.
   - DeepSeek rejects content arrays before credential resolution.
   - Codex translates user content arrays to Responses `input_text` and
     `input_image` items. `input_image` uses
     `{"type":"input_image","image_url":"...","detail":"auto|low|high"}`.
     Assistant text remains `output_text`. System text remains `instructions`.

3. Codex provider options.
   - Add `provider_options.codex.reasoning` with optional `effort` and
     `summary`.
   - Add `provider_options.codex.verbosity`.
   - Add `provider_options.codex.service_tier`.
   - Accept only source-confirmed values:
     - `effort`: `none|minimal|low|medium|high|xhigh`,
     - `summary`: `auto|concise|detailed|none`,
     - `verbosity`: `low|medium|high`,
     - `service_tier`: `default|priority|flex|fast`.
   - Normalize `fast` to `priority`, matching Codex source compatibility.
   - Translate summary `none` to an omitted upstream `reasoning.summary`.
   - Extend Codex model metadata to include supported service tiers. Reject
     `priority` or `flex` clearly when the selected model does not advertise
     support. Omit `default` from the upstream request.
   - Continue rejecting top-level OpenRouter-style `service_tier` for Codex.

4. Codex function-tool translation.
   - Allow OpenAI chat `tools` of type `function` for Codex.
   - Translate OpenAI tool definitions from nested chat shape to Responses
     top-level function tool shape.
   - Preserve schema JSON as data, without interpreting or logging arguments.
   - Allow only `tool_choice:"auto"` for Codex. Reject `none`, `required`, and
     named function tool-choice objects unless later source or live evidence
     proves a compatible Codex request shape.
   - Keep `function.strict:true` rejected globally in this slice. Preserve
     `strict:false` when present.
   - Translate assistant `tool_calls` into Responses `function_call` input
     items.
   - Translate `role:"tool"` messages into Responses `function_call_output`
     input items.
   - Translate Codex Responses `function_call` output items back to
     Chat Completions `message.tool_calls` with `finish_reason:"tool_calls"`
     for non-streaming responses.
   - Translate Codex Responses streaming function-call events into
     Chat Completions streaming `delta.tool_calls`, or reject the upstream
     stream as invalid until the translation is implemented. Request-side
     Codex tools are not complete without response-side translation.
   - Preserve `parallel_tool_calls` behavior from Codex model capability.

5. Complex-tool smoke coverage.
   - Extend existing fake-upstream smokes, not permanent test files.
   - Cover multi-tool schemas, required string choices where supported,
     nested JSON schema preservation, assistant tool-call follow-up, tool
     result follow-up, and streaming tool-call deltas for DeepSeek and
     OpenRouter.
   - Add Codex fake-upstream validation for function tool request translation,
     function-call response translation, function-call follow-up translation,
     multimodal input, reasoning, verbosity, and service tier.
   - Fake-upstream validation errors must report structural labels only. They
     must not include raw request bodies, prompts, completions, tool arguments,
     tool results, image URLs, data URLs, or provider payloads.

6. Real provider smoke.
   - After isolated checks pass, smoke all three provider types using the
     existing credentials under `~/.ilonasin`.
   - Use a temporary copy of the home directory for smoke runs with `chmod 700`
     and `trap` cleanup so copied secrets are removed on success or failure.
   - Keep smoke prompts and image URLs minimal and non-sensitive.
   - Do not print bearer tokens, account IDs, request bodies, image data URLs,
     remote image URLs, provider payloads, tool arguments, or tool results.
   - After smoke, audit logs and SQLite metadata for marker leakage. The audit
     must cover bearer tokens, account IDs, request IDs, prompts, completions,
     image URLs, data URLs, tool arguments, and tool results.
   - Real-provider smoke is capability-gated. A missing image-capable
     OpenRouter model or unsupported Codex service tier is a reported skip, not
     a silent pass.

## Out of scope

- Audio input/output.
- Fetching or validating image bytes locally.
- OpenRouter `modalities` or `image_config` top-level fields unless they are
  needed by the real smoke and have provider-specific validation added here.
- Codex hosted tools such as web search, image generation, namespaces,
  tool-search, or freeform tools.
- Persisted quota tracking and quota pooling.

## Implementation steps

1. Add structured chat content helpers in `internal/openai`.
2. Update provider validation in `internal/provider/chat_validation.go`.
3. Update upstream marshaling for OpenRouter and DeepSeek passthrough/reject
   behavior.
4. Update `internal/provider/codex_responses.go` to translate multimodal input,
   Codex provider options, and function-tool turns.
5. Update fake-upstream check logic in `internal/app/app.go`.
6. Add or extend CLI smoke cases, keeping them as runtime checks rather than
   permanent test files.
7. Run compile, vet, build, serve check, manage check, fake-provider smokes,
   and real-provider smokes.
8. Get three senior code reviews.
9. Commit the plan and implementation with a `Co-Authored-By` line.

## Validation

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin=$(mktemp -d)
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp=$(mktemp -d)
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
rm -rf "$tmp" "$tmpbin"
```

Also run real-provider smoke against a temporary copy of `~/.ilonasin` using
`trap` cleanup and mode `700`, then audit the temporary logs and SQLite
metadata before deleting the copy.

## Review questions for subagents

1. Does this violate the no-raw-body, no-tool-argument, no-image-URL logging or
   persistence constraints?
2. Is the Codex translation compatible with the 0.135 Responses request shape?
3. Are provider rejections strict enough to avoid silently sending unsupported
   DeepSeek or Codex fields?

# Plan 092: Local Responses API Compatibility

Status: draft.

## Goal

Make `ilonasin serve` usable as a Codex CLI custom model provider for basic
text turns by adding a local Responses API entrypoint backed by the existing
routing, credential, fallback, provider-adapter, and metadata boundaries.

## Ground Truth

- `docs/codex-client-red-team.md` shows `codex exec` can target `ilonasin`
  with env-key bearer auth, but all probes fail before inference because
  `ilonasin` only exposes Chat Completions.
- Current Codex source only supports `wire_api = "responses"` for custom model
  providers, and builds `POST {base_url}/responses` with
  `Accept: text/event-stream`.
- Codex's SSE parser accepts `response.created`,
  `response.output_item.done`, and `response.completed` as a minimal assistant
  message stream.
- `docs/ilonasin-architecture.md` keeps local API auth, upstream provider
  credentials, provider adapters, routing, HTTP transport, and SQLite metadata
  as separate boundaries.

## In Scope

1. Add authenticated `POST /v1/responses` and `POST /responses` routes.
   Add bare `GET /models` beside `GET /v1/models` so Codex custom providers
   with a root base URL do not fail model discovery before `/responses`.
2. Decode a strict subset of Codex/OpenAI Responses requests:
   - required `model` using `<provider_instance_id>/<provider_model_id>`,
   - optional string `instructions`,
   - `input` message items with `user`, `assistant`, `system`, or
     `developer` roles, with `developer` translated to internal system
     instructions,
   - text content items of type `input_text`, `output_text`, or `text`,
   - Codex's normal request envelope fields: `stream: true`, `store: false`,
     `tools`, `tool_choice: "auto"`, `parallel_tool_calls`, `include`,
     `prompt_cache_key`, `client_metadata`, `reasoning`, `text`, and
     `service_tier`.
3. Keep local Responses stateless for privacy. Reject `store: true`,
   `previous_response_id`, background mode, and other stateful Responses
   features before upstream dispatch.
4. Reject unsupported local Responses inputs clearly before upstream dispatch:
   - image inputs,
   - tool call outputs and stateful tool-loop inputs,
   - file inputs,
   - `stream: false`,
   - background, previous-response, metadata, and other stateful Responses
     features.
5. Accept opaque client tool definition objects for Codex CLI compatibility,
   but do not forward, persist, or implement local Responses tool-call turns in
   this slice. If an upstream provider returns tool calls through the internal
   Chat Completions result, fail with a clear unsupported error instead of
   emitting incomplete tool events.
6. Translate supported Responses input into an internal non-streaming
   Chat Completions request and dispatch through existing provider adapters.
7. Factor the existing non-streaming Chat execution loop so Chat Completions and
   Responses share credential retry, fallback, health recording, and metadata
   logic without duplicating it.
8. Emit a Codex-compatible SSE response:
   - `response.created` with a top-level `response` object,
   - `response.output_item.done` containing an assistant `output_text`,
   - `response.completed` with `response.id` and full usage shape when usage is
     available.
9. Record the same metadata class as Chat Completions requests, without
   storing prompts, completions, request bodies, response bodies, raw provider
   payloads, raw SSE chunks, secret bearer/API/OAuth tokens, provider request
   IDs, or account IDs. Usage token counts remain allowed scalar metadata.
10. Ensure outbound Codex backend requests remain stateless too:
    `marshalCodexResponsesRequest` must send `store: false`, and
    `serve --check` must fail if the fake Codex backend receives `store: true`.
11. Extend `serve --check` to exercise the local Responses route against the
   existing fake upstream path.

## Out of Scope

- Full local Responses tool loop support and local tool-call event emission.
- Local image input support.
- Local Responses streaming deltas from upstream streaming chunks.
- Codex-compatible `/models?client_version=...` catalog changes unless the
  first Codex CLI smoke proves it blocks this slice.
- Quota tracking or quota pooling.

## Implementation Notes

- Keep the new decoder in the OpenAI/local API layer, not in provider adapters.
- Reuse a factored Chat Completions provider execution helper for credential
  selection, fallback, health recording, and usage metadata.
- Do not forward unknown Responses fields silently. Ignore only explicitly named
  harmless Codex metadata fields, and never persist, log, echo, or forward them
  unless separately implemented.
- Responses option pass-through can be minimal in this slice. If `reasoning`,
  `text.verbosity`, or `service_tier` is present, map it only through the
  existing explicit `provider_options.codex` structure and otherwise keep the
  request strict.
- Merge request-level `instructions` before any `system` input messages when
  translating to Chat Completions.
- Return JSON API errors before opening the SSE stream. Once SSE starts, emit
  only safe local event shapes.
- Logs for the new route must use static endpoint labels and must not include
  raw path/query, request bodies, response bodies, provider payloads,
  `err.Error()` from upstream, request IDs, account IDs, or token-bearing
  values.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
rm -rf "$tmp" "$tmpbin"
```

Then run a Codex CLI smoke against the new route with a temporary
`CODEX_HOME`, `ILONASIN_HOME`, and bearer token. Use disposable credentials
loaded through supported management APIs where possible. If a live credential
copy is unavoidable, copy no logs, cache, request metadata, WAL/SHM files, raw
Codex state, or production account state; use mode `0700`, add a `trap`
cleanup, and scan temp homes, logs, SQLite metadata, and command captures for
forbidden sentinel markers.

# 343 Responses Client Metadata Affinity

## Context

`docs/ilonasin-architecture.md` allows same-provider-instance and same-model
credential pooling, and expects pooling to remain metadata-only and auditable.
Recent slices added request-body affinity for Chat Completions, Anthropic
Messages metadata, Responses `prompt_cache_key`, and safe header fallback.

Codex's Responses client sends `prompt_cache_key` for normal turns, but the
Responses request type also allows `client_metadata`. Codex app-server surfaces
turn-scoped Responses API `client_metadata`, and other Responses-compatible
clients may use stable metadata keys instead of top-level `prompt_cache_key`.
`DecodeResponses` currently allows `client_metadata` but ignores it, so those
clients fall back to headers or no-affinity least-busy routing even when they
provide a safe stable body key.

## Goal

Use safe Responses `client_metadata` values as body-derived credential affinity
when top-level `prompt_cache_key` is absent, without changing public request
acceptance behavior or storing the affinity key.

## Scope

1. Add a `ClientMetadata map[string]string` field to `openai.ResponsesRequest`.
2. Parse optional `client_metadata` permissively for affinity only:
   - absent, `null`, malformed, non-object, non-string value, oversized
     key/value, or excessive entries should keep current public acceptance
     behavior and simply not contribute affinity;
   - use limits consistent with Chat Completions metadata when deciding whether
     a value is usable for affinity.
3. Derive Responses affinity in this order:
   - top-level `prompt_cache_key`;
   - safe `client_metadata` keys that are plausibly session/cache identifiers,
     such as `prompt_cache_key`, `session_id`, `thread_id`, and
     `conversation_id`.
4. Reuse a stricter local affinity safety filter for Responses body affinity
   and server header fallback so request IDs, token markers, account
   identifiers, bearer-like data, JSON/JWT-like values, and oversized values
   are not used. Do not change Chat Completions body-affinity behavior in this
   slice.
5. Keep Responses-to-Chat conversion behavior unchanged except that
   `ChatCompletionRequest.AffinityKey` receives the derived body affinity.
6. Keep header fallback unchanged and lower priority than body affinity.
7. Do not log, store, render, forward, or include the affinity key in request
   metadata, management snapshots, TUI, provider requests, fallback rows, or
   health rows.

## Boundaries

- No credential-pool selection math changes.
- No public API route additions.
- No storage, schema, management, TUI, provider adapter, Anthropic, Chat
  Completions body-affinity, IO logging, or config changes.
- No raw prompts, completions, request bodies, response bodies, provider
  payloads, SSE chunks, tool arguments, tool results, bearer tokens, upstream
  account IDs, API keys, OAuth tokens, or local client tokens in normal logs or
  metadata.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/openai ./internal/server
go test ./...
go vet ./...
```

Run a temporary focused in-package smoke, then remove it before commit:

- decode a Responses request with top-level `prompt_cache_key` and verify it is
  still the affinity key;
- decode a Responses request with only `client_metadata.prompt_cache_key` and
  verify it becomes the affinity key;
- verify top-level `prompt_cache_key` wins over `client_metadata`;
- verify `client_metadata.session_id`, `thread_id`, and `conversation_id` are
  accepted when safe;
- verify unsafe top-level `prompt_cache_key` and `client_metadata` values
  containing token/account/request markers or JSON/JWT-like shapes are ignored
  for affinity;
- verify malformed, non-string, and excessive `client_metadata` values remain
  accepted by request decoding but do not contribute affinity;
- verify route-level behavior keeps body-derived `client_metadata` affinity
  higher priority than header fallback when both are present;
- verify safe header fallback still applies when no body affinity exists,
  including existing `x-codex-window-id` colon-suffix trimming;
- verify Responses-to-Chat conversion sets only `AffinityKey` and does not add
  metadata/provider fields.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify the TUI renders.
5. Remove all temporary artifacts and stop background processes.

During diff review, explicitly verify:

- no new stored metadata fields include the affinity key;
- no provider request marshaling includes `client_metadata` because of this
  change;
- header fallback remains lower priority than body affinity;
- no permanent smoke files remain.

## Acceptance

- Responses `client_metadata` can supply safe routing affinity when
  `prompt_cache_key` is absent.
- Existing top-level `prompt_cache_key` behavior remains higher priority.
- Unsafe or malformed metadata is ignored for affinity without tightening
  current public request acceptance.
- Compile, vet, focused smoke, serve smoke, manage smoke, senior plan review,
  and senior implementation review pass.

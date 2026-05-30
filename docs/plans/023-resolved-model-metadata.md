# Plan 023: Resolved Model Metadata

## Goal

Record the safe resolved model returned by upstream chat providers separately
from the requested local model.

The architecture requires request metadata to preserve both requested and
resolved provider/model fields. The current server records the requested
provider model as the resolved model even when an upstream router returns a
different concrete model. This slice makes that distinction real for chat
responses without storing raw payloads or adding route tracing.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `022`

## Scope

1. Extend typed provider chat summaries:
   - add `ResolvedModel string` to `provider.ChatResult`,
   - add `ResolvedModel string` to `provider.ChatStreamSummary`,
   - keep it as a sanitized model identifier, not a raw response field dump.
2. Extract a safe resolved model from non-streaming OpenAI-compatible chat
   responses:
   - read only top-level `model` from a valid `chat.completion`,
   - accept only values that pass the resolved-model sanitizer described below,
   - treat absent, empty, non-string, or unsafe values as absent,
   - do not reject an otherwise valid upstream response because the model is
     absent or unsafe,
   - keep usage extraction validation behavior unchanged.
3. Extract a safe resolved model from OpenAI-compatible stream chunks:
   - expose the first safe top-level chunk `model` through
     `openai.NormalizedStreamChunk`,
   - capture the first safe value in `provider.ChatStreamSummary`,
   - ignore absent, empty, non-string, or unsafe values,
   - keep existing stream normalization and forwarding behavior unchanged.
4. Record resolved model metadata in the server:
   - use provider result or stream summary `ResolvedModel` when present,
   - otherwise fall back to the requested provider model,
   - leave existing local error paths using the requested provider model.
5. Set Codex chat resolved model explicitly:
   - non-streaming Codex responses use `req.UpstreamModel`,
   - streaming Codex summaries use `req.UpstreamModel`,
   - no Codex route metadata, account metadata, or response payload fields are
     persisted.
6. Make recent request summaries use resolved metadata:
   - include requested provider/model and resolved provider/model in
     `metadata.RequestSummary`,
   - keep existing `ProviderInstanceID` and `ModelID` as resolved display
     fields,
   - make `ilonasin manage` able to show when the requested and resolved model
     differ without exposing unsafe values.
7. Extend smoke checks without permanent tests:
   - fake upstream non-streaming response can return a safe model different
     from the requested model and metadata records it as resolved,
   - fake upstream stream chunks can return a safe model different from the
     requested model and metadata records it as resolved,
   - unsafe non-streaming and streaming model markers are ignored and never
     appear in SQLite, CLI output, TUI output, or local errors,
   - direct storage smoke verifies `RecentRequests` exposes requested and
     resolved fields separately when they differ,
   - `manage --check` shows a safe requested-to-resolved model difference and
     hides unsafe resolved markers,
   - request metadata still records the requested model separately from the
     resolved model,
   - Codex chat records the upstream requested Codex model as resolved.

## Out of Scope

- Resolved provider identity from OpenRouter routing metadata.
- OpenRouter `provider`, `models`, `route`, plugins, BYOK, tracing, generation
  IDs, `/generation`, `/activity`, `/key`, or `/credits`.
- DeepSeek balance or credit metadata.
- Response-side reasoning normalization.
- Cross-provider fallback, cross-model fallback, queueing, rate buckets, or
  credential avoidance based on resolved model.
- SQLite migrations.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- Do not push.
- Storage must not perform HTTP.
- Provider adapters must not import SQLite, TUI, config loaders, or credential
  storage.
- TUI must not mutate `config.toml`.
- `internal/openai` may parse only common OpenAI-compatible response fields.
- Provider-specific routing semantics must stay in `internal/provider`.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, full account IDs, balances, credits,
  generation IDs, or route traces.

## Proposed Package Changes

```text
internal/openai/
  types.go       # safe response and stream model extraction
internal/provider/
  chat.go        # add resolved model fields
  http_chat.go   # carry extracted resolved model through chat summaries
internal/server/
  server.go      # record result resolved model with fallback to requested
internal/metadata/
  metadata.go    # expose requested and resolved fields in summaries
internal/storage/sqlite/
  db.go          # query resolved metadata for recent requests
  smoke.go       # direct storage smoke coverage
internal/tui/
  tui.go         # display resolved model when it differs safely
internal/app/
  app.go         # serve/manage smoke assertions
```

Helper semantics:

```text
safeResolvedModel(raw model) =
  absent, empty, non-string, control chars, or surrounding whitespace -> ""
  length > 256 bytes -> ""
  contains chars outside [A-Za-z0-9._:/+-] -> ""
  contains case-insensitive forbidden privacy markers -> ""
  otherwise -> identifier
```

Forbidden privacy markers include at least bearer, sk-, iln_, oauth, token,
secret, authorization, raw, payload, prompt, completion, body, account, acct_,
request id variants including requestid, req_, balance, credit, and JWT-like `eyJ...`.`...`
prefixes. The allowlist must still permit real provider IDs such as native
DeepSeek IDs and OpenRouter slugs including `/`, `:`, `.`, `_`, `+`, and `-`.

The helper is metadata-only. It must not retain the surrounding provider
response body or stream chunk.

## Smoke Checks

Run:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$tmpbin"' EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
git diff --check
```

Acceptance:

- no permanent tests exist,
- compile/package, vet, build, `serve --check`, `manage --check`, and diff
  whitespace checks pass,
- non-streaming chat records a safe upstream response model as
  `resolved_model` when it differs from the requested model,
- streaming chat records the first safe upstream chunk model as
  `resolved_model` when it differs from the requested model,
- unsafe resolved model markers are ignored and never stored or displayed,
- direct storage smoke proves requested and resolved summary fields remain
  separate when they differ,
- `manage --check` shows a safe requested-to-resolved model difference and
  hides unsafe resolved markers,
- requested model metadata remains intact and separate from resolved model
  metadata,
- Codex chat records its upstream requested model as the resolved model,
- no prompt, completion, request body, response body, raw provider payload, raw
  SSE chunk, tool argument, tool result, bearer token, provider request ID,
  account ID, balance, credit, generation ID, route trace, or unsafe model
  marker appears in SQLite metadata, TUI output, CLI output, or local errors.

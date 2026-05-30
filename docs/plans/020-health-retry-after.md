# Plan 020: Health Retry-After Metadata

## Goal

Wire the existing `health_events.retry_after` SQLite column through typed
provider health metadata.

The architecture says provider and credential health may include retry-after
timestamps. The schema already has `health_events.retry_after`, but the typed
metadata path does not write, read, or display it. This slice records safe
`Retry-After` header timestamps for chat and streaming provider attempts
without changing retry policy, fallback policy, account selection, or provider
request behavior.

## Architecture Inputs

- `AGENTS.md`
- `docs/ilonasin-architecture.md`
- `docs/deepseek-api.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/codex-auth.md`
- `docs/codex-endpoints.md`
- prior plans `001` through `019`

## Scope

1. Extend typed health metadata:
   - add `RetryAfter *time.Time` to `metadata.HealthEvent`,
   - add `RetryAfter *time.Time` to `metadata.HealthSummary`,
   - keep the field as a timestamp only, not a raw header value.
2. Persist and read the existing schema column:
   - `RecordHealthEvent` inserts `retry_after`,
   - `LatestHealth` selects and parses `retry_after`,
   - existing rows with `NULL` retry-after remain valid,
   - no new SQLite migration is needed because `retry_after` already exists.
3. Parse `Retry-After` safely in provider adapters:
   - parse only the standard response header name `Retry-After`,
   - support non-negative integer delta-seconds,
   - support HTTP-date values via the Go standard library HTTP date parser,
   - normalize parsed values to UTC timestamps,
   - ignore invalid, negative, past, empty, repeated, overflowed, or
     unreasonably far-future values,
   - do not reject valid HTTP-date values just because they contain commas,
   - never store the raw header string.
4. Carry retry-after through provider result types:
   - add `RetryAfter *time.Time` to `provider.ChatResult`,
   - add `RetryAfter *time.Time` to `provider.ChatStreamSummary`,
   - optionally add it to `provider.ModelResult` for future model-discovery
     health use, but do not introduce model-discovery health events in this
     slice.
5. Record retry-after for chat health attempts:
   - API-key non-streaming chat records retry-after on upstream HTTP failures
     such as `429` or `503` when the header is safe,
   - API-key streaming records retry-after when an upstream response fails
     before the local stream starts,
   - Codex non-streaming and streaming record retry-after on upstream HTTP
     failures before local response completion or local stream start,
   - success health events leave retry-after empty.
6. Preserve existing retry and fallback behavior:
   - no retry or credential fallback is added for `429`,
   - `Retry-After` does not make a failure retryable,
   - existing availability fallback remains limited to network/timeout and
     `500`, `502`, `503`, `504`,
   - OAuth refresh behavior remains limited to expired access tokens and
     upstream `401`,
   - no account cycling, cross-provider fallback, cross-model fallback, queue,
     scheduler, or rate bucket is added.
7. Display safe retry-after state in `ilonasin manage`:
   - health rows show retry-after only when present,
   - output contains only normalized timestamps,
   - no raw provider header value, provider payload, request body, response
     body, bearer token, account ID, prompt, completion, balance, or credit is
     displayed.
8. Extend smoke checks without permanent tests:
   - fake upstream non-streaming `429` includes a valid `Retry-After` header
     and still does not trigger fallback,
   - fake upstream streaming pre-start `429` includes a valid `Retry-After`
     header and still does not trigger fallback,
   - a fake Codex HTTP failure path includes a valid `Retry-After` header and
     records it without triggering refresh unless the status is `401`,
   - invalid, negative, past, and too-far-future retry-after values are ignored,
   - direct storage smoke records and reads a retry-after timestamp,
   - TUI smoke displays a normalized retry-after timestamp for seeded health
     metadata,
   - privacy scans continue to prove forbidden markers do not appear in SQLite
     metadata, CLI output, TUI output, or local errors.

## Out of Scope

- New SQLite columns or migrations.
- Provider rate bucket scheduling.
- Automatic waiting, queueing, or retry based on `Retry-After`.
- Fallback on `429`.
- OAuth refresh on `429`.
- OpenRouter `/key`, `/activity`, `/generation`, `/credits`, or DeepSeek
  balance polling.
- Persisting raw rate-limit headers, rate-limit IDs, credit counts, balances,
  request IDs, account IDs, or provider error payloads.
- Provider health scoring or credential avoidance based on retry-after.
- Permanent tests.

## Design Constraints

- No permanent `*_test.go` files.
- Do not push.
- Storage must not perform HTTP.
- Provider adapters must not import SQLite, TUI, config loaders, or credential
  storage.
- TUI must not mutate `config.toml`.
- The raw `Retry-After` header value may exist only transiently inside provider
  HTTP response handling.
- Store only parsed UTC timestamps.
- Treat malformed or ambiguous header values as absent.
- Do not store prompts, completions, request bodies, response bodies, raw
  provider payloads, raw SSE chunks, tool arguments, tool results, full bearer
  tokens, full provider request IDs, full account IDs, balances, or credits.

## Proposed Package Changes

```text
internal/provider/
  chat.go       # add retry-after to result summaries
  http_chat.go  # parse Retry-After on upstream HTTP failures
internal/metadata/
  metadata.go   # add retry-after to health event and summary
internal/storage/sqlite/
  db.go         # write/read existing retry_after column
  smoke.go      # direct storage smoke coverage
internal/server/
  server.go     # copy retry-after from provider attempts into health events
internal/tui/
  tui.go        # display retry-after timestamps for health rows
internal/app/
  app.go        # serve/manage smoke assertions
```

Helper semantics:

```text
retryAfterTimestamp(header, now) =
  empty or invalid header -> nil
  non-negative delta seconds -> now + delta
  HTTP date in the future -> parsed UTC date
  past date, negative delta, repeated header value, overflow, or > 365 days -> nil
```

The 365-day cap prevents obviously broken upstream metadata from becoming
long-lived local health state. It is not a rate-limit policy.

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
- health events can persist and read a safe retry-after timestamp,
- non-streaming and streaming upstream `429` with retry-after record health
  metadata without triggering fallback,
- Codex upstream HTTP failure can record retry-after without triggering
  non-401 refresh,
- invalid retry-after values are ignored,
- TUI health output shows only normalized retry-after timestamps,
- no raw prompts, completions, request/response bodies, provider payloads,
  SSE chunks, bearer tokens, provider request IDs, account IDs, balances, or
  credits appear in SQLite metadata, TUI output, CLI output, or local errors.

# 300 Binary IO Logging Policy

## Context

The current logging boundary grew from two different goals:

- normal daemon observability must be safe, metadata-only, and useful without
  exposing request content,
- opt-in IO logging is now being used as a local operator debugging tool where
  raw request, response, SSE, and tool payload fidelity matters.

Plans 077, 102, and 158 pushed the system toward metadata-only logging and a
broad redacting `slog` handler. That made sense for normal logs, but it also
made IO debugging too complex and lossy. The current broad redaction rules
also redact operational fields such as token counts and account-related safe
summaries, which makes the logs harder to interpret.

The desired policy is binary:

- if IO logging is disabled, logs contain only the safe structured subset,
- if IO logging is enabled, IO logs capture full operational payloads needed to
  debug wire-shape issues,
- secrets are never logged in either mode.

## Goal

Replace layered redaction and partial IO metadata with one clear logging
policy.

After this work:

- `ilonasin.log` remains safe structured application logging,
- `ilonasin-io.log` exists only when `[logging].capture_io = true`,
- IO logging records full local request/response bodies and streamed event
  payloads where the server already has those bytes,
- known secret carriers are removed before logging rather than broadly
  redacting unrelated fields,
- provider credentials, local bearer tokens, OAuth tokens, cookies, auth
  headers, device codes, code verifiers, and provider command stdout are never
  written to either log,
- the code path for deciding what gets logged is easy to audit.

## Policy

### Normal Logging

Normal application logs are always the safe subset. They may include:

- event name, level, timestamp, and event ID,
- method and static route label,
- provider instance, provider type, and local credential ID,
- status, latency, retry/fallback counts, and normalized error class,
- request shape counts, usage counts, cache counts, and safe rate metrics.

Normal application logs must not include:

- prompts, completions, request bodies, response bodies, raw provider payloads,
  raw SSE chunks, tool arguments, tool results, auth headers, cookies, bearer
  tokens, API keys, OAuth tokens, device codes, code verifiers, provider
  command stdout, or raw token endpoint bodies.

### IO Logging

IO logging is explicitly operator-local and opt-in. When enabled, it may
include:

- local inbound request bodies after known local auth headers are excluded,
- local outbound response bodies,
- local Responses and Chat Completions SSE event payloads,
- upstream provider request and response bodies when captured at adapter
  boundaries,
- tool call/result payloads as they pass through local request/response bodies,
- safe headers needed for debugging content negotiation and streaming, such as
  `content-type` and static route labels.

IO logging must still strip:

- `Authorization`,
- `Cookie`,
- `Set-Cookie`,
- `Proxy-Authorization`,
- local client tokens,
- upstream API keys,
- OAuth access, refresh, and ID tokens,
- Codex agent identity assertions,
- OAuth device/user codes,
- authorization codes,
- PKCE code verifiers,
- provider command stdout,
- any configured credential secret values.

## Scope

1. Split normal logs from IO logs in code.
   - Keep `internal/logging/logging.go` for normal structured logs.
   - Keep `internal/logging/io.go` for opt-in IO records.
   - Do not route IO records through the normal `slog` redacting handler.
2. Replace broad key-substring redaction for normal logs with explicit safe
   attribute constructors or a narrow denylist for secret carriers only.
3. Add a small secret scrubber for IO logging.
   - It should strip known secret headers and known token fields.
   - It should not redact ordinary payload fields such as `prompt_tokens`,
     `completion_tokens`, `account_hash`, `reasoning_tokens`, or `cache_hit`.
4. Restore IO body capture where the server already has local request and
   response bytes.
5. Extend capture to streaming event writes where response bytes are written in
   chunks.
6. Keep file permissions strict: log directory `0700`, log files `0600`.
7. Keep `[logging].capture_io` as the config switch for this slice.
8. Update architecture docs to reflect the binary policy and explicitly mark
   IO logging as opt-in local debugging.
9. Do not add permanent tests.

## Non-Goals

- No remote log shipping.
- No TUI log viewer.
- No new SQLite storage for raw payloads.
- No persistence of IO payloads outside `ilonasin-io.log`.
- No config rename from `capture_io`.
- No attempt to classify arbitrary user content as secret.

## Implementation Steps

1. Inventory every log call and classify it as normal log or IO log.
2. Define safe normal-log helpers for common fields and remove the broad
   substring redaction from values that are safe metrics.
3. Implement IO scrubbing as a focused boundary:
   - strip secret headers,
   - strip known JSON token fields,
   - strip configured credential secret values if they appear exactly,
   - leave non-secret request and response content intact.
4. Update `internal/server/io_logging.go` so `capture_io` writes full local
   input/output payload records plus byte counts.
5. Add streaming capture at the response writer layer without changing stream
   flushing behavior.
6. Review provider adapter boundaries and add upstream IO records only where
   request and response bytes are already available without extra buffering
   risk.
7. Update `docs/ilonasin-architecture.md` and supersede the metadata-only IO
   statement from plan 158 in this plan.
8. Run direct compile, vet, serve, manage, and disposable IO logging smokes.

## Smoke Checks

Run direct checks rather than adding permanent tests:

```sh
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
```

Then run a disposable `capture_io = true` daemon and verify:

- `ilonasin.log` has safe metadata and no body payloads,
- `ilonasin-io.log` contains the disposable prompt marker and response/error
  body marker,
- `ilonasin-io.log` does not contain the generated local bearer token,
- `ilonasin-io.log` does not contain upstream credential secret values,
- streamed routes write multiple output records or one complete captured stream
  without changing client-visible bytes,
- `capture_io = false` produces no `ilonasin-io.log`.

## Acceptance

- Normal logs expose safe operational metadata without accidental redaction of
  metrics.
- IO logs are useful for debugging wire-shape and TUI fidelity issues because
  they preserve local payloads when explicitly enabled.
- Known secrets are absent from both normal logs and IO logs.
- The implementation has a single clear decision point for whether payloads may
  be logged: `[logging].capture_io`.
- The deployment smoke passes with active Codex traffic after switching.

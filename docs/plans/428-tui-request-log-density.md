# 428 TUI Request Log Density

## Context

The logs tab already renders recent requests with a table-like header and a
wrapped row. Each request still expands into several extra detail lines:

- route/model line;
- token/cache line;
- retry/credential/latency line;
- optional extras line.

The user asked for logs to become more table-like and less text-heavy while
still wrapping rather than ellipsizing. This slice focuses only on the request
metadata pane in the logs tab.

## Goal

Make request log rows denser by consolidating detail lines under the existing
table row, while preserving full wrapped model/credential/error visibility and
metadata-only semantics.

## Scope

1. Update `internal/tui/log_requests.go`.
2. Keep the existing request table header, separator, and first table row.
3. Replace the separate `model`, `tokens`, and `retry` detail lines with two
   compact wrapped detail rows:
   - route/model plus credential;
   - token/cache mix plus attempt/retry/latency/performance chips.
4. Keep optional extras visible and wrapped.
5. Do not ellipsize model route, credential labels, error classes, service
   tiers, reasoning, or thinking values.
6. Preserve blank-line separation between request blocks.
7. Do not change management DTOs, storage, request metadata recording, logging
   policy, API behavior, provider behavior, config, or subscription usage
   rendering.
8. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- a request row still renders the table row with status, route, HTTP status,
  time, stream mode, credential, attempt counts, latency, and token count;
- long model routes wrap rather than ellipsize;
- credential labels remain visible when safe and fall back to credential ID when
  redacted or missing;
- token/cache details still render prompt, completion, reasoning, cache hit,
  cache miss, cache write, total, and cache hit-rate information when present;
- retry/auth/fallback counts and latency/performance chips still render;
- error class, fallback reason, requested/effective service tiers, reasoning
  effort, and thinking type extras still render and wrap without ellipsis.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Request log rows are more compact and scan like table entries with concise
  wrapped details.
- Important metadata remains visible and wrapped.
- No runtime behavior outside TUI rendering changes.

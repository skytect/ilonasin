# 425 Stream Quota Status Helper

## Context

`internal/server/chat_stream.go` has two places that normalize a
`provider.ChatStreamSummary` into the local status/error-class pair used for
quota behavior:

- `quotaRetryableStreamAttempt`;
- quota observation recording inside `executeStreamingChat`.

Both paths currently repeat the same rules:

- status defaults to `502` when the summary status is zero;
- an empty error class with a `4xx` or `5xx` status becomes
  `upstream_http_error`;
- quota behavior then delegates to `isQuotaObservation`.

`docs/ilonasin-architecture.md` requires quota pooling to use only local quota
observations produced by routed requests. Keeping stream quota normalization in
one helper makes the retry and observation paths easier to audit together.

## Goal

Add one private helper for stream quota status/error normalization and use it
from both stream quota retry and stream quota observation code, without changing
retry decisions, observation rows, fallback events, streaming response behavior,
metadata, storage, logging, management, TUI, config, or provider behavior.

## Scope

1. Add a private helper in `internal/server/chat_stream.go`, for example
   `streamQuotaStatusAndError(summary provider.ChatStreamSummary) (int, string)`.
2. Keep the helper behavior exactly equal to the duplicated current code:
   - status is `summary.StatusCode`;
   - if status is `0`, use `http.StatusBadGateway`;
   - error class is `summary.ErrorClass`;
   - if error class is empty and status is `>= 400`, use
     `upstream_http_error`.
3. Use the helper in:
   - `quotaRetryableStreamAttempt`, after preserving the existing
     `sinkStarted || summary.Started` guard;
   - `executeStreamingChat` when deciding whether to append a quota
     observation.
4. Keep `isQuotaObservation` unchanged.
5. Keep non-streaming chat helpers unchanged in this slice.
6. Do not change auth retry, availability retry, quota retry, fallback event,
   health event, request metadata, route response, storage, management, TUI,
   config, logging, or provider adapter behavior.
7. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- zero stream status normalizes to `502`;
- empty error class with `429` normalizes to `upstream_http_error`;
- non-empty error class is preserved;
- `quotaRetryableStreamAttempt` still returns false when the sink or upstream
  stream has started;
- `quotaRetryableStreamAttempt` still returns true for normalized `429` and
  quota error classes before the stream starts;
- `quotaRetryableStreamAttempt` still returns true for quota-class-only
  summaries with zero status, such as `StatusCode: 0` plus
  `rate_limit_exceeded` or `insufficient_quota`;
- stream quota observation classification still uses the same normalized
  status/error pair.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Stream quota status/error normalization has one implementation point.
- Stream quota retry and observation behavior are unchanged.
- Non-streaming chat, auth retry, availability retry, fallback metadata, health
  metadata, storage, logging, management, TUI, config, and providers are
  unchanged.

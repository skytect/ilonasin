# 482 TUI Request Log Endpoint Rollup

## Context

The current TUI request log repeats status, IO policy, route, token, timing,
attempt, and input fields across every recent request row. That makes the logs
hard to scan, especially when many rows share the same endpoint. User feedback
asked for less repetition, more digestible presentation, endpoint-level
summaries instead of per-event detail where possible, and a compact IO policy
display.

Broader backlog to handle in later slices:

- sweep all panes for repeated chips and duplicate summary elements,
- make IO policy and pruning compact,
- continue replacing text-heavy panes with tables, meters, and aligned rows,
- keep panes independently scrollable and prioritize subscription pools,
- show downstream API key usage clearly,
- make refresh/live data behavior feel automatic,
- review all keybindings for a parsimonious, intuitive set,
- refactor logging policy around binary IO capture versus metadata-only logs,
- revisit file logging durability and queryability,
- improve Codex account pooling with safe cache-affinity and load distribution.

## Goal

Make the request metadata pane easier to scan by grouping recent requests by
endpoint and reducing repeated per-event details while preserving metadata
needed for diagnosis.

## Scope

1. Update only TUI request-log rendering helpers, expected in
   `internal/tui/log_requests.go`.
2. Keep storage, management DTOs, routing, logging policy, keybindings, and
   snapshot refresh behavior unchanged.
3. Do not add permanent tests.

## Implementation

1. Replace the multi-line route chip summary in the request overview with an
   endpoint rollup table.
2. Group recent request rows by endpoint and show count, OK, warning, error,
   stream count, total tokens, average latency, and average TTFT.
3. Render IO capture policy as one compact line in the overview instead of
   repeating it with the request count and route counts.
4. Remove the repeated `endpoint` detail field from each request event, since
   endpoint is now represented by the rollup and the event table.
5. Preserve per-event token and timing diagnostics, but combine them into one
   compact aligned `io` detail field instead of two separate verbose rows.
6. Keep per-event diagnostic details for route, credential, attempts, inputs,
   fallback, service tier, and reasoning/thinking metadata.
7. Run `gofmt` on touched Go files.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run a direct CLI smoke by building a temporary `ilonasin` binary, starting
`ilonasin serve` with an isolated temporary home and config, checking the
management snapshot over the Unix socket, running bounded `ilonasin manage` at
70, 100, and 140 columns, then cleaning up all temporary files and processes.

## Acceptance

- Request metadata starts with a concise status line and endpoint rollup.
- Per-event request rows no longer repeat endpoint detail.
- Per-event token and timing metadata remains available in a compact diagnostic
  line.
- IO capture policy is visible but compact.
- Request event rows remain available in Logs.
- No storage, routing, logging-policy, or keybinding behavior changes.

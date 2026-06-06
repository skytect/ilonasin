# 484 TUI Empty State Dedup

## Context

Several TUI panes repeat the same empty-state vocabulary:

- disabled cards,
- `visibility metadata-only`,
- `state quiet`,
- `content redacted`,
- repeated zero-count chips.

This makes quiet dashboards look busier than live dashboards. The architecture
requires metadata-only boundaries to remain visible, but the UI should express
that boundary once per empty block instead of as repeated card chrome.

## Goal

Deduplicate empty-state rendering for usage and log panes with one compact,
shared helper.

## Scope

1. Add a shared TUI helper for compact empty states, expected in
   `internal/tui/visual_cards.go`.
2. Update empty states in:
   - `internal/tui/usage_metrics.go`,
   - `internal/tui/usage_health.go`,
   - `internal/tui/log_requests.go`,
   - `internal/tui/log_fallbacks.go`.
3. Keep non-empty row rendering unchanged.
4. Do not change DTOs, storage, routing, logging policy, snapshot refresh,
   keybindings, or management APIs.
5. Do not add permanent tests.

## Implementation

1. Add `renderCompactEmptyState(width, status, title string, parts ...string)
   string` that renders the existing status semantics, title, and wrapped
   detail chips without card borders.
2. Replace repeated quiet empty cards and metric lines in the scoped panes with
   that helper.
3. Keep metadata/content boundaries visible with concise labels, including the
   current request-log IO capture mode derived from `m.runtime.CaptureIO`.
4. Remove newly dead imports where card rendering is no longer used.
5. Run `gofmt` on touched Go files.
6. Build the helper from existing safe display and wrapping helpers so the
   change does not introduce clipping, ellipsizing, unsafe display paths, or
   new sanitization/redaction decisions.

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
70, 100, and 140 columns, capturing the terminal output for the scoped empty
usage/log panes, confirming `metadata-only`, `content redacted`, and the IO
capture mode remain visible without ellipsizing or overlap, then cleaning up
all temporary files and processes.

## Acceptance

- Scoped empty states use one shared compact helper.
- Empty usage and log panes consume less vertical and visual space.
- Metadata-only and redacted-content boundaries remain visible.
- Existing status semantics such as disabled and quiet remain visible.
- The helper accepts only display strings already derived from metadata DTO
  fields; it introduces no IO-bearing fields, direct storage reads, or policy
  decisions.
- Non-empty data rendering remains unchanged.
- No behavior changes outside TUI rendering.

# 487 TUI Downstream Usage Strip

## Context

Plan 459 exposed downstream local API-key usage in the management snapshot and
the API pane. The API pane now shows per-token usage rows, but the summary still
reads as a sequence of chips. The backlog asks for downstream API-key usage to
be monitorable and for text-heavy panes to become more visual.

The TUI already receives daemon-owned `local_token_usage` metadata. This slice
should improve presentation only.

## Goal

Add a compact downstream activity strip to the API `downstream keys` pane,
showing request status mix, token mix, cache/reasoning rates, latency, and latest
activity from existing local-token usage metadata.

## Scope

1. Update `internal/tui/api_local_tokens.go` only.
2. Split downstream-token summary responsibilities:
   - the section banner carries inventory only: local API scope plus enabled and
     disabled token counts;
   - the aggregate strip carries usage totals for current visible tokens only;
   - unknown/deleted-token usage stays in its existing separate row so it is not
     visually counted twice;
   - per-token rows keep per-token identity, lifecycle, and per-token usage;
     only current-token aggregate totals are removed from adjacent overview lines.
3. Extend the existing local-token overview rendering to include:
   - status mix totals across current visible downstream tokens;
   - token mix visual using existing `compactTokenMixLine`;
   - cache/reasoning rates using existing compact meter helpers;
   - average latency and latest request time.
4. Keep per-token rows, empty state, token creation/disable behavior,
   management DTOs, storage queries, config, routing, logging, and keybindings
   unchanged.
5. Do not add permanent tests.

The aggregate strip should be at most three visual groups:

- total request/status mix;
- token mix line;
- cache/reasoning/latency/latest activity line.

At 70 columns the groups may stack on separate wrapped lines. At 100 and 140
columns they may be denser. No content should be clipped or ellipsized. The
implementation should prefer existing wrapping helpers and compact meters over
adding long prose.

It should replace repeated aggregate chips rather than adding another text block.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a direct CLI smoke with a temporary binary and isolated home:

- start `ilonasin serve`;
- create at least two local tokens over the management socket;
- use temporary seeded request metadata as the source of truth when live
  provider traffic cannot produce deterministic token/cache/reasoning values;
- seed or otherwise produce known rows equivalent to:
  - token A: requests 2, ok 1, warning 1, error 0, prompt 100, completion 40,
    total 140, reasoning 10, cache hit 30, cache write 20, average latency
    200ms, latest `2026-01-02T03:04:05Z`;
  - token B: requests 1, ok 0, warning 0, error 1, prompt 50, completion 20,
    total 70, reasoning 5, cache hit 10, cache write 5, average latency 500ms,
    latest `2026-01-02T03:05:05Z`;
- verify the aggregate source values are: requests 3, ok 1, warning 1, error 1,
  prompt 150, completion 60, total 210, reasoning 15, cache hit 40, cache write
  25, cache miss 110, request-weighted average latency 300ms, latest
  `2026-01-02T03:05:05Z`;
- run bounded `ilonasin manage` through a PTY at 70, 100, and 140 columns;
- confirm the API downstream pane renders banner inventory, status mix, token mix,
  cache/reasoning rates, latency, and latest activity without ellipsizing or
  duplicating aggregate request/token/latest values in adjacent summary lines.

## Acceptance

- The API downstream pane shows aggregate current-token downstream usage visually before
  individual token rows.
- Aggregate inventory and current-token aggregate usage are not repeated across
  adjacent banner and overview lines. Per-token rows still show per-token usage.
- No local token secret material, token hashes, prompts, completions, or raw
  payloads are newly displayed.
- All data still comes from daemon-owned management snapshots.
- No behavior changes outside TUI rendering.

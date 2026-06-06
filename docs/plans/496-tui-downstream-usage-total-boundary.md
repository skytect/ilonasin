# 496 TUI Downstream Usage Total Boundary

## Context

Plan 490 found that the API downstream-key aggregate has inconsistent semantics:
normal rendering totals only current visible local tokens, while the empty state
totals all retained downstream usage. The separate unknown/deleted-token row is
visible, but the aggregate label does not make it clear whether unknown/deleted
usage is included.

The architecture expects the management TUI to expose metadata-only usage totals
and downstream API-key usage clearly. The TUI already receives daemon-owned
`local_token_usage` metadata, so this slice should only clarify rendering
semantics.

## Goal

Make downstream local-token usage aggregates have one clear semantic in the API
`downstream keys` pane: all retained downstream usage from the management
snapshot, with current-token rows and unknown/deleted-token rows presented as
breakdowns of that same total.

## Scope

1. Update `internal/tui/api_local_tokens.go` only.
2. Change local-token overview usage aggregation to include every
   `management.LocalTokenUsageSummary` row, including `LocalTokenID == 0`.
3. Keep lifecycle inventory, enabled/disabled counts, newest created, and latest
   disabled scoped to current token rows.
4. Label the overview as all retained downstream usage so it cannot be confused
   with current-token-only usage.
5. Keep the unknown/deleted row visible as a breakdown, but make clear it is
   included in the aggregate total.
6. Preserve per-token rows, local token mutations, management DTOs, storage,
   routing, logging, config, and keybindings.
7. Do not display local token secrets, token hashes, prompts, completions, raw
   payloads, account IDs, or request bodies.
8. Do not add permanent tests.

## Verification

Run:

```sh
gofmt -w internal/tui/api_local_tokens.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes with a temporary binary:

1. Start `ilonasin serve` with isolated home and a valid config.
2. Verify management health and snapshot over the Unix socket.
3. Run bounded `ilonasin manage` through a PTY at narrow and wide widths.
4. Clean up all temporary files and processes.

Run a temporary focused rendering check and remove it before commit:

1. Build `localTokenOverviewFromRows` input with two current token rows and
   three usage rows: token A, token B, and `LocalTokenID == 0`.
2. Verify the overview request/status/token/cache/latency/latest totals include
   all three usage rows.
3. Verify current-token inventory fields still come only from the two current
   token rows.
4. Verify the unknown/deleted usage row remains renderable and separately
   labeled as included in the aggregate.

## Acceptance

- The API downstream usage aggregate has one visible semantic: all retained
  downstream usage in the management snapshot.
- Unknown/deleted usage remains visible as its own row and is clearly a
  breakdown included in the aggregate, not a separate side total.
- Current-token lifecycle inventory remains scoped to current token rows.
- Empty and non-empty token states use the same usage total semantics.
- No management, storage, routing, logging, config, or keybinding behavior
  changes.

# 483 TUI Compact IO Pruning Pane

## Context

The Logs screen still gives IO policy and pruning too much visual weight.
Request logs now have their own compact IO policy line, so the dedicated
`io policy + pruning` pane should become a concise operational status block
instead of another broad chip cluster.

Prior plans 458 and 472 identified this same problem. This slice implements the
small rendering cleanup while preserving behavior.

## Goal

Compact the IO policy and pruning pane without hiding the logging boundary or
manual prune state.

## Scope

1. Update only `internal/tui/log_pruning.go`.
2. Keep data sourced from existing `Model` runtime, snapshot rows, and
   `pruneResult`.
3. Preserve visible metadata row counts, IO capture mode, storage boundary,
   manual 30 day pruning, and last-prune result when present.
4. Do not change pruning behavior, logging behavior, management APIs, SQLite,
   config handling, snapshot refresh, or keybindings.
5. Do not add permanent tests.

## Implementation

1. Replace the broad `pruningPolicyParts` chip cluster with a compact
   two-line pane:
   - one line for metadata counts,
   - one line for IO mode, storage target, content boundary, and prune mode.
2. Keep the last-prune result as a single aligned detail line.
3. Remove or adjust helpers that become misleading after compaction.
4. Run `gofmt` on touched Go files.

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

- The pruning pane normal state is compact and no longer reads as a large card.
- IO capture mode and metadata-only boundary remain visible.
- Manual 30 day pruning and last-prune counts remain visible.
- No runtime behavior changes.

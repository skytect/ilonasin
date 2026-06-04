# 472 TUI Compact Pruning Policy

## Context

The logs screen still gives the IO policy and pruning state too much vertical
space. `writePruning` currently renders one line for metadata counts and a
second line for IO policy, content, retention, prune mode, and cutoff. That
repeats the same metadata-only state already visible in request and log
summaries.

The active TUI backlog calls out shrinking IO policy/pruning display and
deduplicating repeated labels while keeping operational detail visible.

## Goal

Compact the IO policy and pruning pane so the normal state is easier to scan
without removing the policy facts.

## Scope

1. Update only `internal/tui/log_pruning.go` unless gofmt requires otherwise.
2. Keep all data sourced from the existing `Model` snapshot/runtime fields.
3. Preserve the optional last-prune result line.
4. Do not change pruning behavior, management APIs, SQLite storage, keybindings,
   logging policy, or config handling.
5. Do not add permanent tests.

## Implementation

1. Replace the two normal-state metric lines in `writePruning` with one compact
   summary line.
2. Add a small helper that builds the compact policy/pruning parts from the
   existing row counts and IO capture state.
3. Keep existing IO helper functions where they still describe the compact
   line, and remove any helper that becomes dead.
4. Run `gofmt` on touched Go files.

## Verification

Run:

```sh
git diff --check
go test ./internal/tui
go test ./...
go vet ./...
```

Run a direct CLI smoke by building a temporary `ilonasin` binary, starting
`ilonasin serve` with an isolated temporary home and config, checking the
management health and snapshot endpoints over the Unix socket, running bounded
`ilonasin manage` at several terminal widths, then cleaning up all temporary
files and processes.

## Acceptance

- The pruning pane normal state consumes one policy/count line instead of two.
- The line still exposes request, fallback, health, quota, IO mode, storage
  policy, and manual 30 day pruning.
- Last-prune details still render when present.
- No unrelated files are changed or staged.

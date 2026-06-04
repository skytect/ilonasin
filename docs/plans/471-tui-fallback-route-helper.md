# 471 TUI Fallback Route Helper

## Context

The fallback log view renders the provider/model route in both the summary row
and the detail rows. The detail rows already use `fallbackRouteDisplay`, while
the summary row reconstructs the same string locally.

This is small duplication in a code path that has been getting progressively
cleaned up to make the logs view easier to maintain.

## Goal

Use one fallback route display helper for the fallback summary table and the
fallback detail rows.

## Scope

1. Update only `internal/tui/log_fallbacks.go` unless gofmt requires otherwise.
2. Keep rendered behavior unchanged.
3. Do not change management DTOs, storage, routing behavior, keybindings, or
   snapshot refresh behavior.
4. Do not add permanent tests.

## Implementation

1. Change `fallbackTableRow` to use `fallbackRouteDisplay(row)` for its route
   cell.
2. Remove the local provider/model route reconstruction from `fallbackTableRow`.
3. Run `gofmt` on touched Go files.

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

- `fallbackTableRow` and `fallbackDetailFields` share the same route formatter.
- Fallback table wrapping still works at narrow and wide widths.
- No unrelated files are changed or staged.

# 473 TUI Request Route Helper

## Context

The request log view renders provider/model route strings in both the compact
table and the detail rows. `compactRequestTableDetail` and `requestModelRoute`
currently assemble those route strings separately, which keeps small display
edge cases and sanitization choices duplicated.

The recent fallback log cleanup moved the same kind of route display through one
helper. Request logs should follow that pattern.

## Goal

Use one request route display helper for compact request table routes and
request detail route rows.

## Scope

1. Update only `internal/tui/log_requests.go` unless gofmt requires otherwise.
2. Keep rendered behavior equivalent for normal rows.
3. Improve partial route display by avoiding synthetic slash separators when one
   side of the route is empty.
4. Do not change request metadata, management DTOs, storage, routing behavior,
   keybindings, or snapshot refresh behavior.
5. Do not add permanent tests.

## Implementation

1. Add a helper that formats provider/model as `provider/model`, `provider`,
   `model`, or `none` through the existing safe display path.
2. Use that helper in `compactRequestTableDetail`.
3. Use that helper in `requestModelRoute` for both requested and resolved route
   display.
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

- Compact request table route display and detail route display share the same
  formatter.
- Rows with both provider and model still render as `provider/model`.
- Partial rows render without a stray leading or trailing slash.
- No unrelated files are changed or staged.

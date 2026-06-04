# 464 TUI Fallback Log Detail Rows

## Goal

Make fallback logs match the request log table/detail-row style so wrapped log
entries are easier to scan.

## Scope

- Keep the existing fallback table header and main table row.
- Replace the chip-heavy fallback `meta` detail line with aligned key/value
  detail rows.
- Preserve route, fallback reason, source credential, and target credential.
- Do not change request logs, pruning display, storage, management DTOs,
  fallback metadata recording, keybindings, or pane layout policy.
- Do not add permanent tests.

## Verification

- `gofmt` on touched Go files.
- `git diff --check`.
- `go test ./internal/tui`.
- `go test ./...`.
- `go vet ./...`.
- Build a temporary binary, run isolated `serve`, verify management health and
  snapshot over the Unix socket, run bounded `manage` at 70, 100, and 140
  columns, then clean up.

## Risks

- Long provider/model IDs and fallback reasons must wrap without truncation.
- Credential labels must remain safe display fragments only.
- Sharing request-log detail helpers must not blur request-specific labels with
  fallback-specific labels.

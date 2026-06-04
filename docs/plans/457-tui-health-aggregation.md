# 457 TUI Health Aggregation

## Goal

Make the Usage health pane less repetitive by summarizing the management snapshot's current health rows by route and credential state.

## Scope

- Change only the TUI rendering path for health rows.
- Group `management.HealthSummary` rows in `internal/tui/usage_health.go` by provider, model, credential, event class, HTTP status, and error class so duplicate snapshot rows collapse without inventing unavailable ledger statistics.
- Show concise current-state summaries with row count, HTTP status, error class, latest observed time, and retry-after when present.
- Do not label or present rows as event history in the Health pane.
- Do not change management DTOs, SQLite schema, provider behavior, logging policy, or routing.

## Verification

- Temporary render check for widths 70, 100, and 140, then remove it.
- `gofmt` on touched Go files.
- `git diff --check`.
- `find . -name '*_test.go' -type f -print`.
- `go test ./internal/tui`.
- `go test ./...`.
- `go vet ./...`.
- Build a temp `ilonasin` binary, run isolated `serve`, smoke `/health` and `/snapshot`, and open `manage` at 70, 100, and 140 columns with cleanup afterward.

## Risks

- Over-aggregation could hide a failing credential if the group key is too broad, so status and error class stay in the key.
- Rendering must remain readable at narrow widths without ellipsizing.
- The current management snapshot is latest health state, not the full health ledger, so the TUI must not imply historical counts.

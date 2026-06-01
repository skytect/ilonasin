# 225 TUI Remove Legacy Viewport

## Goal

Remove TUI code that became dead after the `api`, `providers`, `usage`, and
`logs` pane dashboard migration. The active TUI render path should be the pane
dashboard only, with pane-local scrolling as the single scrolling model.

## Current Evidence

- `layout.go` renders `m.renderDashboard()`.
- `panes.go` owns pane focus and pane-local scroll offsets.
- `layout.go` still contains `activeTabBody` and `tabBody`, but they are only
  used by old viewport helpers.
- `viewport.go` still contains `renderViewport`, tab-wide scroll helpers, and
  `scrollOffsets` clamping.
- `overview.go`, `accounts.go`, and `observability.go` define old whole-tab
  composers that are no longer referenced by the active dashboard.
- `overview_observability.go` is only used by the old overview composer.

## Implementation

1. Delete the old whole-tab composers:
   - `writeOverview`
   - `writeAccounts`
   - `writeObservability`
   - `writeOverviewObservabilitySummary`
2. Remove the old tab body helpers from `layout.go`:
   - `activeTabBody`
   - `tabBody`
3. Remove tab-wide viewport state and helpers:
   - `scrollOffsets` from `Model`
   - `renderViewport`
   - `activeScrollMax`
   - `scrollMax`
   - `scrollActive`
   - `setActiveScroll`
4. Keep shared helpers that are still used by the pane renderer:
   - `splitBodyLines`
   - `viewWidth`
   - `viewHeight`
   - `viewportHeight`
   - `validActiveTab`
   - `clipPlainLine`
   - `maxInt`
5. Keep focused pane clamping as the only scroll clamp in `clampScrolls`.
6. Preserve `writeAPI`, `writeProviders`, `writeUsage`, and `writeLogs` only if
   they remain useful as documented section composers; otherwise remove them in
   this slice only if references prove they are dead.

## Verification

- Inspect the diff before checks.
- Run `rg` to prove removed helpers have no remaining references.
- Run `git diff --check`.
- Run `go test ./...` as compile/package check.
- Run `go vet ./...`.
- Build `cmd/ilonasin`.
- Start a temp daemon and smoke `ilonasin manage` in a PTY.

## Boundaries

- Do not touch `internal/server/*` dirty files.
- Do not change management DTOs, storage, provider adapters, or config.
- Do not add permanent tests.

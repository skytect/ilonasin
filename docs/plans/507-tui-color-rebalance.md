# 507 TUI Color Rebalance

## Context

Plan 501 restored stronger shared TUI colors and plan 503 forced the management
TUI to use an ANSI-256 color profile. A fresh smoke capture still shows the
shared chrome leaning heavily on muted color `60` for chips and pane borders,
which can make the interface feel washed out even though ANSI color is present.

The architecture says `ilonasin manage` is a first-class Bubble Tea/Lipgloss
management UI backed by daemon-owned management APIs. This slice should improve
visual clarity without changing data flow, storage, routing, provider behavior,
management DTOs, layout, or controls.

## Goal

Rebalance the shared TUI palette so the management UI has stronger color
separation and less washed-out chrome while preserving runtime behavior.

## Scope

1. Update `internal/tui/visual_styles.go` shared style colors only.
2. Reduce reliance on muted `60` for high-frequency chrome such as chips and
   pane borders.
3. Preserve a multi-hue operational palette for titles, labels, values, tabs,
   chips, badges, meters, panes, and focused states.
4. Preserve every rendered data field, keybinding hint, pane layout, wrapping
   helper, status rule, management client call, and action behavior.
5. Do not change management DTOs, storage, provider behavior, server behavior,
   routing, logging, config, or SQLite.
6. Do not add permanent tests.

## Out Of Scope

- New panes, copy, keyboard controls, or interaction changes.
- Per-pane accent changes outside the shared style palette.
- Changing the forced ANSI-256 color profile.
- Screenshots or terminal captures as committed artifacts.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, and keepalive disabled.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a pseudo-terminal.
5. Inspect non-committed TUI captures and confirm ANSI 256-color sequences are
   present with varied shared chrome colors.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- Shared TUI chrome uses a stronger, more varied 256-color palette.
- Existing layout, labels, wrapping, and management behavior are preserved.
- The change is limited to `internal/tui/visual_styles.go` and this plan.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/tui/visual_styles.go` shared styles only.
- Moved high-frequency chip and pane chrome off muted color `60`.
- Rebalanced shared title, label, value, tab, chip, badge, meter, pane, and
  focused-state colors across a stronger multi-hue ANSI-256 palette.
- Preserved all layout, pane rendering, keybinding, DTO, management API,
  storage, routing, provider, server, logging, config, and SQLite behavior.

## Verification Record

- Senior plan review: three reviewers reported no findings.
- `git diff --check`: passed.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal.
- Temporary TUI color capture: passed. Visible shared chrome used the new
  varied palette and no longer emitted old muted color `60` in the captured
  default API view.
- Senior implementation review: three reviewers reported no findings.
- Cleanup: temporary home, binary, config, terminal captures, marker files, and
  daemon process were removed.

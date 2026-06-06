# 501 TUI Color Restoration

## Context

After the Codex cyber observation slice, the next requested cleanup is to add
color back to the management TUI because the current interface looks washed
out. The highest-leverage shared chrome style surface is
`internal/tui/visual_styles.go`, with pane, chip, badge, title, and meter styles
reused across the management screens.

The architecture says `ilonasin manage` is a first-class Bubble Tea/Lipgloss
management UI backed by daemon-owned management APIs. This slice should improve
visual clarity without touching data flow, storage, routing, provider behavior,
or management DTOs.

The worktree still contains unrelated dirty Codex quota-pool response edits in
server files. Those are out of scope for this slice and must stay unstaged.

## Goal

Restore stronger, more legible shared TUI chrome color while preserving the
existing layout, copy, controls, and management behavior.

## Scope

1. Update the shared TUI visual palette in `internal/tui/visual_styles.go`.
2. Increase contrast and saturation for:
   - app title and focused pane title;
   - active tab and focused pane border;
   - chip backgrounds and foregrounds;
   - pane/card borders;
   - labels, values, and muted text;
   - success, warning, and error badges and meter bars.
3. Keep the palette operational and multi-hue, not dominated by one washed-out
   gray or one color family.
4. Preserve every rendered data field, keybinding hint, pane layout, wrapping
   helper, status rule, management client call, and action behavior.
5. Do not change management DTOs, storage, provider behavior, server behavior,
   routing, logging, config, or SQLite.
6. Do not add permanent tests.

## Out Of Scope

- New panes, copy, keyboard controls, or interaction changes.
- TUI data-model changes.
- Per-pane hardcoded accent tuning outside the shared style palette.
- Screenshots as committed artifacts.
- The existing dirty Codex quota-pool response edits.

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
5. Inspect a non-committed terminal capture from the bounded TUI run and confirm
   ANSI color escape output is present for chrome, tabs, badges, and meters.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- The TUI shared chrome palette has visibly stronger color and contrast.
- Existing pane layout, wrapping, labels, and status semantics are preserved.
- The change is limited to TUI visual style code and this plan.
- A temporary TUI capture confirms colored ANSI output is present.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/tui/visual_styles.go` shared styles only.
- Strengthened shared chrome, tabs, chips, pane borders, titles, badge, and
  meter colors using a multi-hue 256-color palette.
- Preserved all layout, pane rendering, keybinding, DTO, management API,
  storage, routing, provider, and server behavior.

## Verification Record

- Senior plan review: three subagents reported no findings after plan fixes for
  ANSI capture verification and shared-chrome scope.
- `git diff --check`: passed.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported
  no test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, and management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal.
- Temporary TUI capture: passed, with 626 SGR color sequences and expected
  shared chrome/status palette codes present.
- Cleanup: temporary home, binary, config, terminal captures, and daemon process
  were removed by the smoke script.

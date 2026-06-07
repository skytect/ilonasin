# 521 TUI Color Saturation

## Context

The structured-log redaction slice is committed. The user also noticed that
the management TUI still looks washed out and asked to add color back after the
plan.

Plans 501, 503, and 507 already restored shared palette color and forced an
ANSI-256 Lipgloss profile. A fresh inspection shows the remaining washed-out
surface is mostly shared chrome in `internal/tui/visual_styles.go` plus many
empty or low-data cards that still use the same pale accent color `110`.

`docs/ilonasin-architecture.md` treats `ilonasin manage` as the local control
plane. It should be visually polished, backed by daemon-owned management APIs,
and must not mutate `config.toml` or read/write SQLite directly.

## Goal

Increase visible TUI color saturation and separation while preserving layout,
data flow, controls, management API behavior, and all rendered information.

## Scope

1. Update shared TUI colors in `internal/tui/visual_styles.go`.
2. Replace repeated pale empty-state accent `110` with stronger multi-hue
   accents in the affected TUI pane renderers.
3. Preserve existing labels, layout, wrapping, status semantics, keybindings,
   management client calls, DTOs, storage, provider behavior, routing, logging,
   config, and SQLite behavior.
4. Do not add permanent tests.

## Out Of Scope

- New TUI panes, text, controls, or data fields.
- Management API, provider, routing, storage, or logging changes.
- Screenshots or terminal captures as committed artifacts.
- Changing the forced ANSI-256 color profile.

## Verification

Run:

```sh
git diff --check
git diff --no-index --check "$tmpempty" docs/plans/521-tui-color-saturation.md
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, keepalive disabled, and configured
   provider instances.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a pseudo-terminal.
5. Confirm the capture contains varied ANSI 256-color sequences from the new
   shared palette and empty-state accents.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- The TUI emits stronger, more varied shared chrome and empty-state colors.
- Existing layout, labels, wrapping, and management behavior are preserved.
- The change is limited to TUI visual color code and this plan.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Updated `internal/tui/visual_styles.go` shared styles only for stronger
  title, tab, chip, badge, meter, pane, label, and muted-text colors.
- Replaced repeated pale empty-state accent `110` with varied per-pane accents
  in the existing empty metric card call sites.
- Preserved all layout, text, wrapping, controls, management API calls, DTOs,
  storage, provider behavior, routing, logging, config, and SQLite behavior.

## Verification Record

- Senior plan review: two reviewers reported no findings; one reviewer found
  that the untracked plan file needed its own whitespace check. The plan was
  updated with `git diff --no-index --check "$tmpempty"
  docs/plans/521-tui-color-saturation.md`.
- `gofmt -w internal/tui/visual_styles.go internal/tui/api_local_tokens.go
  internal/tui/providers_instances.go internal/tui/providers_upstreams.go
  internal/tui/providers_oauth.go internal/tui/providers_fallback.go
  internal/tui/providers_model_cache.go internal/tui/usage_subscription.go`:
  passed.
- `git diff --check`: passed.
- `git diff --no-index --check "$tmpempty"
  docs/plans/521-tui-color-saturation.md`: passed for the new untracked plan
  file. Git returned status `1` only because the files differ, with no
  whitespace findings.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a
  pseudo-terminal. Both bounded runs exited by timeout with status `124` as
  expected.
- TUI color capture: passed. The 80-column capture contained 436 SGR sequences
  and the 140-column capture contained 658 SGR sequences. Captures included new
  palette colors such as `46`, `62`, `67`, `81`, `93`, `161`, and `219`.
- Senior implementation review: three reviewers reported no findings.
- Cleanup: temporary home, binary, config, terminal captures, and daemon
  process were removed.

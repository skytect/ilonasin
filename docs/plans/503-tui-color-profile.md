# 503 TUI Color Profile

## Context

Plan 501 strengthened the shared TUI palette, but a follow-up smoke during plan
502 found the management TUI can still render without ANSI color when the
environment contains `NO_COLOR=1`. Lipgloss uses termenv's environment-aware
profile detection, so `NO_COLOR` downgrades the default renderer to plain text
even though the TUI palette is defined.

The user explicitly requested color back in `ilonasin manage`. This slice
should make the management TUI render its own palette consistently without
changing layout, data flow, management APIs, provider behavior, storage, or
config.

## Goal

Ensure `ilonasin manage` uses the shared TUI color palette at runtime, including
in environments where `NO_COLOR` is set.

## Scope

1. Set the Lipgloss color profile for `ilonasin manage` startup to ANSI-256.
2. Keep the existing shared palette in `internal/tui/visual_styles.go`
   unchanged.
3. Keep TUI layout, controls, rendered fields, management client calls,
   storage, provider behavior, routing, and config unchanged.
4. Do not add permanent tests.

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
5. With `NO_COLOR=1` in the environment, capture `ilonasin manage` and confirm
   ANSI SGR color sequences are present.
6. Remove all temporary files and terminate the daemon.

## Acceptance

- `ilonasin manage` emits colored ANSI output for its shared palette even when
  `NO_COLOR=1` is set.
- Existing TUI layout and behavior are unchanged.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, ANSI color smoke, senior plan review,
  and senior implementation review pass.

## Implementation Record

- Set the Lipgloss default renderer color profile to ANSI-256 when
  `ilonasin manage` starts.
- Left the shared TUI palette, layout, controls, management APIs, server,
  provider behavior, storage, and config unchanged.
- Ran `go mod tidy`, which marks directly imported TUI dependencies as direct
  requirements.

## Verification Record

- Senior plan review: three reviewers reported no findings.
- `git diff --check`: passed.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported
  no test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, and management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal.
- `NO_COLOR=1` TUI color capture: passed with 17,594 ANSI SGR color sequences.
- Senior implementation review: three reviewers reported no findings.
- Cleanup: temporary home, binary, config, captures, and daemon process were
  removed.

# 479 AGENTS TUI SQLite Boundary

## Context

Plan 478 recorded a high-severity whole-codebase review finding: `AGENTS.md`
still says "The TUI may mutate SQLite", while the active architecture says
`ilonasin manage` is a client of the daemon-owned management API and must not
read or write SQLite directly.

`AGENTS.md` is active repository guidance, so this stale instruction should be
fixed before more implementation slices rely on it.

## Goal

Align active repository guidance with the architecture's daemon-owned SQLite
boundary for the management TUI.

## Scope

1. Update `AGENTS.md` only.
2. Replace the stale "TUI may mutate SQLite" guidance with guidance that TUI
   mutations must go through daemon-owned management APIs and must not mutate
   `config.toml`.
3. Do not rewrite historical plan files in this slice.
4. Do not change runtime behavior.
5. Do not add permanent tests.

## Implementation

1. Edit the `Coding Style` section in `AGENTS.md`.
2. Preserve the existing boundary guidance around local API auth, upstream
   provider credentials, provider adapters, routing, HTTP transport, TUI,
   config, and SQLite storage.
3. Keep the IO logging guidance unchanged.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run a direct CLI smoke by building a temporary `ilonasin` binary, starting
`ilonasin serve` with an isolated temporary home and config, checking the
management health and snapshot endpoints over the Unix socket, running bounded
`ilonasin manage` at several terminal widths, then cleaning up all temporary
files and processes.

## Acceptance

- `AGENTS.md` no longer says the TUI may mutate SQLite directly.
- `AGENTS.md` says TUI mutations must go through daemon-owned management APIs.
- `AGENTS.md` still says the TUI must not mutate `config.toml`.
- No runtime files are changed.

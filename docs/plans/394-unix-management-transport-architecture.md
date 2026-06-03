# 394 Unix Management Transport Architecture

## Context

`docs/ilonasin-architecture.md` still asks how daemon management transport should
work on non-Unix platforms. Current code and prior plans are consistently
Unix-domain-socket based:

- `internal/management/socket.go` prepares a secured Unix listener under the
  selected home runtime directory.
- `internal/management/http_client.go` exposes `NewUnixClient`.
- `internal/app/management.go` starts management HTTP only on that Unix listener.
- Production `manage` connects through the derived management socket path.
- Many prior management/TUI plans explicitly smoke the Unix socket boundary.

There is no implemented non-Unix transport, and choosing one now would be
speculative. The current architecture should say the implemented management
transport is Unix-only for now, while non-Unix support remains deferred research.

## Goal

Remove the stale non-Unix management transport open question by making the
current Unix-only management transport decision explicit.

## Scope

1. In the management API section of `docs/ilonasin-architecture.md`, state that
   the current implementation uses a Unix-domain HTTP socket under the selected
   home runtime directory.
2. State the boundary:
   - local-only;
   - separate from public compatibility routes;
   - socket path derived from selected home/config/database identity;
   - runtime directory/socket permissions are tightened where Unix permissions
     apply.
3. Keep non-Unix transport in Deferred Research.
4. Remove "How should daemon management transport work on non-Unix platforms?"
   from Open Questions.

## Out Of Scope

- Implementing Windows, TCP, named pipe, or launchd/systemd transport.
- Runtime code changes.
- Management route changes.
- TUI changes.
- Permanent tests.

## Verification

Run:

```sh
rg -n "Unix|non-Unix|management transport|management API|management socket|NewUnixClient|PrepareUnixListener" docs/ilonasin-architecture.md internal/management internal/app
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke even though this is
docs-only, to keep the slice discipline consistent.

## Acceptance

- Architecture clearly states the current Unix-domain management transport.
- Non-Unix transport remains deferred research, not an active current-design
  open question.
- No runtime behavior changes.

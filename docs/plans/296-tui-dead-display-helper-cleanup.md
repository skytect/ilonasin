# 296 TUI Dead Display Helper Cleanup

## Context

`docs/ilonasin-architecture.md` treats `ilonasin manage` as a first-class TUI,
not a debug panel. Recent TUI slices replaced older request-model and identity
rendering paths with pane-specific renderers:

- request logs now use `requestModelRoute` in `internal/tui/log_requests.go`;
- subscription and OAuth account renderers use their own identity helpers.

Two helpers now appear stale:

- `requestModelDisplay` in `internal/tui/display.go`;
- `highlightedIdentity` in `internal/tui/visual_identity.go`.

Dead rendering helpers make the TUI harder to reason about and preserve legacy
UI architecture that no longer corresponds to the current pane design.

## Scope

1. Prove the suspected helpers are unreferenced with source search.
2. Remove only dead TUI helper code and any now-unused imports.
3. Preserve all existing TUI rendering behavior, management DTOs, storage,
   provider behavior, server routes, config, logging, and privacy redaction.
4. Do not add permanent tests.

## Verification

Run:

```sh
rg -n 'requestModelDisplay|highlightedIdentity' internal/tui internal
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a direct serve/manage smoke:

```sh
# build a temporary ilonasin binary
# start ilonasin serve with ILONASIN_HOME and a temp config
# verify management health over the Unix socket
# run ilonasin manage under bounded narrow and wide PTY sessions
# assert api/providers/usage/logs render
```

## Acceptance

- The stale helpers are absent.
- No other TUI behavior changes are introduced.
- Compile, vet, source checks, serve smoke, and manage smoke pass.

# 269 TUI Provider And Log Density

## Goal

Make `ilonasin manage` easier to scan within the existing API, providers,
usage, and logs sections by tightening provider and log pane rendering. Keep the
current pane-local scroll model and avoid a new overview section.

## Context

- `docs/ilonasin-architecture.md` treats the TUI as a first-class management
  surface and keeps mutation behind the daemon-owned management API.
- `internal/tui/model.go` already defines the requested top-level sections:
  `api`, `providers`, `usage`, and `logs`.
- `internal/tui/panes.go` already provides per-pane scroll offsets and bounded
  pane rendering.
- Reviewers warned that pane IDs are action contracts, especially for provider
  key, OAuth, and fallback actions.

## Plan

1. Keep tab labels, pane IDs, pane order, key routing, and pane layout unchanged.
2. Tighten provider inventory rows so provider type/auth/capabilities remain
   readable while base URLs use horizontal room without dominating the pane.
3. Tighten upstream key, OAuth account, provider account, and fallback policy
   rows by reducing repeated labels and preserving visible safe account labels,
   including email-like display labels.
4. Tighten log request and fallback rows so metadata, IO policy, token mix, and
   performance read as compact visual rows instead of long text blocks.
5. Keep logs metadata-only. Do not add IO body rendering, direct SQLite access,
   config mutation, new management DTOs, or provider behavior changes.

## Files

- `internal/tui/providers_instances.go`
- `internal/tui/providers_upstreams.go`
- `internal/tui/providers_oauth.go`
- `internal/tui/providers_fallback.go`
- `internal/tui/log_requests.go`
- `internal/tui/log_fallbacks.go`
- shared visual helpers only if needed for renderer-local density

## Verification

1. `find . -name '*_test.go' -type f -print`
2. `git diff --check`
3. `go test ./...`
4. `go vet ./...`
5. Build `ilonasin`, start a disposable daemon, smoke management health and
   snapshot routes, then run `ilonasin manage` in a short PTY at a wide terminal.
6. Inspect plain TUI output for the four sections: `api`, `providers`, `usage`,
   `logs`.

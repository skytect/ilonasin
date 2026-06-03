# 403 App Keepalive Codex OAuth Capability

## Context

Recent slices centralized Codex OAuth capability checks in the credentials and
management boundaries. The app subscription keepalive path still has one local
inline predicate:

```go
instance.Type != "codex" || !instance.OAuth
```

`docs/ilonasin-architecture.md` keeps subscription account keepalive and quota
pooling as an auditable, provider-term-sensitive boundary. The keepalive app
DTO is intentionally separate from provider, credentials, and management DTOs,
so this slice should not import a management helper or widen cross-package
coupling. It should make the app boundary itself easier to audit.

## Goal

Centralize the app keepalive Codex OAuth capability predicate behind a local
helper while preserving runtime behavior exactly.

## Scope

1. Add a private helper near the keepalive provider DTO boundary, for example:
   `supportsCodexOAuthKeepalive(instance keepaliveProvider) bool`.
2. Use that helper in `keepaliveRunner.runDue`.
3. Preserve behavior exactly:
   - Codex OAuth provider instances are eligible for keepalive.
   - Codex API-key-only provider instances are skipped.
   - Non-Codex OAuth provider instances are skipped.
   - Non-Codex API-key-only provider instances are skipped.
4. Do not change keepalive scheduling, prompt, max token cap, usage refresh,
   logging, provider adapters, credential resolution, storage, management
   routes, TUI, config, public API routes, quota behavior, or metadata fields.

## Verification

Use temporary focused checks, then remove them before commit:

- `supportsCodexOAuthKeepalive` returns true only for Codex OAuth providers.
- `runDue` resolves bearers only for Codex OAuth providers.
- `runDue` still skips Codex API-key-only, non-Codex OAuth, and non-Codex
  API-key-only providers.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/app
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Keepalive Codex OAuth eligibility has one local app-boundary helper.
- Runtime keepalive behavior is unchanged.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

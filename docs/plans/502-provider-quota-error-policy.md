# 502 Provider Quota Error Policy

## Context

Plan 499 selected a follow-up for the dirty Codex quota-pool response work:
`internal/server/provider_policy.go` currently checks `instance.Type ==
"codex"` to decide whether an all-known-blocked 429 should be returned as a
Codex-shaped usage-limit error. The target architecture keeps
provider-specific behavior behind provider-owned policies, with server routes
consuming typed policy instead of switching on provider type.

Plan 501 already restored TUI color in `internal/tui/visual_styles.go` and
committed that slice. This slice should finish and clean up the remaining
dirty quota-pool response work without changing TUI code.

## Goal

Preserve the current Codex quota-pool exhausted client response behavior while
moving its provider selection out of server raw type checks and into
`provider.RoutePolicy`.

## Scope

1. Add a provider-owned, dependency-neutral route-policy field for writing the
   quota-pool usage-limit envelope.
2. Enable that policy only for Codex in `internal/provider/route_policy.go`.
3. Update `internal/server/provider_policy.go` to evaluate the new policy
   together with status `429` and error class `upstream_quota_pool_exhausted`.
4. Keep the existing Codex-shaped response writer and retry/reset headers
   behavior for non-stream chat, stream pre-response errors, and local
   Responses pre-stream errors.
5. Keep quota observation, credential selection, routing, storage, management
   APIs, TUI, config, provider adapters, and IO logging behavior unchanged.
6. Do not add permanent tests.

## Out Of Scope

- Changing which errors count as quota observations.
- Adding account switching, hidden failover, or provider rotation on 429.
- Broadening provider support for the Codex-shaped usage-limit envelope.
- TUI palette work, already handled by plan 501.

## Verification

Run:

```sh
rg -n 'instance.Type == "codex"|shouldWriteCodexQuotaPoolExhausted|case "codex"' internal/server/provider_policy.go internal/provider
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Use a temporary focused harness, then remove it before commit, to verify:

- Codex non-stream chat all-known-blocked 429 writes the usage-limit envelope.
- Codex stream pre-response all-known-blocked 429 writes the same envelope.
- Codex Responses pre-stream all-known-blocked 429 writes the same envelope.
- Retry/reset headers are present when retry-after is available.
- Non-Codex providers do not select the Codex-shaped envelope for the same
  status and error class.

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, and keepalive disabled.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a pseudo-terminal.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- Server policy helpers no longer raw-check `instance.Type == "codex"` for the
  quota-pool exhausted response.
- Codex remains the only provider whose route policy selects the
  quota-pool usage-limit envelope.
- Existing quota-pool exhausted behavior is preserved on chat, streaming chat,
  and Responses routes.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Added `provider.ErrorResponseRoutePolicy` with
  `WriteQuotaPoolUsageLimitEnvelope`.
- Enabled the usage-limit envelope policy only for Codex route policy.
- Updated server pre-response writers to consult provider route policy instead
  of checking `instance.Type == "codex"` directly.
- Preserved the Codex-shaped quota-pool exhausted envelope and retry/reset
  headers for non-stream chat, streaming pre-response errors, and local
  Responses pre-stream errors.

## Verification Record

- Senior plan review: two reviewers requested direct behavior coverage for the
  quota-pool envelope; plan verification was updated accordingly. One reviewer
  reported no findings.
- Temporary focused harness: passed for Codex non-stream chat, stream
  pre-response, local Responses pre-stream, retry/reset headers, and non-Codex
  generic behavior. Temporary harness was removed before commit.
- `rg -n 'instance.Type == "codex"|shouldWriteCodexQuotaPoolExhausted|case "codex"' internal/server/provider_policy.go internal/provider`:
  passed; no raw Codex quota envelope check remains in
  `internal/server/provider_policy.go`.
- `git diff --check`: passed.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported
  no test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, and management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal
  during the direct smoke.
- Senior implementation review: three reviewers reported no findings.
- Cleanup: temporary home, binary, config, harness, captures, and daemon process
  were removed.

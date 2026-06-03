# 345 Management Credential Kind Labels

## Context

`docs/ilonasin-architecture.md` treats the daemon management API as its own
boundary. Management DTOs should expose safe labels and operator metadata,
while credential storage and mutation internals remain in the credential
domain.

`internal/management/upstreams.go` currently publishes management credential
kind labels by aliasing credential-domain constants:

```go
const (
	CredentialKindAPIKey = credentials.CredentialKindAPIKey
	CredentialKindOAuth  = credentials.CredentialKindOAuth
)
```

The JSON vocabulary is already just `"api_key"` and `"oauth"`. The aliasing
keeps management's public helper surface coupled to credential internals even
though the management API can own those public labels directly.

## Scope

1. Keep this slice limited to `internal/management/upstreams.go` and this plan.
2. Replace the management credential-kind constant aliases with management-owned
   string constants:
   - `CredentialKindAPIKey = "api_key"`
   - `CredentialKindOAuth = "oauth"`
3. Preserve all existing behavior:
   - fallback-policy request/response JSON values remain unchanged;
   - `ProviderAllowsFallbackCredentialKind` returns the same values;
   - visible fallback filtering remains unchanged;
   - credential mutation still delegates to the credential domain;
   - TUI action filtering still uses management helpers.
4. Do not remove the legitimate `credentials` import from management in this
   slice, because the same file still converts credential metadata DTOs and
   calls credential-domain mutation interfaces.
5. Do not change routes, management socket transport, storage schema, provider
   logic, config, TUI rendering, logging, affinity, quota behavior, or permanent
   tests.

## Verification

Before implementation review:

1. Review the diff manually for scope and behavior.
2. Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/management ./internal/tui
go test ./...
go vet ./...
```

3. Build `ilonasin`, start `ilonasin serve` with an isolated temporary
   `ILONASIN_HOME`, verify the management health route over the Unix socket,
   run a short `ilonasin manage` TUI smoke, then terminate and clean up.

## Expected Outcome

- Management owns its public credential-kind labels instead of aliasing
  credential-domain constants.
- Public JSON values and TUI-visible fallback behavior are unchanged.
- No storage, route, provider, config, logging, affinity, quota, or TUI behavior
  changes are introduced.

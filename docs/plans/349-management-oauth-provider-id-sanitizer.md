# 349 Management OAuth Provider ID Sanitizer

## Context

`docs/ilonasin-architecture.md` treats provider instance IDs as configured
machine identifiers and management DTOs as safe daemon-owned projections. The
snapshot sanitizer consistently uses `safeMachineString` for provider instance
IDs across provider rows, credentials, accounts, model cache, usage, health,
fallback, and quota DTOs.

The OAuth mutation response conversion path in `internal/management/oauth.go`
still sanitizes `ProviderInstanceID` with `safeSnapshotString`:

```go
ProviderInstanceID: safeSnapshotString(row.ProviderInstanceID)
```

That is display-string sanitization, not machine-ID sanitization. It is also
inconsistent with upstream credential conversions in
`internal/management/upstreams.go`, which already use `safeMachineString` for
provider instance IDs.

## Scope

1. Keep this slice limited to:
   - `internal/management/oauth.go`;
   - this plan.
2. Change OAuth management DTO conversion for provider instance IDs to use
   `safeMachineString`:
   - `oauthChallengeFromCredentials`;
   - `oauthCredentialFromCredentials`.
3. Preserve all other OAuth DTO sanitization:
   - verification URL remains `safeBaseURL`;
   - user code remains `safeSnapshotString`;
   - handle remains `safeOAuthHandle`;
   - label, plan, scopes, account display, refresh failure class, and refresh
     failure description behavior remain unchanged.
4. Do not change OAuth route shapes, credential mutation behavior, storage
   schema, provider behavior, TUI rendering, config, logging, affinity, quota
   behavior, or permanent tests.

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

3. Run a temporary focused management smoke, removed before commit, that calls
   the two OAuth conversion helpers with provider IDs containing whitespace and
   unsafe-marker text such as `token`, and confirms the IDs are normalized with
   `safeMachineString` semantics rather than `safeSnapshotString` redaction,
   while other fields keep their existing sanitizers.
4. Build `ilonasin`, start `ilonasin serve` with an isolated temporary
   `ILONASIN_HOME`, verify the management health route over the Unix socket,
   run a short `ilonasin manage` TUI smoke, then terminate and clean up.

## Expected Outcome

- OAuth management action responses treat provider instance IDs as machine IDs.
- OAuth challenge and credential response shape is unchanged.
- Existing OAuth field sanitizers remain otherwise unchanged.

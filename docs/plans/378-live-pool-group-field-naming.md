# 378 Live Pool Group Field Naming

## Context

The architecture describes provider credential grouping as credential pool
groups. Plans 370 through 374 removed the old fallback-policy management alias,
but live credential records still expose and pass a field named
`FallbackGroup` / `fallback_group`.

Current serving no longer uses that field to decide eligibility. Credential
selection is same provider instance plus same provider model, with quota and
pressure handling. The remaining group value is operator/display metadata.

SQLite still has a historical `provider_credentials.fallback_group` column. A
schema migration can rename that later. This slice keeps the legacy name
contained at the SQLite boundary and uses pool-group naming in live domain,
management, and TUI code.

## Scope

1. Rename live credential domain fields:
   - `DefaultFallbackGroup` to `DefaultPoolGroup`;
   - `NewUpstreamCredential.FallbackGroup` to `PoolGroup`;
   - `UpstreamCredentialMetadata.FallbackGroup` to `PoolGroup`;
   - `ResolvedAPIKeyCredential.FallbackGroup` to `PoolGroup`;
   - `ResolvedOAuthBearerCredential.FallbackGroup` to `PoolGroup`.
2. Rename the management upstream credential field from
   `FallbackGroup json:"fallback_group"` to `PoolGroup json:"pool_group"`.
   This intentionally changes the local management wire shape for both
   snapshots and add-credential responses, matching the recent removal of
   fallback-policy compatibility aliases.
3. Update management sanitization, conversion, and TUI upstream credential
   rendering to use `PoolGroup`.
4. Keep `CredentialPoolGroup.GroupLabel` unchanged.
5. Keep SQLite SQL column names as `fallback_group` in this slice, but map them
   immediately into pool-group fields.
6. Keep historical migrations unchanged except for no edits.
7. Do not change serving credential selection, provider adapters, quota
   handling, request/fallback metadata, subscription usage, config, logging, or
   TUI layout.
8. Verify `provider.BearerCredential` has no group field. If that changes
   during implementation, keep provider-domain naming aligned with `PoolGroup`
   instead of leaving a live `FallbackGroup` field outside SQLite.

## Verification

Review the diff before checks for:

- no live source references to `FallbackGroup` or `DefaultFallbackGroup`;
- `fallback_group` remains only in SQLite SQL/migrations and older docs/plans;
- management snapshots and add-credential responses expose upstream credentials
  with `pool_group`, not `fallback_group`;
- serving credential selection remains unchanged.

Then run:

```sh
rg -n "FallbackGroup|DefaultFallbackGroup" internal
rg -n "fallback_group" internal
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/credentials ./internal/storage/sqlite ./internal/management ./internal/tui
go test ./...
go vet ./...
```

The `fallback_group` search should only find SQLite SQL/migration references
under `internal/storage/sqlite`.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, verifying the management snapshot and add-credential response do
not emit upstream credential `fallback_group`, running bounded
`ilonasin manage` at narrow and wide terminal widths, and cleaning up all
temporary files and processes.

## Acceptance

- Live domain, management, and TUI code use pool-group terminology for upstream
  credential grouping.
- SQLite is the only remaining live internal boundary that mentions the legacy
  `fallback_group` column.
- Upstream credential management JSON uses `pool_group`.
- Serving behavior and metadata recording are unchanged.

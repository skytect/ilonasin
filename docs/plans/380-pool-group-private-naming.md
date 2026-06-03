# 380 Pool Group Private Naming

## Context

Recent slices moved live credential grouping from fallback terminology to pool
groups across management JSON and SQLite schema. Architecture audits still found
two live private naming leftovers:

- management helper names `fallbackCredentialKinds` and
  `allowedFallbackCredentialKindsByProvider`;
- SQLite OAuth resolution local field `fallback` carrying `pool_group`.

Management also duplicates credential-kind string constants that already exist
in `internal/credentials`.

## Scope

1. Rename management private helpers to pool-group terminology:
   - `fallbackCredentialKinds` to `poolGroupCredentialKinds`;
   - `allowedFallbackCredentialKindsByProvider` to
     `allowedPoolGroupCredentialKindsByProvider`.
2. Replace management duplicate credential-kind constants with
   `credentials.CredentialKindAPIKey` and `credentials.CredentialKindOAuth`.
3. Rename `oauthBearerRow.fallback` to `poolGroup` and update its scan/copy
   sites.
4. Keep fallback event logging and fallback reason terminology unchanged. Those
   describe actual retry/fallback events, not credential pool group metadata.
5. Keep SQLite migration historical references unchanged.
6. Do not change JSON, SQLite schema, serving behavior, selection policy,
   management visibility rules, TUI layout, provider adapters, logging, or
   request metadata.

## Verification

Run:

```sh
rg -n "fallbackCredentialKinds|allowedFallbackCredentialKindsByProvider|\\.fallback\\b|\\bfallback\\b" internal/management internal/storage/sqlite
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management ./internal/storage/sqlite
go test ./...
go vet ./...
```

Review any remaining `fallback` search hits and confirm they refer only to
actual fallback events, historical migrations, or unrelated time parser
fallbacks.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Credential pool group helper names no longer use fallback terminology.
- Management uses the shared credential-kind constants from
  `internal/credentials`.
- SQLite OAuth resolution local names reflect `pool_group`.
- Behavior and wire/storage shapes are unchanged.

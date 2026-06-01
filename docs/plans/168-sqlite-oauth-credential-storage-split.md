# 168 SQLite OAuth Credential Storage Split

## Context

The target architecture separates local client tokens, upstream provider
credentials, OAuth account state, routing metadata, and daemon-owned SQLite
mutation boundaries.

Recent slices split SQLite telemetry, model cache, subscription usage, active
quota blocks, local client tokens, and API-key upstream credential storage into
focused same-package files. `internal/storage/sqlite/db.go` now still owns core
store setup and migrations, plus a large OAuth credential/account/token cluster
and fallback policy storage.

OAuth storage is a coherent boundary because it owns:

- OAuth credential insert/upsert behavior,
- provider account matching and metadata,
- OAuth access and refresh token secret references,
- OAuth token refresh updates,
- bearer and refresh-token resolution,
- refresh failure metadata.

Moving that cluster into a dedicated file keeps API-key credentials, OAuth
credentials, local client tokens, fallback policy state, and core SQLite setup
separate without changing public behavior.

## Goal

Move SQLite OAuth credential, account, and token storage out of `db.go` into
`internal/storage/sqlite/oauth_credentials.go` without changing behavior.

After this slice:

- `oauth_credentials.go` owns OAuth credential persistence, provider account
  metadata listing, OAuth bearer resolution, OAuth refresh credential
  resolution, token updates, and refresh failure persistence.
- `api_key_credentials.go` owns API-key upstream credential storage.
- `local_tokens.go` owns ilonasin local client token storage.
- `db.go` keeps core store setup, migrations, shared helpers, and fallback
  policy storage for a later slice.

## Scope

1. Add `internal/storage/sqlite/oauth_credentials.go`.
2. Move these exported store methods from `db.go`:
   - `UpsertOAuthCredentialForAccountHash`
   - `InsertOAuthCredential`
   - `ListOAuthCredentials`
   - `ListProviderAccounts`
   - `MarkOAuthRefreshFailure`
   - `ResolveOAuthBearerCredential`
   - `ResolveOAuthBearerCredentials`
   - `ResolveOAuthBearerCredentialByID`
   - `ResolveOAuthRefreshCredential`
   - `ResolveOAuthRefreshCredentialForProvider`
   - `ResolveOAuthRefreshToken`
   - `UpdateOAuthTokens`
3. Move the private OAuth helper types and functions used only by those
   methods:
   - `upsertOAuthCredentialForAccountHash`
   - `providerAccountMatch`
   - `findProviderAccountForUpdate`
   - `insertOAuthCredentialTx`
   - `upsertExistingOAuthCredentialTx`
   - `existingOAuthCredentialForAccount`
   - `insertOAuthCredentialWithoutAccountTx`
   - `upsertCredentialSecret`
   - `upsertOAuthTokenRow`
   - `updateProviderAccountTx`
   - `insertCredentialSecret`
   - `oauthBearerRow`
   - `materializeOAuthBearer`
   - `updateCredentialSecret`
4. Keep shared helpers such as `isUniqueConstraint`, `nullableTime`,
   `parseSQLiteTime`, and `cloneTime` in `db.go` at package scope.
5. Include only imports required by the moved OAuth code.
6. Do not change SQL text, transaction boundaries, duplicate handling, account
   hash matching, secret storage location, list redaction behavior, bearer
   routing claim derivation, resolver ordering, refresh failure behavior,
   metadata DTOs, management APIs, TUI rendering, provider behavior, config,
   migrations, or tests.

## Out of Scope

- Fallback policy storage moves.
- Shared SQLite helper refactors.
- Credential secret abstraction changes.
- OAuth service behavior changes.
- OAuth login, device flow, token exchange, or provider HTTP changes.
- Schema changes.
- Management route changes.
- TUI changes.
- Permanent tests.
- Broader storage refactors.

## Implementation Steps

1. Create `internal/storage/sqlite/oauth_credentials.go` with `package sqlite`.
2. Move the OAuth methods, helper types, and helper functions listed above from
   `db.go`.
3. Remove now-unused imports from `db.go`.
4. Run `gofmt` on touched Go files.
5. Review the diff before running checks, with special attention to:
   - OAuth token material still stored only in `credential_secrets`,
   - list paths not selecting secret material,
   - full account IDs not being stored or rendered,
   - access-token routing claims still derived transiently only during bearer
     resolution,
   - refresh failure descriptions still limited to the existing safe storage
     path.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
pid=""
cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
cfg="$tmp/config.toml"
cat >"$cfg" <<'EOF'
[server]
bind = "127.0.0.1:0"
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" >"$tmp/serve.log" 2>&1 &
pid="$!"
for _ in $(seq 1 80); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null; then
    break
  fi
  sleep 0.1
done
if [ -z "${sock:-}" ]; then
  echo "management socket not found"
  cat "$tmp/serve.log"
  exit 1
fi
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null
set +e
printf '\tq' | timeout 3s script -q -e -c \
  "env ILONASIN_HOME='$tmp/home' '$tmpbin/ilonasin' manage --config '$cfg'" \
  "$tmp/manage.typescript" >/dev/null
manage_status="$?"
set -e
if [ "$manage_status" -ne 0 ] && [ "$manage_status" -ne 124 ]; then
  cat "$tmp/manage.typescript" 2>/dev/null || true
  exit "$manage_status"
fi
git diff --check
oauth_symbols="UpsertOAuthCredentialForAccountHash|upsertOAuthCredentialForAccountHash|InsertOAuthCredential|providerAccountMatch|findProviderAccountForUpdate|insertOAuthCredentialTx|upsertExistingOAuthCredentialTx|existingOAuthCredentialForAccount|insertOAuthCredentialWithoutAccountTx|upsertCredentialSecret|upsertOAuthTokenRow|updateProviderAccountTx|insertCredentialSecret|ListOAuthCredentials|ListProviderAccounts|MarkOAuthRefreshFailure|ResolveOAuthBearerCredential|ResolveOAuthBearerCredentials|oauthBearerRow|materializeOAuthBearer|ResolveOAuthBearerCredentialByID|ResolveOAuthRefreshCredential|ResolveOAuthRefreshCredentialForProvider|ResolveOAuthRefreshToken|UpdateOAuthTokens|updateCredentialSecret"
rg -n "$oauth_symbols" internal/storage/sqlite/oauth_credentials.go
if rg -n "$oauth_symbols" internal/storage/sqlite/db.go; then
  echo "oauth credential storage remains in db.go"
  exit 1
fi
```

## Acceptance

- OAuth credential, account, and token storage methods compile from
  `oauth_credentials.go`.
- `db.go` no longer owns OAuth storage methods, OAuth-only helper types, or
  OAuth-only helper functions.
- OAuth token material is still inserted into, updated in, and resolved from
  `credential_secrets`.
- List paths still return safe metadata and do not select secret material.
- Full upstream account IDs remain transient routing claims only.
- Duplicate, disabled, expired, missing-secret, and no-eligible-credential
  behavior remains unchanged.
- Public storage interfaces and behavior remain unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

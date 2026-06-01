# 167 SQLite API Key Credential Storage Split

## Context

The target architecture separates local client tokens from upstream provider
credentials. Plan 166 moved local client token storage into its own file. The
remaining `internal/storage/sqlite/db.go` storage methods now mostly cover
upstream API-key credentials, OAuth credentials/accounts/tokens, fallback policy
state, and core store setup.

The API-key credential methods are a coherent storage cluster:

- `InsertAPIKeyCredential`
- `ListUpstreamCredentials`
- `DisableUpstreamCredential`
- `ResolveAPIKeyCredential`
- `ResolveAPIKeyCredentials`
- `scanUpstreamCredentialMetadata`

Moving them into a focused same-package file keeps API-key storage separate
from OAuth state and makes the upstream credential boundary clearer without
changing auth, routing, management, or TUI behavior.

## Goal

Move SQLite API-key upstream credential storage out of `db.go` into
`internal/storage/sqlite/api_key_credentials.go` without changing behavior.

After this slice:

- `api_key_credentials.go` owns API-key credential persistence, listing,
  disabling, resolving, and metadata scanning.
- `local_tokens.go` owns local client tokens.
- `db.go` keeps core store setup, migrations, OAuth storage, fallback policy
  storage, and shared helpers for later slices.
- The shared row scanner interface has a neutral name instead of
  `localTokenScanner`.

## Scope

1. Add `internal/storage/sqlite/api_key_credentials.go`.
2. Move these methods/helpers intact from `db.go`:
   - `InsertAPIKeyCredential`
   - `ListUpstreamCredentials`
   - `DisableUpstreamCredential`
   - `ResolveAPIKeyCredential`
   - `ResolveAPIKeyCredentials`
   - `scanUpstreamCredentialMetadata`
3. Rename the shared `localTokenScanner` interface to `rowScanner` because it
   is used by both local token and upstream credential scanners.
4. Include only imports required by the moved code.
5. Keep shared helpers such as `parseSQLiteTime` and `isUniqueConstraint` at
   package scope.
6. Do not change SQL text, transaction boundaries, duplicate handling, secret
   storage location, list redaction behavior, disabled credential behavior,
   resolver ordering, returned metadata, management DTOs, TUI rendering,
   routing, provider behavior, config, migrations, or tests.

## Out of Scope

- OAuth credential/account/token storage moves.
- Credential secret helper refactors.
- Fallback policy storage moves.
- Local token behavior changes.
- Upstream management API changes.
- Routing or provider behavior changes.
- SQLite schema changes.
- Permanent tests.
- Broader storage refactors.

## Implementation Steps

1. Create `internal/storage/sqlite/api_key_credentials.go` with
   `package sqlite`.
2. Move the five API-key storage methods plus
   `scanUpstreamCredentialMetadata` from `db.go`.
3. Rename `localTokenScanner` to `rowScanner` and update
   `scanLocalTokenMetadata` plus `scanUpstreamCredentialMetadata`.
4. Add imports for `context`, `database/sql`, `errors`, `time`, and
   `ilonasin/internal/credentials` as required.
5. Remove any now-unused imports from `db.go`.
6. Run `gofmt` on touched Go files.
7. Review the diff before running checks, with special attention to SQL text,
   API-key secret material staying only in `credential_secrets`, and list paths
   not selecting secret material.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
cleanup() {
  if [ -n "${pid:-}" ]; then
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
rg -n "InsertAPIKeyCredential|ListUpstreamCredentials|DisableUpstreamCredential|ResolveAPIKeyCredential|ResolveAPIKeyCredentials|scanUpstreamCredentialMetadata" internal/storage/sqlite/api_key_credentials.go
if rg -n "InsertAPIKeyCredential|ListUpstreamCredentials|DisableUpstreamCredential|ResolveAPIKeyCredential|ResolveAPIKeyCredentials|scanUpstreamCredentialMetadata" internal/storage/sqlite/db.go; then
  echo "api key credential storage remains in db.go"
  exit 1
fi
```

## Acceptance

- API-key upstream credential storage methods compile from
  `api_key_credentials.go`.
- `db.go` no longer owns API-key storage methods.
- API-key secret material is still inserted into and resolved only from
  `credential_secrets`.
- List paths still return redacted metadata and do not select secret material.
- Duplicate, disabled, and no-eligible-credential behavior remains unchanged.
- Public storage interfaces and behavior remain unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

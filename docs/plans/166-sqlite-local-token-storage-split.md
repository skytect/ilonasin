# 166 SQLite Local Token Storage Split

## Context

The target architecture separates ilonasin client tokens from upstream provider
credentials. Local client tokens live in SQLite, are one-way hashed before
storage, and must never be confused with provider API keys or OAuth material.

Recent storage slices split non-credential telemetry and cache concerns out of
`internal/storage/sqlite/db.go`. The file now primarily owns core store setup,
credential storage, and fallback policy storage. The local client token methods
are a coherent storage cluster:

- `InsertLocalToken`
- `ListLocalTokens`
- `DisableLocalToken`
- `FindLocalTokenByHash`
- `scanLocalTokenMetadata`

Moving them into a focused same-package file makes the local-token boundary
explicit without changing authentication, management routes, TUI behavior, or
storage semantics.

## Goal

Move SQLite local client token storage out of `db.go` into
`internal/storage/sqlite/local_tokens.go` without changing behavior.

After this slice:

- `local_tokens.go` owns local client token persistence and scanning.
- `db.go` keeps store setup, migrations, upstream credentials, OAuth storage,
  fallback policy storage, and shared helpers for later slices.
- The local-token repository interface remains unchanged.

## Scope

1. Add `internal/storage/sqlite/local_tokens.go`.
2. Move these methods/helpers intact from `db.go`:
   - `InsertLocalToken`
   - `ListLocalTokens`
   - `DisableLocalToken`
   - `FindLocalTokenByHash`
   - `scanLocalTokenMetadata`
3. Keep the shared `localTokenScanner` interface in `db.go` for now because
   upstream credential scanning still uses it.
4. Include only imports required by the moved code.
5. Keep shared helpers such as `parseSQLiteTime` at package scope.
6. Do not change SQL text, inserted fields, returned metadata, disabled
   semantics, token lookup behavior, error messages, auth behavior, management
   DTOs, TUI rendering, config, migrations, or tests.

## Out of Scope

- Local token format or hashing changes.
- Local token management route changes.
- Auth middleware changes.
- TUI changes.
- Upstream provider credential or OAuth storage moves.
- Fallback policy storage moves.
- SQLite schema changes.
- Permanent tests.
- Broader storage refactors.

## Implementation Steps

1. Create `internal/storage/sqlite/local_tokens.go` with `package sqlite`.
2. Move the four local-token storage methods and `scanLocalTokenMetadata` from
   `db.go` into the new file.
3. Add imports for `context`, `database/sql`, `errors`, `fmt`, `time`, and
   `ilonasin/internal/credentials` as required.
4. Remove any now-unused imports from `db.go`.
5. Run `gofmt` on touched Go files.
6. Review the diff before running checks, with special attention to SQL text,
   `client token not found` error behavior, and disabled timestamp parsing.

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
rg -n "InsertLocalToken|ListLocalTokens|DisableLocalToken|FindLocalTokenByHash|scanLocalTokenMetadata" internal/storage/sqlite/local_tokens.go
if rg -n "InsertLocalToken|ListLocalTokens|DisableLocalToken|FindLocalTokenByHash|scanLocalTokenMetadata" internal/storage/sqlite/db.go; then
  echo "local token storage remains in db.go"
  exit 1
fi
```

## Acceptance

- Local token storage methods compile from `local_tokens.go`.
- `db.go` no longer owns local token storage methods.
- SQL, metadata mapping, disabled timestamp behavior, and token lookup error
  behavior are unchanged.
- Public storage interfaces and behavior remain unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

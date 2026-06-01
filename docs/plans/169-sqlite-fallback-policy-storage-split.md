# 169 SQLite Fallback Policy Storage Split

## Context

The target architecture keeps credential storage, routing/fallback metadata,
observability metadata, and core SQLite lifecycle concerns as separate
boundaries. Recent slices split SQLite telemetry, model cache, subscription
usage, active quota blocks, local client tokens, API-key credentials, and OAuth
credentials into focused same-package files.

`internal/storage/sqlite/db.go` now owns only:

- core store lifecycle and migrations,
- fallback policy storage,
- shared SQLite helpers.

Fallback policy storage is a coherent boundary because it owns the persisted
enablement state for credential fallback groups and exposes the current fallback
groups derived from enabled API-key and OAuth credentials. Moving it out leaves
`db.go` focused on SQLite core setup plus shared helpers.

## Goal

Move SQLite fallback policy storage out of `db.go` into
`internal/storage/sqlite/fallback_policies.go` without changing behavior.

After this slice:

- `fallback_policies.go` owns fallback policy listing, mutation, and lookup.
- `api_key_credentials.go` owns API-key upstream credential storage.
- `oauth_credentials.go` owns OAuth credential/account/token storage.
- `local_tokens.go` owns ilonasin local client token storage.
- `db.go` owns core store setup, migrations, and shared helpers.

## Scope

1. Add `internal/storage/sqlite/fallback_policies.go`.
2. Move these methods intact from `db.go`:
   - `ListFallbackPolicies`
   - `SetFallbackGroupEnabled`
   - `fallbackGroupEnabled`
3. Include only imports required by the moved code.
4. Keep shared helpers such as `boolToInt`, `parseSQLiteTime`, `nullableTime`,
   and `isUniqueConstraint` in `db.go` at package scope.
5. Do not change SQL text, sorting, explicit/default policy semantics,
   fallback group eligibility, mutation behavior, management DTOs, management
   routes, TUI behavior, routing behavior, provider behavior, config,
   migrations, or tests.

## Out of Scope

- Fallback routing policy changes.
- Credential fallback semantics.
- TUI fallback controls.
- Management route changes.
- Schema changes.
- Shared helper refactors.
- Moving migrations or core SQLite lifecycle code.
- Permanent tests.
- Broader storage refactors.

## Implementation Steps

1. Create `internal/storage/sqlite/fallback_policies.go` with `package sqlite`.
2. Move `ListFallbackPolicies`, `SetFallbackGroupEnabled`, and
   `fallbackGroupEnabled` from `db.go`.
3. Remove now-unused imports from `db.go`.
4. Run `gofmt` on touched Go files.
5. Review the diff before running checks, with special attention to:
   - the list query still deriving groups from enabled `api_key` and `oauth`
     credentials only,
   - explicit policy overlay semantics staying unchanged,
   - mutation still using the same `ON CONFLICT` upsert,
   - missing policies still defaulting to disabled.

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
fallback_symbols="ListFallbackPolicies|SetFallbackGroupEnabled|fallbackGroupEnabled"
rg -n "$fallback_symbols" internal/storage/sqlite/fallback_policies.go
if rg -n "$fallback_symbols" internal/storage/sqlite/db.go; then
  echo "fallback policy storage remains in db.go"
  exit 1
fi
```

## Acceptance

- Fallback policy storage compiles from `fallback_policies.go`.
- `db.go` no longer owns fallback policy storage methods.
- `db.go` remains core store setup, migrations, scanner interface, and shared
  SQLite helpers.
- List semantics, mutation semantics, missing-policy defaults, and ordering
  remain unchanged.
- Public storage interfaces and behavior remain unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

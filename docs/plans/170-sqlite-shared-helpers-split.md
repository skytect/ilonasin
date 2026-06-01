# 170 SQLite Shared Helpers Split

## Context

The target architecture separates SQLite core lifecycle, credential storage,
fallback policy storage, telemetry storage, and model/subscription state into
focused boundaries. Recent slices moved local tokens, API-key credentials,
OAuth credentials, fallback policies, telemetry summaries, request metadata,
pruning, active quota blocks, subscription usage, model cache, and event
recording into separate same-package files.

`internal/storage/sqlite/db.go` now contains core store lifecycle and shared
helper functions used by those split files. Keeping broad shared helpers in the
core lifecycle file makes `db.go` less focused than it needs to be.

The remaining shared helper cluster is coherent:

- `rowScanner`
- `isUniqueConstraint`
- `nullableInt`
- `nullableInt64`
- `boolToInt`
- `tokenRate`
- `cacheMissTokens`
- `nullableTime`
- `parseSQLiteTime`
- `cloneTime`

Moving these helpers to a dedicated same-package file leaves `db.go` focused on
opening, closing, and migrating the store.

## Goal

Move SQLite package shared helpers out of `db.go` into
`internal/storage/sqlite/helpers.go` without changing behavior.

After this slice:

- `helpers.go` owns shared scanner, nullable conversion, time parsing, rate,
  cache, and uniqueness helpers.
- `db.go` owns `Store`, `Open`, `Close`, `Migrate`, and `migrationApplied`.
- Split storage files continue to use the same package-private helper names.

## Scope

1. Add `internal/storage/sqlite/helpers.go`.
2. Move these declarations intact from `db.go`:
   - `rowScanner`
   - `isUniqueConstraint`
   - `nullableInt`
   - `nullableInt64`
   - `boolToInt`
   - `tokenRate`
   - `cacheMissTokens`
   - `nullableTime`
   - `parseSQLiteTime`
   - `cloneTime`
3. Include only imports required by the moved helper code.
4. Remove now-unused imports from `db.go`.
5. Do not change helper behavior, names, signatures, call sites, SQL, storage
   behavior, management APIs, TUI behavior, provider behavior, config,
   migrations, or tests.

## Out of Scope

- Helper behavior refactors.
- Renaming helpers.
- Moving migrations.
- Moving core store lifecycle methods.
- Schema changes.
- Storage behavior changes.
- Permanent tests.
- Broader package layout changes.

## Implementation Steps

1. Create `internal/storage/sqlite/helpers.go` with `package sqlite`.
2. Move the shared helper declarations listed above from `db.go`.
3. Run `gofmt` on touched Go files.
4. Review the diff before running checks, with special attention to:
   - `parseSQLiteTime` retaining the legacy fallback format,
   - `nullableTime` still writing UTC RFC3339Nano strings,
   - `isUniqueConstraint` still matching the existing SQLite error text,
   - token/cache rate helpers preserving zero and negative guards.

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
helper_symbols="rowScanner|isUniqueConstraint|nullableInt\\(|nullableInt64|boolToInt|tokenRate|cacheMissTokens|nullableTime|parseSQLiteTime|cloneTime"
rg -n "$helper_symbols" internal/storage/sqlite/helpers.go
if rg -n "$helper_symbols" internal/storage/sqlite/db.go; then
  echo "sqlite shared helpers remain in db.go"
  exit 1
fi
```

## Acceptance

- Shared helper declarations compile from `helpers.go`.
- `db.go` no longer owns shared helper declarations.
- `db.go` remains focused on `Store`, `Open`, `Close`, `Migrate`, and
  `migrationApplied`.
- Helper behavior and call sites remain unchanged.
- Public storage interfaces and behavior remain unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

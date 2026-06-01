# 160 SQLite Model Cache Split

## Context

The target architecture keeps provider model discovery, sanitized model
metadata, SQLite storage, management snapshots, and TUI rendering as separate
boundaries. Plans 006 and 058 established provider-backed model discovery and
model cache behavior, while later TUI slices split model-cache rendering.

`internal/storage/sqlite/db.go` still owns the model cache persistence methods:

- `ReplaceModelCache`
- `ListModelCache`

These methods are self-contained around the `model_cache` table. Moving them
into a focused storage file continues reducing the monolithic SQLite store
without changing SQL, cache semantics, provider behavior, management behavior,
or privacy policy.

## Goal

Move model cache storage methods out of `db.go` into a focused same-package file
without changing behavior.

After this slice:

- `internal/storage/sqlite/model_cache.go` owns model cache persistence and
  listing.
- `db.go` no longer contains model cache methods.
- The `Store` interface surface remains unchanged for callers.

## Scope

1. Add `internal/storage/sqlite/model_cache.go`.
2. Move `ReplaceModelCache` from `db.go` unchanged.
3. Move `ListModelCache` from `db.go` unchanged.
4. Add only the imports needed by moved code.
5. Remove any now-unused imports from `db.go`.
6. Do not change SQL text, transaction behavior, replacement semantics,
   ordering, scan behavior, timestamp handling, nullable handling, migrations,
   or table shape.
7. Do not change provider model discovery, routing, model resolution,
   management DTOs, TUI rendering, config, auth, or request metadata.
8. Do not add permanent tests.
9. Do not push.

## Non-Goals

- No SQLite schema changes.
- No query optimization.
- No migration split.
- No provider model discovery changes.
- No model cache response shape changes.
- No management route or TUI changes.
- No broader storage refactor in this slice.

## Implementation

1. Create `internal/storage/sqlite/model_cache.go` with `package sqlite`.
2. Move the two model cache methods unchanged from `db.go`.
3. Keep dependencies on existing helpers such as `nullableInt` and
   `parseSQLiteTime`.
4. Run `gofmt`.
5. Review the diff to confirm this is relocation only plus import cleanup.
6. Verify `rg -n "ReplaceModelCache|ListModelCache|model_cache"
   internal/storage/sqlite` shows the methods in the new file and migrations in
   `migrations.go`.

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
rg -n "ReplaceModelCache|ListModelCache" internal/storage/sqlite/model_cache.go
if rg -n "ReplaceModelCache|ListModelCache" internal/storage/sqlite/db.go; then
  echo "model cache storage methods remain in db.go"
  exit 1
fi
git diff --check
```

## Acceptance

- Model cache storage methods compile from the new file.
- SQL, transaction behavior, nullable handling, ordering, timestamp handling,
  and scan behavior are unchanged.
- `db.go` is smaller and no longer owns model cache storage methods.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

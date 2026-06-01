# 159 SQLite Subscription Usage Split

## Context

The target architecture keeps SQLite storage as a separate boundary from
management, provider adapters, routing, config, and TUI. Recent subscription
usage work already split management DTOs, sanitization, response aggregation,
keepalive reporting, and provider-window extraction into focused files.

`internal/storage/sqlite/db.go` remains the largest file in the codebase and
still owns all storage domains. The subscription usage methods are self-contained
around the `subscription_usage_snapshots` table:

- `UpsertSubscriptionUsageSnapshot`
- `LatestSubscriptionUsageSnapshots`

Moving those methods into a focused storage file makes the SQLite boundary more
modular without changing SQL, migrations, management behavior, provider
behavior, or privacy policy.

## Goal

Move subscription usage storage methods out of `db.go` into a focused
same-package file without changing behavior.

After this slice:

- `internal/storage/sqlite/subscription_usage.go` owns subscription usage
  snapshot persistence and query methods.
- `db.go` no longer contains subscription usage snapshot methods.
- The `Store` interface surface remains unchanged for callers.

## Scope

1. Add `internal/storage/sqlite/subscription_usage.go`.
2. Move `UpsertSubscriptionUsageSnapshot` from `db.go` unchanged.
3. Move `LatestSubscriptionUsageSnapshots` from `db.go` unchanged.
4. Add only the imports needed by moved code.
5. Remove any now-unused imports from `db.go`.
6. Do not change SQL text, ordering, scan behavior, logging attributes,
   timestamp handling, nullable handling, migrations, or table shape.
7. Do not change management DTOs, provider usage calls, TUI rendering, config,
   routing, auth, or request metadata.
8. Do not add permanent tests.
9. Do not push.

## Non-Goals

- No SQLite schema changes.
- No query optimization.
- No migration split.
- No management route changes.
- No subscription usage response shape changes.
- No keepalive behavior changes.
- No broader storage refactor in this slice.

## Implementation

1. Create `internal/storage/sqlite/subscription_usage.go` with `package sqlite`.
2. Move the two subscription usage methods unchanged from `db.go`.
3. Keep dependencies on existing helpers such as `nullableInt64`,
   `nullableTime`, `boolToInt`, and `parseSQLiteTime`.
4. Run `gofmt`.
5. Review the diff to confirm this is relocation only plus import cleanup.
6. Verify `rg -n "SubscriptionUsageSnapshot|subscription_usage_snapshots"
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
rg -n "UpsertSubscriptionUsageSnapshot|LatestSubscriptionUsageSnapshots" internal/storage/sqlite/subscription_usage.go
if rg -n "UpsertSubscriptionUsageSnapshot|LatestSubscriptionUsageSnapshots" internal/storage/sqlite/db.go; then
  echo "subscription usage storage methods remain in db.go"
  exit 1
fi
git diff --check
```

## Acceptance

- Subscription usage storage methods compile from the new file.
- SQL, logging, nullable handling, ordering, and scan behavior are unchanged.
- `db.go` is smaller and no longer owns subscription usage snapshot methods.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

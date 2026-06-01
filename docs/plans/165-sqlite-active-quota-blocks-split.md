# 165 SQLite Active Quota Blocks Split

## Context

The target architecture uses local quota observations as metadata-only routing
input for same-provider, same-model credential pooling. Plan 097 added the
`ActiveQuotaBlocks(ctx, providerInstanceID, modelID, now)` read interface so
the server can avoid credentials with active quota pressure. Recent SQLite
storage slices moved subscription usage, model cache, telemetry writes,
summaries, request metadata recording, and pruning out of
`internal/storage/sqlite/db.go`.

`db.go` still owns `ActiveQuotaBlocks` and `activeQuotaFallbackCooldown`. This
is a routing-sensitive read, but it is also a focused quota storage concern:
it reads `quota_events`, parses retry/reset timestamps, derives a local active
window, and returns safe metadata-only blocks. Moving it into its own file makes
the quota-read boundary explicit without changing credential planning behavior.

## Goal

Move SQLite active quota block reads out of `db.go` into
`internal/storage/sqlite/quota_blocks.go` without changing behavior.

After this slice:

- `quota_blocks.go` owns `ActiveQuotaBlocks` and
  `activeQuotaFallbackCooldown`.
- `events.go` owns quota observation writes.
- `summaries.go` owns quota observability summaries.
- `pruning.go` owns quota pruning as part of telemetry pruning.
- `db.go` no longer owns non-credential telemetry or quota storage methods.

## Scope

1. Add `internal/storage/sqlite/quota_blocks.go`.
2. Move `activeQuotaFallbackCooldown` and `ActiveQuotaBlocks` intact from
   `db.go` into the new file.
3. Include only imports required by the moved code.
4. Keep shared helpers such as `parseSQLiteTime` at package scope.
5. Do not change SQL text, ordering, deduplication, cooldown duration,
   retry/reset precedence, active-until filtering, server credential planning,
   metadata structs, management DTOs, TUI rendering, provider behavior, config,
   auth, migrations, or tests.

## Out of Scope

- Quota policy changes.
- Credential planner changes.
- SQL/index/schema changes.
- Management or TUI changes.
- Quota observation write changes.
- Telemetry pruning changes.
- Permanent tests.
- Broader storage refactors.

## Implementation Steps

1. Create `internal/storage/sqlite/quota_blocks.go` with `package sqlite`.
2. Move `activeQuotaFallbackCooldown` and `ActiveQuotaBlocks` from `db.go` into
   the new file.
3. Add imports for `context`, `database/sql`, `time`, and
   `ilonasin/internal/metadata` as required.
4. Remove any now-unused imports from `db.go`.
5. Run `gofmt` on touched Go files.
6. Review the diff before running checks, with special attention to SQL,
   duplicate-row handling, active window calculation, and UTC normalization.

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
rg -n "ActiveQuotaBlocks|activeQuotaFallbackCooldown" internal/storage/sqlite/quota_blocks.go
if rg -n "ActiveQuotaBlocks|activeQuotaFallbackCooldown" internal/storage/sqlite/db.go; then
  echo "active quota blocks remain in db.go"
  exit 1
fi
```

## Acceptance

- `ActiveQuotaBlocks` and `activeQuotaFallbackCooldown` compile from
  `quota_blocks.go`.
- `db.go` no longer owns active quota block reads.
- SQL, ordering, deduplication, cooldown duration, retry/reset handling, and
  active-until filtering are unchanged.
- Public storage interfaces and behavior remain unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

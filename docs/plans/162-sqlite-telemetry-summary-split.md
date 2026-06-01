# 162 SQLite Telemetry Summary Split

## Context

The target architecture keeps SQLite as the storage boundary for metadata-only
request, usage, latency, health, fallback, and quota observations. Recent
storage slices moved subscription usage, model cache, and event recording out of
`internal/storage/sqlite/db.go`.

`db.go` still owns read-side telemetry summary methods:

- `RecentRequests`
- `UsageByProvider`
- `LatencyByProvider`
- `StreamSummary`
- `LatestHealth`
- `RecentFallbacks`
- `QuotaByProvider`

These methods are a coherent management/observability read cluster. Moving them
into a focused same-package file continues reducing the monolithic SQLite store
without changing SQL, storage schema, telemetry semantics, management DTOs, TUI
rendering, routing, providers, config, or auth.

## Goal

Move SQLite telemetry summary readers out of `db.go` into a focused
`internal/storage/sqlite/summaries.go` file while preserving behavior exactly.

After this slice:

- `summaries.go` owns read-only telemetry summary queries.
- `db.go` keeps core store setup, credential storage, request metadata
  recording, pruning, fallback policy storage, and active quota block routing
  helpers for later slices.
- Metadata-only observability behavior stays unchanged.

## Scope

1. Add `internal/storage/sqlite/summaries.go`.
2. Move these methods intact from `db.go`:
   - `RecentRequests`
   - `UsageByProvider`
   - `LatencyByProvider`
   - `StreamSummary`
   - `LatestHealth`
   - `RecentFallbacks`
   - `QuotaByProvider`
3. Include only imports needed by the moved methods.
4. Keep existing helpers shared from package scope, including `parseSQLiteTime`,
   `tokenRate`, and `cacheMissTokens`.
5. Do not move `PruneTelemetryBefore` in this slice because pruning has
   cross-table delete semantics.
6. Do not move `ActiveQuotaBlocks` in this slice because it is routing-sensitive
   read behavior.
7. Do not change SQL text, JSON DTOs, TUI rendering, provider behavior,
   routing, config, auth, migrations, or tests.

## Out of Scope

- SQLite schema changes.
- New management routes.
- TUI changes.
- Request metadata write changes.
- Telemetry pruning changes.
- Active quota block or credential routing changes.
- Permanent tests.
- Broader storage refactors.

## Implementation Steps

1. Create `internal/storage/sqlite/summaries.go` with `package sqlite`.
2. Move the seven summary reader methods from `db.go` into the new file.
3. Add imports for `context`, `database/sql`, and `ilonasin/internal/metadata`
   if needed by the moved code.
4. Remove now-unused imports from `db.go`.
5. Run `gofmt` on touched Go files.
6. Review the diff before running checks.

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
rg -n "RecentRequests|UsageByProvider|LatencyByProvider|StreamSummary|LatestHealth|RecentFallbacks|QuotaByProvider" internal/storage/sqlite/summaries.go
if rg -n "RecentRequests|UsageByProvider|LatencyByProvider|StreamSummary|LatestHealth|RecentFallbacks|QuotaByProvider" internal/storage/sqlite/db.go; then
  echo "telemetry summary readers remain in db.go"
  exit 1
fi
```

## Acceptance

- The seven summary reader methods compile from `summaries.go`.
- `db.go` no longer owns those summary reader methods.
- Public storage interfaces and behavior remain unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

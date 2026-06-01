# 164 SQLite Telemetry Pruning Split

## Context

The target architecture treats SQLite as the daemon-owned mutable source of
truth and keeps metadata pruning behind the management API. Plans 159 through
163 progressively split focused SQLite storage clusters out of
`internal/storage/sqlite/db.go`: subscription usage, model cache, telemetry
event writes, telemetry summaries, and request metadata recording.

`db.go` still owns `PruneTelemetryBefore`, a cross-table metadata-only pruning
operation for request metadata, stream metrics, fallback events, health events,
and quota events. The method is intentionally separate from credential storage,
provider behavior, routing, and TUI rendering. Moving it into its own storage
file makes that boundary explicit while preserving the plan 009 and plan 084
pruning semantics.

## Goal

Move SQLite telemetry pruning out of `db.go` into
`internal/storage/sqlite/pruning.go` without changing behavior.

After this slice:

- `pruning.go` owns `PruneTelemetryBefore` and `resetPruneTable`.
- `request_metadata.go` owns request metadata writes.
- `events.go` owns secondary telemetry event writes.
- `summaries.go` owns read-only telemetry summaries.
- `db.go` keeps store setup, migrations, credential storage, fallback policy
  storage, and active quota block reads for later slices.

## Scope

1. Add `internal/storage/sqlite/pruning.go`.
2. Move `PruneTelemetryBefore` and `resetPruneTable` intact from `db.go` into
   the new file.
3. Include only imports required by the moved code.
4. Keep shared helpers such as `parseSQLiteTime` at package scope.
5. Do not change prune table names, SQL text, deletion order, cutoff semantics,
   result counts, log fields, management DTOs, TUI rendering, routing,
   providers, credentials, config, migrations, or tests.
6. Leave `ActiveQuotaBlocks` in `db.go` for a later routing-sensitive read
   split.

## Out of Scope

- SQLite schema changes.
- Pruning semantic changes.
- Scheduled pruning.
- TUI or management route changes.
- Credential, OAuth, provider, routing, or quota policy changes.
- Active quota block changes.
- Permanent tests.
- Broader storage refactors.

## Implementation Steps

1. Create `internal/storage/sqlite/pruning.go` with `package sqlite`.
2. Move `PruneTelemetryBefore` and `resetPruneTable` from `db.go` into the new
   file.
3. Add imports for `context`, `database/sql`, `log/slog`, `time`, and
   `ilonasin/internal/metadata` as required.
4. Remove any now-unused imports from `db.go`.
5. Run `gofmt` on touched Go files.
6. Review the diff before running checks, with special attention to temp table
   names, strict-before cutoff behavior, delete order, and counts.

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
rg -n "PruneTelemetryBefore|resetPruneTable" internal/storage/sqlite/pruning.go
if rg -n "PruneTelemetryBefore|resetPruneTable" internal/storage/sqlite/db.go; then
  echo "telemetry pruning remains in db.go"
  exit 1
fi
```

## Acceptance

- `PruneTelemetryBefore` and `resetPruneTable` compile from `pruning.go`.
- `db.go` no longer owns telemetry pruning.
- Temp table names, cutoff behavior, delete order, counts, and log fields are
  unchanged.
- Public storage interfaces and behavior remain unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

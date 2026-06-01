# 161 SQLite Event Recording Split

## Context

The target architecture treats SQLite as the storage boundary for typed
metadata-only telemetry. Plans 007, 019, 020, 081, and 102 established request
usage, stream, health, fallback, and quota events as safe scalar metadata with
no prompts, completions, raw bodies, raw provider payloads, raw SSE chunks,
tool arguments/results, bearer tokens, provider request IDs, full account IDs,
balances, or credits.

`internal/storage/sqlite/db.go` still owns several event-recording methods:

- `RecordStreamMetrics`
- `RecordHealthEvent`
- `RecordFallbackEvent`
- `RecordQuotaObservation`

These methods are a coherent write-side telemetry cluster. Moving them into a
focused storage file reduces the monolithic SQLite store while keeping request
metadata recording, telemetry pruning, summary readers, and quota block readers
unchanged for later slices.

## Goal

Move SQLite event-recording methods out of `db.go` into a focused same-package
file without changing behavior.

After this slice:

- `internal/storage/sqlite/events.go` owns stream, health, fallback, and quota
  event writes.
- `db.go` no longer contains those event-recording methods.
- The `Store` interface surface remains unchanged for callers.

## Scope

1. Add `internal/storage/sqlite/events.go`.
2. Move `RecordStreamMetrics` from `db.go` unchanged.
3. Move `RecordHealthEvent` from `db.go` unchanged.
4. Move `RecordFallbackEvent` from `db.go` unchanged.
5. Move `RecordQuotaObservation` from `db.go` unchanged.
6. Add only the imports needed by moved code.
7. Remove any now-unused imports from `db.go`.
8. Do not change SQL text, nullable handling, timestamp handling, logging
   attributes, event classes, table names, migrations, pruning, summaries, or
   quota block behavior.
9. Do not change server recording logic, provider adapters, routing,
   management DTOs, TUI rendering, config, auth, or request metadata.
10. Do not add permanent tests.
11. Do not push.

## Non-Goals

- No SQLite schema changes.
- No query optimization.
- No migration split.
- No changes to `RecordRequestMetadata`.
- No telemetry summary reader moves in this slice.
- No telemetry pruning moves in this slice.
- No quota policy or routing changes.
- No broader storage refactor in this slice.

## Implementation

1. Create `internal/storage/sqlite/events.go` with `package sqlite`.
2. Move the four event-recording methods unchanged from `db.go`.
3. Keep dependencies on existing helpers such as `nullableInt`,
   `nullableInt64`, `nullableTime`, and `boolToInt`.
4. Run `gofmt`.
5. Review the diff to confirm this is relocation only plus import cleanup.
6. Verify `rg -n "RecordStreamMetrics|RecordHealthEvent|RecordFallbackEvent|RecordQuotaObservation"
   internal/storage/sqlite` shows the methods in the new file.

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
rg -n "RecordStreamMetrics|RecordHealthEvent|RecordFallbackEvent|RecordQuotaObservation" internal/storage/sqlite/events.go
if rg -n "RecordStreamMetrics|RecordHealthEvent|RecordFallbackEvent|RecordQuotaObservation" internal/storage/sqlite/db.go; then
  echo "event recording methods remain in db.go"
  exit 1
fi
git diff --check
```

## Acceptance

- Event-recording methods compile from the new file.
- SQL, logging attributes, nullable handling, timestamp handling, event classes,
  and table names are unchanged.
- `db.go` is smaller and no longer owns stream, health, fallback, or quota
  event write methods.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

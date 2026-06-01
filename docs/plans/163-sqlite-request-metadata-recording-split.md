# 163 SQLite Request Metadata Recording Split

## Context

The target architecture treats SQLite as the storage boundary for metadata-only
observability. Plans 159 through 162 progressively split focused storage
clusters out of `internal/storage/sqlite/db.go`: subscription usage, model
cache, event recording, and telemetry summary readers.

`db.go` still owns `RecordRequestMetadata`, the write-side request metadata
ledger. That method is a coherent storage concern: it inserts one safe
metadata-only request row and emits structured metadata-only logging. It does
not need to live beside credential storage, pruning, migrations, or routing
quota reads.

Moving it into a focused file continues reducing the monolithic SQLite store
while preserving the architecture requirement that telemetry remains
metadata-only and that request bodies, prompts, completions, provider payloads,
tokens, tool data, and full account IDs are not stored or logged.

## Goal

Move SQLite request metadata recording out of `db.go` into
`internal/storage/sqlite/request_metadata.go` without changing behavior.

After this slice:

- `request_metadata.go` owns `RecordRequestMetadata`.
- `events.go` owns secondary telemetry event writes.
- `summaries.go` owns read-only telemetry summaries.
- `db.go` keeps store setup, migrations, credential storage, fallback policy
  storage, telemetry pruning, and active quota block reads for later slices.

## Scope

1. Add `internal/storage/sqlite/request_metadata.go`.
2. Move `RecordRequestMetadata` intact from `db.go` into the new file.
3. Include only imports required by the moved method.
4. Keep shared helpers such as `boolToInt`, `cacheMissTokens`, and `tokenRate`
   at package scope.
5. Do not change inserted columns, argument order, logging fields, SQL text,
   metadata shape, pruning behavior, summary readers, event writers, routing,
   provider behavior, management DTOs, TUI rendering, config, auth, migrations,
   or tests.

## Out of Scope

- SQLite schema changes.
- Metadata field additions or removals.
- Logging behavior changes.
- Provider, routing, management, or TUI changes.
- Telemetry pruning changes.
- Active quota block changes.
- Permanent tests.
- Broader storage refactors.

## Implementation Steps

1. Create `internal/storage/sqlite/request_metadata.go` with `package sqlite`.
2. Move `RecordRequestMetadata` from `db.go` into the new file.
3. Add imports for `context`, `log/slog`, `time`, and
   `ilonasin/internal/metadata` as required.
4. Remove any now-unused imports from `db.go`.
5. Run `gofmt` on touched Go files.
6. Review the diff before running checks, with special attention to inserted
   column order, value order, and structured logging fields.

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
rg -n "RecordRequestMetadata" internal/storage/sqlite/request_metadata.go
if rg -n "RecordRequestMetadata" internal/storage/sqlite/db.go; then
  echo "request metadata recorder remains in db.go"
  exit 1
fi
```

## Acceptance

- `RecordRequestMetadata` compiles from `request_metadata.go`.
- `db.go` no longer owns request metadata recording.
- Inserted SQL, value order, derived token/cache rates, and logging fields are
  unchanged.
- Public storage interfaces and behavior remain unchanged.
- Direct compile, vet, serve, management route, and manage PTY smokes pass.

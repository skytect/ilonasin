# 158 Metadata Only IO Logging

## Context

The architecture requires metadata-only observability and explicitly forbids
storing prompts, completions, request bodies, response bodies, raw provider
payloads, raw SSE chunks, tool arguments, tool results, full bearer tokens,
full provider request IDs, and full account IDs.

Plan 077 added structured logging and stated that it must not introduce raw HTTP
capture. Plan 102 reiterated that body capture, prompt capture, and unsafe debug
mode are out of scope.

The current `capture_io` path in `internal/logging/io.go` and
`internal/server/io_logging.go` writes an `ilonasin-io.log` record with a raw
`body` field for both request input and response output. That conflicts with
the target architecture even though the feature is opt-in. The safe part of the
feature is the metadata: direction, method, route, status, content type, event
ID, and byte counts.

## Goal

Make IO logging metadata-only without changing route behavior.

After this slice:

- `capture_io` no longer persists request or response bodies.
- IO log records keep safe operational metadata and byte counts.
- Server routes may still read request and response bytes transiently for
  normal parsing and forwarding, but the logging boundary stores no body text.

## Scope

1. Remove `Body` from `logging.IORecord`.
2. Stop converting request and response byte slices into strings for IO log
   records.
3. Preserve byte counts, direction, method, route, status, content type, event
   ID, and timestamp.
4. Keep `capture_io` config compatibility for now, but redefine the feature as
   metadata-only IO logging.
5. Replace response wrapping with count-only status and byte tracking. Do not
   buffer response bodies in the wrapper.
6. Do not change provider adapters, request parsing, response writing,
   management APIs, storage, TUI rendering, routing, auth, or normal structured
   logs.
7. Do not add permanent tests.
8. Do not push.

## Non-Goals

- No unsafe debug mode.
- No body capture under a different field name.
- No config migration or config key rename.
- No removal of `capture_io` in this slice.
- No changes to persisted SQLite request metadata.
- No changes to provider response bodies returned to local clients.

## Implementation

1. Update `internal/logging/io.go`:
   - remove `Body string` from `IORecord`,
   - keep JSON field compatibility for all safe metadata fields.
2. Update `internal/server/io_logging.go`:
   - set `Bytes` only for input/output body lengths,
   - do not store body strings in records.
3. Replace `ioCaptureResponseWriter` with a count-only response writer that
   records status and byte counts without retaining response bytes.
4. Run `gofmt`.
5. Review the diff for any remaining `Body: string(body)`, `bytes.Buffer`, or
   `capture.body` usage in IO logging.
6. Smoke `capture_io = true` with a local request and assert
   `<tmp>/logs/ilonasin-io.log` contains no `body` JSON key and no request
   prompt marker while still containing input/output records and byte counts.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
port="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"
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
bind = "__BIND__"
[paths]
log_dir = "__LOG_DIR__"
[logging]
capture_io = true
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
sed -i "s#__BIND__#127.0.0.1:$port#g" "$cfg"
sed -i "s#__LOG_DIR__#$tmp/logs#g" "$cfg"
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
token="$(curl --silent --fail --unix-socket "$sock" -X POST http://ilonasin/_ilonasin/manage/local-tokens -d '{"label":"smoke"}' | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
curl --silent --output "$tmp/chat.out" --write-out "%{http_code}" \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  --data '{"model":"missing:model","messages":[{"role":"user","content":"prompt-body-marker-unsafe"}]}' \
  "http://127.0.0.1:$port/v1/chat/completions" >/dev/null || true
if [ ! -s "$tmp/logs/ilonasin-io.log" ]; then
  echo "missing io log"
  cat "$tmp/serve.log"
  exit 1
fi
if grep -q '"body"' "$tmp/logs/ilonasin-io.log" ||
  grep -q 'prompt-body-marker-unsafe' "$tmp/logs/ilonasin-io.log"; then
  echo "io log captured body content"
  cat "$tmp/logs/ilonasin-io.log"
  exit 1
fi
grep -q '"direction":"input"' "$tmp/logs/ilonasin-io.log"
grep -q '"direction":"output"' "$tmp/logs/ilonasin-io.log"
grep -q '"bytes":' "$tmp/logs/ilonasin-io.log"
if rg -n 'Body: string\\(|bytes\\.Buffer|capture\\.body' internal/server internal/logging; then
  echo "unsafe io body capture remains"
  exit 1
fi
git diff --check
```

## Acceptance

- `logging.IORecord` has no body field.
- IO logging persists byte counts and safe metadata only.
- IO response wrapping counts bytes without retaining response bodies.
- No route behavior changes.
- Direct compile, vet, serve, management route, manage PTY, and metadata-only
  IO logging smokes pass.

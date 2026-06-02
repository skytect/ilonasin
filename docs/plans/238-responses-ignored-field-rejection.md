# 238 Responses Ignored Field Rejection

## Context

`docs/ilonasin-architecture.md` requires the local API surface to be strict:
unsupported fields should return clear errors instead of being silently
forwarded or ignored.

The local `/responses` parser currently allows `prompt_cache_key` and
`client_metadata`, type-checks them, and then drops them during conversion to
the common chat request. The Codex provider adapter later sends internally
generated `prompt_cache_key` and `client_metadata` values for its own routing
needs. That makes client-supplied values accepted-but-unhonored.

## Goal

Reject client-supplied `/responses` `prompt_cache_key` and `client_metadata`
fields with clear unsupported-field errors until they are explicitly modeled and
forwarded.

## Scope

1. Reject `prompt_cache_key` and `client_metadata` explicitly before generic
   top-level unsupported-field handling.
2. Return a clear bad-request error when either field is present, including when
   the value is `null`. The message must identify the field, for example
   `prompt_cache_key is unsupported`.
3. Keep Codex adapter internally generated `prompt_cache_key` and
   `client_metadata` behavior unchanged.
4. Keep supported `/responses` fields and conversion behavior unchanged.
5. Do not change chat completions, Anthropic routes, provider adapters, storage,
   metadata schema, management DTOs, TUI, config, or logging behavior.
6. Do not add permanent tests.
7. Do not touch unrelated concurrent work.

## Non-Goals

- No forwarding support for client-supplied `prompt_cache_key`.
- No forwarding support for client-supplied `client_metadata`.
- No route preflight refactor.
- No provider adapter changes.
- No docs architecture rewrite in this slice.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmp=$(mktemp -d)
tmpbin="$tmp/bin"
mkdir -p "$tmpbin"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
port=$(python - <<'PY'
import socket
s=socket.socket()
s.bind(('127.0.0.1',0))
print(s.getsockname()[1])
s.close()
PY
)
cat >"$tmp/config.toml" <<EOF
[server]
bind = "127.0.0.1:$port"

[paths]
database = "$tmp/home/ilonasin.sqlite"
log_dir = "$tmp/home/logs"
cache_dir = "$tmp/home/cache"

[logging]
capture_io = false

[subscription_keepalive]
enabled = false

[providers.deepseek]
type = "deepseek"

[providers.codex]
type = "codex"
EOF
cleanup() {
  if [ -n "${pid:-}" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$tmp/config.toml" >"$tmp/serve.log" 2>&1 &
pid=$!
for i in $(seq 1 50); do
  if [ -d "$tmp/home/run" ] && find "$tmp/home/run" -name 'manage-*.sock' -type s | rg . >/dev/null; then
    break
  fi
  sleep 0.1
done
sock="$(find "$tmp/home/run" -name 'manage-*.sock' -type s | head -n 1)"
test -S "$sock"
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
for cols in 80 120 160; do
  timeout 4s script -q -e -c "stty cols $cols rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage-$cols.out" || true
  rg "api|providers|usage|logs" "$tmp/manage-$cols.out" >/dev/null
done
```

Also run a temporary focused parser smoke, then remove it before commit. It
should assert:

- `prompt_cache_key` present with a string is rejected and the error message
  contains `prompt_cache_key`;
- `prompt_cache_key` present with `null` is rejected and the error message
  contains `prompt_cache_key`;
- `client_metadata` present with an object is rejected and the error message
  contains `client_metadata`;
- `client_metadata` present with `null` is rejected and the error message
  contains `client_metadata`;
- an otherwise equivalent minimal supported `/responses` request still decodes.

Also run a disposable HTTP smoke against `ilonasin serve` with a temp home and
local token. Prove `POST /responses` returns `400` for string/object/null cases,
that the JSON error message contains the rejected field name, and that the
error envelope retains `type=invalid_request_error` and `code=invalid_json`
unless a narrower decode-error classifier is added in this slice. Remove all
temporary artifacts.

## Acceptance

- `/responses` no longer accepts fields that are currently ignored.
- Error messages identify the unsupported field clearly.
- Codex provider internally generated request IDs and metadata remain unchanged.
- Compile, vet, serve/manage smoke, focused parser smoke, whitespace checks,
  and implementation review pass.

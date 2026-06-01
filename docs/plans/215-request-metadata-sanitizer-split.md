# 215 Request Metadata Sanitizer Split

## Context

`docs/ilonasin-architecture.md` allows metadata-only telemetry but forbids
storing prompts, completions, request bodies, response bodies, raw provider
payloads, raw stream chunks, full bearer tokens, full provider request IDs, and
full account IDs in normal operation. Request metadata therefore needs small,
explicit sanitizers for values that are safe to persist.

Plans 210 through 214 split request option metadata, image counting,
throughput math, quota observation construction, and token-limit extraction
out of `internal/server/request_metadata.go`. The remaining provider-type
sanitizer, `safeMetadataToken`, still lives in that file. It is a distinct
metadata-safety concern and should live with other request metadata helpers
rather than base request construction.

The worktree currently contains unrelated uncommitted changes in
`internal/server/chat_nonstream.go`, `internal/server/chat_stream.go`, and
`internal/server/credentials.go`. This slice must not modify or stage those
files.

## Goal

Move provider metadata token sanitization into a focused helper file without
changing which provider-type values are accepted or rejected.

## Scope

1. Add `internal/server/request_metadata_sanitize.go`.
2. Move `safeMetadataToken` from `request_metadata.go` into the new file.
3. Preserve exact allowlist behavior:
   - ASCII letters are allowed;
   - ASCII digits are allowed;
   - `_`, `-`, `.`, and `/` are allowed;
   - any other rune makes the returned value empty.
4. Keep `requestMetadataBase` and `responsesRequestMetadataBase` sanitizing
   `instance.Type` through `safeMetadataToken`.
5. Do not change endpoint names, base metadata construction, chat
   finalization, option sanitization, image counting, token-limit extraction,
   quota observations, throughput math, route handlers, provider adapters,
   storage, management, TUI, config, IO logging, schema, or public route shape.
6. Do not modify or stage unrelated dirty files.

## Non-Goals

- No behavior change.
- No schema change.
- No new sanitizer rules.
- No permanent tests.

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
snapshot="$(curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot)"
printf '%s' "$snapshot" | jq -e '.providers | length >= 2' >/dev/null
timeout 3s script -q -e -c "stty cols 140 rows 45; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >/dev/null || true
```

Also run a temporary focused in-package smoke proving:

- a normal provider type such as `codex` is preserved;
- slash, dash, underscore, and dot remain allowed;
- whitespace, `@`, and non-ASCII characters return empty;
- `requestMetadataBase` and `responsesRequestMetadataBase` still populate
  sanitized provider type.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- the helper is behavior-equivalent;
- both metadata base constructors still call `safeMetadataToken`;
- no route, provider, storage, management, TUI, config, schema, IO logging, or
  unrelated dirty files changed.

## Acceptance

- Provider metadata token sanitization lives in
  `request_metadata_sanitize.go`.
- Request metadata constructors record the same sanitized provider type values
  as before.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass, or any failure is proven to come from unrelated pre-existing dirty
  work.

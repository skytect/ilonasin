# 201 Chat Metadata Finalizer

## Context

`docs/ilonasin-architecture.md` treats the request metadata ledger as a core
side-plane: request handling and provider adapters should produce metadata-only
usage, latency, retry, fallback, and credential information without storing
request or response bodies.

The non-streaming and streaming chat paths currently duplicate much of the
metadata finalization in `internal/server/chat_nonstream.go` and
`internal/server/chat_stream.go`: credential ID, resolved model, retry counts,
fallback counts, usage counters, cost, latency, effective service tier, and
output-token-per-second fields. That duplication makes future changes to the
metadata contract easier to apply to one path but miss the other.

## Goal

Extract shared chat request metadata population into a focused server helper
without changing execution behavior, request/response wire shape, fallback
behavior, health recording, quota observations, stream metrics, or persisted
field values.

## Scope

1. Add a helper in `internal/server/request_metadata.go` that fills the common
   final metadata fields for both chat paths.
2. Keep request metadata base construction in `requestMetadataBase`.
3. Keep stream-only fields in `handleStreamingChat`:
   - `TimeToFirstTokenMS`
   - stream summary fallback for total TPS
   - after-TTFT TPS
   - `metadata.Stream` recording
4. Keep non-stream-specific record context and response writing in
   `chat_nonstream.go`.
5. Keep stream execution, stream sink, pre-stream error writing, and stream
   completion recording in `chat_stream.go`.
6. Do not change retry/fallback selection, quota observation generation,
   provider health recording, adapter calls, IO logging, request validation,
   response writers, storage schema, management DTOs, or TUI rendering.

## Non-Goals

- No behavior change.
- No storage or migration change.
- No route, provider, or TUI change.
- No permanent tests.
- No attempt to unify streaming and non-streaming execution loops in this
  slice.

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

Also run a temporary focused in-package smoke for the new helper that proves:

- non-stream metadata fields match the current `recordNonStreamingChat`
  assignments for credential, resolved model, HTTP status, error class,
  retry/auth/attempt/fallback counts, fallback reason, usage, cache, cost,
  latency, service tier, and total TPS;
- stream metadata fields match the current `handleStreamingChat` assignments
  for the same common fields;
- non-stream `RequestedModel` and `MaxOutputTokens` overrides from
  `nc.clientModel` and `nc.maxOutputTokens` are preserved, either outside the
  helper or through explicit helper inputs;
- stream-only TTFT and after-TTFT TPS remain outside the common helper.

Remove the temporary smoke before commit.

During diff review, explicitly verify that:

- `executeNonStreamingChat` is unchanged except for any compile-required names;
- `handleNonStreamingChat` response/error behavior is unchanged;
- `handleStreamingChat` retry/fallback loop and pre-stream error behavior are
  unchanged;
- stream sink headers, flushing, and IO logging are unchanged.

## Acceptance

- Common chat metadata finalization is centralized in one helper.
- Non-streaming and streaming chat metadata field values are unchanged.
- Execution, response writing, health, quota, fallback, and stream metric
  behavior are unchanged.
- Compile, vet, serve smoke, manage smoke, focused metadata smoke, and
  whitespace checks pass.

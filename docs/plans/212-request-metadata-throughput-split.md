# 212 Request Metadata Throughput Split

## Context

`docs/ilonasin-architecture.md` treats metadata-only observability as a core
boundary and explicitly allows latency, TTFT, output tokens per second, and
stream completion status as telemetry. `internal/server/request_metadata.go`
still mixes base request metadata construction, finalization, quota observation
helpers, provider token sanitization, token-limit extraction, and throughput
math.

Plans 210 and 211 already split request option sanitization and image counting
into focused files. Throughput math is another distinct metadata concern:
calculating derived output token rates from already-safe token and timing
counts.

## Goal

Move request throughput helper functions into a focused file without changing
how output token rates are calculated or recorded.

## Scope

1. Add `internal/server/request_metadata_throughput.go`.
2. Move these helpers from `request_metadata.go` into the new file:
   - `outputTPS`;
   - `outputTPSAfterTTFT`.
3. Preserve exact behavior:
   - zero or negative completion token counts return `0`;
   - zero or negative total latency returns `0`;
   - zero or negative TTFT returns `0` for post-TTFT throughput;
   - total latency less than or equal to TTFT returns `0`;
   - otherwise rates remain `completion_tokens / elapsed_seconds`.
4. Keep `finalizeChatRequestMetadata` assigning
   `OutputTokensPerSecondTotal` through `outputTPS`.
5. Keep `recordStreamingChat` assigning `OutputTokensPerSecondAfterTTFT`
   through `outputTPSAfterTTFT`.
6. Do not change base metadata construction, option sanitization, image
   counting, quota observations, route handlers, provider adapters, storage,
   management, TUI, config, IO logging, schema, or public route shape.

## Non-Goals

- No behavior change.
- No schema change.
- No new metric fields.
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

Also run a temporary focused in-package smoke covering:

- `outputTPS(10, 2000) == 5`;
- zero or negative completion tokens return `0`;
- zero or negative latency returns `0`;
- `outputTPSAfterTTFT(10, 3000, 1000) == 5`;
- zero or negative TTFT returns `0`;
- latency equal to or below TTFT returns `0`;
- `finalizeChatRequestMetadata` still populates
  `OutputTokensPerSecondTotal` and `OutputTokensPerSecond`.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- moved functions are behavior-equivalent;
- `request_metadata.go` retains only calls to the moved helpers;
- no storage fallback behavior for `OutputTokensPerSecondTotal` changes;
- no route, execution, provider, management, TUI, config, or IO logging code
  changed.

## Acceptance

- Request throughput math lives in `request_metadata_throughput.go`.
- Request metadata finalization and streaming metadata recording compute the
  same output token rates as before.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass.

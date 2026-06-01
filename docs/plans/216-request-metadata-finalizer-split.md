# 216 Request Metadata Finalizer Split

## Context

`docs/ilonasin-architecture.md` treats request metadata as a daemon-owned
side-plane for usage, latency, fallback, retry, credential, and resolved-model
information without storing prompts, completions, request bodies, response
bodies, raw provider payloads, raw stream chunks, tool arguments/results, full
bearer tokens, full provider request IDs, or full account IDs.

Plan 201 centralized shared chat metadata finalization into
`chatMetadataFinalizer` and `finalizeChatRequestMetadata`. Plans 210 through
215 then split surrounding request metadata helpers into focused files. The
finalizer now remains in `internal/server/request_metadata.go`, leaving that
file to mix base request metadata construction with final response metadata
population.

The worktree currently contains unrelated uncommitted changes in
`internal/server/chat_nonstream.go`, `internal/server/chat_stream.go`, and
`internal/server/credentials.go`. This slice must not modify or stage those
files.

## Goal

Move common chat request metadata finalization into a focused file without
changing any finalized metadata fields or chat execution behavior.

## Scope

1. Add `internal/server/request_metadata_finalizer.go`.
2. Move these definitions from `request_metadata.go` into the new file:
   - `chatMetadataFinalizer`;
   - `finalizeChatRequestMetadata`.
3. Preserve exact field mapping:
   - credential ID;
   - resolved model through `resolvedChatModel`;
   - HTTP status;
   - error class;
   - retry/auth/attempt/fallback counts;
   - fallback reason through `fallbackReason`;
   - prompt, completion, total, reasoning, cache-hit, and cache-write tokens;
   - cost microunits;
   - total and upstream latency milliseconds;
   - effective service tier when non-empty;
   - total output TPS through `outputTPS`;
   - `OutputTokensPerSecond` mirroring total output TPS.
4. Keep existing call sites unchanged in:
   - `recordNonStreamingChat`;
   - `recordStreamingChat`.
5. Do not change stream-only TTFT and after-TTFT TPS logic, non-stream request
   model/max-token overrides, request metadata base construction, option
   sanitization, image counting, token-limit extraction, quota observations,
   throughput math, route handlers, provider adapters, storage, management,
   TUI, config, IO logging, schema, or public route shape.
6. Do not modify or stage unrelated dirty files.

## Non-Goals

- No behavior change.
- No schema change.
- No new metadata fields.
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

Also run a temporary focused in-package smoke proving
`finalizeChatRequestMetadata` still populates every common finalizer field,
including resolved model fallback, fallback reason, effective service tier
empty/non-empty behavior, latency milliseconds, and total output TPS.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- the moved type and helper are behavior-equivalent;
- `recordNonStreamingChat` and `recordStreamingChat` still call the helper in
  the same way;
- stream-only TTFT and after-TTFT TPS remain outside the helper;
- no route, provider, storage, management, TUI, config, schema, IO logging, or
  unrelated dirty files changed.

## Acceptance

- Common chat metadata finalization lives in
  `request_metadata_finalizer.go`.
- Non-streaming and streaming chat metadata finalization values are unchanged.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass, or any failure is proven to come from unrelated pre-existing dirty
  work.

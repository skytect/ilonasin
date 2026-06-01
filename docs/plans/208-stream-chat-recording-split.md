# 208 Stream Chat Recording Split

## Context

`docs/ilonasin-architecture.md` separates provider execution, response writing,
metadata-only observability, and stream metrics. Non-streaming chat already has
a `recordNonStreamingChat` boundary that owns request metadata recording and
fallback/quota recording after execution.

Streaming chat now has `executeStreamingChat`, but `handleStreamingChat` still
constructs request metadata, derives the final stream status/error class,
records stream metrics, and records fallback/quota observations inline. That
keeps route orchestration coupled to metadata persistence.

## Goal

Move streaming chat metadata and stream-metric recording into focused helpers
without changing status codes, error classes, request metadata fields, stream
metric fields, fallback/quota recording, pre-response error writing order, or
SSE behavior.

## Scope

1. Add `streamStatusAndError(summary, sinkStarted)` to centralize the existing
   final status and error-class derivation:
   - use `summary.StatusCode` when present;
   - if missing and the sink started, use `200`;
   - if missing and the sink did not start, use `502`;
   - default empty error class to `"upstream_http_error"` for status `>= 400`.
2. Add `streamCompletionStatus(summary)` to preserve the existing default
   completion status of `"upstream_invalid"`.
3. Add `recordStreamingChat(r, sc, exec, summary, sinkStarted)` on `Server` to
   own:
   - detached recording context with five-second timeout;
   - `requestMetadataBase`;
   - `finalizeChatRequestMetadata`;
   - stream TTFT/TPS fields;
   - `recordWithID`;
   - `recordStream`;
   - `recordQuotaObservations`;
   - `recordFallbacks`.
4. Update `handleStreamingChat` to:
   - validate flusher;
   - create the sink;
   - execute streaming chat;
   - call `writeStreamingChatPreResponseError`;
   - call `recordStreamingChat`.
5. Preserve the existing order where pre-response error writing occurs before
   metadata recording.
6. Do not change streaming execution, retry/fallback policy, OAuth refresh,
   health recording, non-streaming chat, Anthropic compatibility, Responses API,
   provider adapters, storage, management, TUI, config, IO logging, or public
   route shape.

## Non-Goals

- No behavior change.
- No schema change.
- No retry/fallback policy change.
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

- `streamStatusAndError` returns `200` when status is missing and the sink
  started;
- `streamStatusAndError` returns `502 upstream_http_error` when status is
  missing and the sink did not start;
- non-empty stream error class is preserved;
- `streamCompletionStatus` defaults empty completion status to
  `"upstream_invalid"`;
- `recordStreamingChat` writes request metadata, stream metrics, quota
  observations, and fallback events with the same status, error class, retry
  counts, usage, latency, TTFT, TPS, completion status, and chunk count fields
  as the old inline path.
- the recorded request metadata explicitly preserves `CredentialID`,
  `ResolvedModel`, `EffectiveServiceTier`, `AuthRetryCount`, `AttemptCount`,
  `FallbackCount`, and `FallbackReason`;
- recording still uses a detached context with a five-second timeout so stream
  metadata can be recorded after client cancellation.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- pre-response error writing remains before recording;
- `summary` after `writeStreamingChatPreResponseError` is what
  `recordStreamingChat` receives;
- `OutputTokensPerSecond`, `OutputTokensPerSecondTotal`, and
  `OutputTokensPerSecondAfterTTFT` preserve current assignments;
- fallback/quota recordings still use the request metadata ID returned by
  `recordWithID`;
- `recordStreamingChat` keeps `context.WithTimeout(context.WithoutCancel(...),
  5*time.Second)` rather than the request context directly;
- stream execution, retry/fallback, OAuth refresh, health recording, SSE sink,
  and IO logging are unchanged.

## Acceptance

- Streaming chat recording is isolated in `recordStreamingChat`.
- `handleStreamingChat` is limited to route orchestration, execution,
  pre-response error writing, and recording delegation.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass.

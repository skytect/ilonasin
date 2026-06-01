# 207 Stream Chat Execution Split

## Context

`docs/ilonasin-architecture.md` separates route handling, routing policy,
credential resolution, provider adapter execution, metadata recording, and
response writing. Non-streaming chat already has an `executeNonStreamingChat`
boundary that owns provider attempts, OAuth refresh retry, fallback decisions,
health events, and quota observations.

Streaming chat still keeps its equivalent provider-attempt loop inside
`handleStreamingChat`. That makes the handler responsible for both route
orchestration and execution policy.

## Goal

Extract streaming provider-attempt execution into a focused helper so
`handleStreamingChat` owns flusher validation, sink setup, pre-response error
writing, and metadata recording, while the execution helper owns provider
attempts and retry/fallback/quota/health collection.

## Scope

1. Add a `streamExecution` struct near `streamAttempt` with:
   - final attempt;
   - fallback events;
   - quota observations;
   - auth retry count;
   - attempt count.
2. Add `executeStreamingChat(r, sc, sink)` on `Server`, mirroring
   `executeNonStreamingChat`.
3. Move the existing streaming execution block out of `handleStreamingChat`
   into the helper:
   - credential attempt planning;
   - quota-pool exhausted final attempt;
   - adapter `StreamChat` calls;
   - model-credential and attempt-credential OAuth refresh retry paths;
   - `recordHealth`;
   - quota observation collection;
   - retry reason selection;
   - fallback event collection.
4. Update `handleStreamingChat` to consume the returned execution object and
   preserve existing metadata finalization, stream metric recording,
   pre-response error writing, and fallback/quota recording.
5. Do not change non-streaming chat, Anthropic compatibility, Responses API,
   provider adapters, credential pool policy, storage, management, TUI, config,
   IO logging, or public route shape.

## Non-Goals

- No behavior change.
- No retry/fallback policy change.
- No status or error-class normalization change.
- No new abstraction shared with non-streaming chat in this slice.
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

Also run a temporary focused in-package smoke covering at least:

- quota-pool exhausted returns a final `429 upstream_quota_pool_exhausted`
  stream attempt and no adapter call;
- successful first attempt records one attempt and no fallback;
- retryable pre-stream failure records a fallback event and calls the next
  credential;
- sink-started failure does not produce a fallback retry.
- model-credential OAuth refresh retry increments `authRetries`, increments
  `attemptCount`, updates the model credential used by the retry, and preserves
  the retry result;
- attempt-credential OAuth refresh retry increments `authRetries`, increments
  `attemptCount`, updates the attempted credential used by the retry, and
  preserves the retry result.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- `handleStreamingChat` still writes the pre-response error before metadata
  recording, as before;
- summary status normalization still feeds request metadata;
- `authRetries`, `attemptCount`, fallback events, and quota observations are
  wired into metadata exactly as before;
- health recording, retry reasons, OAuth refresh retry behavior, SSE sink
  behavior, and IO logging are unchanged.

## Acceptance

- Streaming chat execution policy is isolated in `executeStreamingChat`.
- `handleStreamingChat` is narrower and mirrors the non-streaming chat shape.
- Compile, vet, serve smoke, manage smoke, and whitespace checks pass.

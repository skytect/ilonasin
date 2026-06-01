# 203 Chat Quota Observation Builder

## Context

`docs/ilonasin-architecture.md` requires quota pooling to remain auditable
through metadata-only request, health, fallback, and quota rows. The streaming
and non-streaming chat execution paths both construct `metadata.QuotaObservation`
rows inline after normalizing status and error class.

The row shape is duplicated across `internal/server/chat_nonstream.go` and
`internal/server/chat_stream.go`: observed time, provider instance, credential,
model, source, local HTTP status, normalized error class, and retry-after. This
is part of the metadata side-plane and should be a single local helper so future
quota metadata changes do not drift between stream and non-stream paths.

## Goal

Centralize chat quota-observation construction without changing quota detection,
retry/fallback decisions, health recording, metadata recording, response
behavior, stream behavior, IO logging, storage, management, TUI, or public
endpoints.

## Scope

1. Add a helper in `internal/server/request_metadata.go` or
   `internal/server/chat_helpers.go` that constructs a
   `metadata.QuotaObservation` from:
   - current time,
   - model address,
   - credential ID,
   - source,
   - status,
   - error class,
   - retry-after.
2. Replace the duplicated inline quota-observation literals in:
   - `executeNonStreamingChat`
   - `handleStreamingChat`
3. Preserve exact values:
   - `ObservedAt`
   - `ProviderInstanceID`
   - `CredentialID`
   - `ModelID`
   - `Source`
   - `HTTPStatus`
   - `ErrorClass`
   - `RetryAfter`
4. Keep `isQuotaObservation` checks exactly where they are.
5. Do not change retry/fallback selection, quota retryability checks, provider
   health recording, request metadata finalization, stream metrics, adapter
   calls, response writers, stream sink behavior, IO logging, storage,
   management, TUI, or config.

## Non-Goals

- No behavior change.
- No new quota policy.
- No new metadata fields.
- No storage or migration change.
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

Also run a temporary focused in-package smoke proving the helper preserves:

- provider instance ID;
- credential ID;
- model ID;
- source;
- status;
- error class;
- retry-after pointer;
- observed-at time from the injected clock.

Remove the temporary smoke before commit.

During diff review, explicitly verify that:

- `isQuotaObservation` calls remain outside the helper and unchanged;
- chat and stream sources remain `"chat"` and `"stream"`;
- retry-after still comes from `provider.ChatResult.RetryAfter` and
  `provider.ChatStreamSummary.RetryAfter`;
- retry/fallback branches, health recording, metadata finalization, response
  behavior, stream sink behavior, and IO logging are unchanged.

## Acceptance

- Quota-observation construction is centralized.
- Stream and non-stream quota row field values are unchanged.
- Compile, vet, serve smoke, manage smoke, focused helper smoke, and whitespace
  checks pass.

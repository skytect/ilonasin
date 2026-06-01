# 213 Request Metadata Quota Split

## Context

`docs/ilonasin-architecture.md` requires quota pooling to remain auditable
through metadata-only request, health, fallback, and quota rows. Plan 203
centralized chat quota-observation construction into `chatQuotaObservation`.
That helper now sits in `internal/server/request_metadata.go` alongside base
request metadata construction, finalization, token-limit extraction, and
provider-type sanitization.

Plans 210 through 212 split option metadata, image counting, and throughput
math into focused files. Quota observation construction is another distinct
metadata concern: it builds a safe row from model address, local credential ID,
status/error class, source, and retry-after timing.

## Goal

Move chat quota-observation construction into a focused file without changing
which quota rows are produced or how quota detection, retry, fallback, health,
or storage behave.

## Scope

1. Add `internal/server/request_metadata_quota.go`.
2. Move `chatQuotaObservation` from `request_metadata.go` into the new file.
3. Preserve exact field mapping:
   - `ObservedAt`;
   - `ProviderInstanceID`;
   - `CredentialID`;
   - `ModelID`;
   - `Source`;
   - `HTTPStatus`;
   - `ErrorClass`;
   - `RetryAfter`.
4. Keep existing call sites unchanged in:
   - `executeNonStreamingChat`;
   - `executeStreamingChat`.
5. Keep `isQuotaObservation` checks outside this helper and unchanged.
6. Do not change quota retryability checks, fallback decisions, health
   recording, request metadata finalization, stream metrics, route handlers,
   provider adapters, storage, management, TUI, config, IO logging, schema, or
   public route shape.

## Non-Goals

- No behavior change.
- No new quota policy.
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

Also run a temporary focused in-package smoke proving the moved helper
preserves:

- observed-at time;
- provider instance ID;
- credential ID;
- model ID;
- source;
- HTTP status;
- error class;
- retry-after pointer.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- the helper is behavior-equivalent;
- `isQuotaObservation` calls remain outside the helper and unchanged;
- chat and stream sources remain `"chat"` and `"stream"`;
- retry-after still comes from `provider.ChatResult.RetryAfter` and
  `provider.ChatStreamSummary.RetryAfter`;
- retry/fallback branches, health recording, metadata finalization, stream
  sink behavior, storage fallback behavior, management, TUI, config, and IO
  logging are unchanged.

## Acceptance

- Chat quota-observation construction lives in
  `request_metadata_quota.go`.
- Stream and non-stream quota row field values are unchanged.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass.

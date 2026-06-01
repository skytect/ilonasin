# 220 Server Quota Recording Split

## Context

`docs/ilonasin-architecture.md` treats quota observations as part of the
metadata-only observability side-plane. Quota rows must stay auditable without
storing prompts, completions, request bodies, response bodies, raw provider
payloads, raw stream chunks, tool arguments/results, full bearer tokens, full
provider request IDs, or full account IDs.

Plans 210 through 219 split request metadata construction into focused files.
`internal/server/metadata.go` still mixes generic metadata recording wrappers
with quota-specific recording behavior:

- `recordQuota`;
- `recordQuotaObservations`;
- `isQuotaObservation`.

The worktree currently contains unrelated uncommitted auth-retry changes in:

- `internal/server/chat_nonstream.go`;
- `internal/server/chat_stream.go`;
- `internal/server/credentials.go`.

This slice must not modify or stage those files.

## Goal

Move server quota-observation recording into a focused file without changing
which quota rows are recorded, how retry-after timing is normalized, or how
chat and stream callers detect quota conditions.

## Scope

1. Add `internal/server/metadata_quota.go`.
2. Move these helpers from `internal/server/metadata.go` into the new file:
   - `recordQuota`;
   - `recordQuotaObservations`;
   - `isQuotaObservation`.
3. Keep the generic metadata wrapper helpers in `metadata.go`:
   - `record`;
   - `recordWithID`;
   - `recordStream`;
   - `recordHealth`;
   - `recordFallbacks`.
4. Preserve exact quota recording behavior:
   - no-op when `s.meta == nil`;
   - no-op when request metadata ID is zero;
   - no-op unless status or error class is quota-related;
   - fill `ObservedAt` from `s.now()` when unset;
   - derive `ResetAt` from `RetryAfter.UTC()` when reset is absent;
   - call `RecordQuotaObservation` and ignore its error.
5. Preserve exact quota classification:
   - HTTP `429 Too Many Requests`;
   - HTTP `402 Payment Required`;
   - error class `rate_limit_exceeded`;
   - error class `insufficient_quota`.
6. Do not change non-stream chat, stream chat, credential refresh, retry
   policy, fallback recording, health recording, request metadata finalization,
   stream metrics, storage, management DTOs, TUI, config, IO logging, schema,
   or public routes.
7. Do not modify or stage unrelated dirty files.

## Non-Goals

- No behavior change.
- No new quota policy.
- No schema change.
- No new metadata fields.
- No permanent tests.
- No cleanup of concurrent auth-retry work.

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

Also run a temporary focused in-package smoke proving quota classification,
zero request ID no-op behavior, nil metadata-store no-op behavior, observed-at
defaulting, and retry-after to reset normalization are unchanged. Remove any
temporary smoke before commit.

During diff review, explicitly verify that:

- `metadata.go` retains only generic metadata recording wrappers;
- `metadata_quota.go` contains only quota-observation recording and
  classification helpers;
- call sites in chat and stream files remain package-scope callers and are not
  edited;
- no route, provider, storage, management, TUI, config, schema, IO logging, or
  unrelated dirty files changed.

## Acceptance

- Server quota-observation recording lives in `metadata_quota.go`.
- Generic metadata recording wrappers remain in `metadata.go`.
- Quota row gating, observed time defaulting, reset derivation, and quota
  classification are unchanged.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass, or any failure is proven unrelated to this slice.

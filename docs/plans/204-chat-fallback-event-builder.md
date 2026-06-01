# 204 Chat Fallback Event Builder

## Context

`docs/ilonasin-architecture.md` requires credential pooling and fallback to be
auditable through metadata-only fallback rows. The streaming and non-streaming
chat execution paths both construct identical `metadata.FallbackEvent` rows
inline after retry eligibility is known.

The duplicated row shape records occurrence time, provider instance, model,
source credential, target credential, retry reason, and policy allowance. This
is the fallback metadata side-plane and should be built in one local helper so
future metadata changes do not drift between streaming and non-streaming paths.

## Goal

Centralize chat fallback-event construction without changing retry/fallback
decisions, credential selection, adapter call order, health recording, quota
observations, request metadata, response behavior, stream behavior, IO logging,
storage, management, TUI, or public endpoints.

## Scope

1. Add a helper in `internal/server/chat_helpers.go` that constructs a
   `metadata.FallbackEvent` from:
   - occurrence time,
   - model address,
   - from credential,
   - to credential,
   - retry reason.
2. Replace the duplicated inline fallback-event literals in:
   - `executeNonStreamingChat`
   - `handleStreamingChat`
3. Preserve exact values:
   - `OccurredAt`
   - `ProviderInstanceID`
   - `ModelID`
   - `FromCredentialID`
   - `ToCredentialID`
   - `Reason`
   - `AllowedByPolicy: true`
4. Keep retry-reason calculation and `i == len(plan.attempts)-1` gating exactly
   where they are.
5. Do not change retry/fallback selection, credential refresh, provider health
   recording, quota observation generation, request metadata finalization,
   stream metrics, adapter calls, response writers, stream sink behavior, IO
   logging, storage, management, TUI, or config.

## Non-Goals

- No behavior change.
- No fallback policy change.
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

- occurrence time;
- provider instance ID;
- model ID;
- from credential ID;
- to credential ID;
- reason;
- `AllowedByPolicy: true`.

Remove the temporary smoke before commit.

During diff review, explicitly verify that:

- retry-reason calculation remains unchanged;
- fallback-event creation still happens only after non-empty retry reason and
  before moving to the next eligible credential;
- next credential selection remains `plan.attempts[i+1]`;
- retry counters, quota observations, health recording, metadata finalization,
  response behavior, stream sink behavior, and IO logging are unchanged.

## Acceptance

- Fallback-event construction is centralized.
- Stream and non-stream fallback row field values are unchanged.
- Compile, vet, serve smoke, manage smoke, focused helper smoke, and whitespace
  checks pass.

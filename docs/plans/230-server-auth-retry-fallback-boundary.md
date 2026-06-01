# 230 Server Auth Retry Fallback Boundary

## Context

`docs/ilonasin-architecture.md` requires credential pooling to remain
same-provider and same-model, auditable through metadata-only request, fallback,
health, and quota rows. It explicitly allows switching to another eligible
credential before a response is committed, and `docs/codex-auth.md` says 401
recovery should be implemented separately from 429 handling.

The current server already refreshes Codex OAuth credentials after upstream auth
failures. If the refreshed credential still fails with `upstream_auth_failed`,
that terminal auth failure is not classified through a focused helper and the
chat executors do not consistently carry the next credential as the model
credential for the following same-provider attempt. That weakens the credential
pooling boundary for auth recovery: the next credential can be selected for the
request credential while model-discovery credentials may still be stale for the
retry path.

The worktree currently contains an uncommitted server diff in:

- `internal/server/chat_nonstream.go`;
- `internal/server/chat_stream.go`;
- `internal/server/credentials.go`.

This slice will take ownership of those exact changes after review. It must not
modify unrelated files beyond the numbered plan unless verification exposes a
narrow bug in this same auth-retry boundary.

## Goal

Make exhausted post-refresh upstream auth failures participate in same-provider
credential fallback before a response is committed, while keeping auth retry
classification separate from quota and availability retry logic.

## Scope

1. Add focused auth-retry classification helpers in `internal/server/credentials.go`:
   - non-streaming: retryable only for local `502` with
     `upstream_auth_failed`;
   - streaming: retryable only before any stream has started or response has
     been committed, with `PreStreamError`, local `502`, and
     `upstream_auth_failed`.
2. In `executeNonStreamingChat`, use the auth helper before quota and
   availability helpers, and record fallback reason `auth_retry`.
3. In `executeStreamingChat`, use the stream auth helper before quota and
   availability helpers, and record fallback reason `auth_retry`.
4. When an auth fallback chooses the next same-provider credential, update the
   model credential to that next credential so the next attempt's request and
   model-discovery credentials stay aligned.
5. Preserve existing behavior for:
   - one in-place OAuth refresh retry on initial upstream 401;
   - quota retry classification and quota observations;
   - availability retry classification;
   - no fallback after streaming has started;
   - request metadata, health rows, fallback rows, auth retry counts, attempt
     counts, response writers, and route shapes.
6. Do not change provider adapters, routing policy, management DTOs, storage,
   TUI, config, IO logging, or public APIs.
7. Do not add permanent tests.

## Non-Goals

- No new cross-provider fallback.
- No cross-model fallback.
- No retry-loop redesign.
- No changes to subscription quota keepalive or usage.
- No Anthropic or Responses route changes.

## Verification

Run a temporary focused in-package smoke test, then remove it before commit. It
must prove:

- non-streaming post-refresh `upstream_auth_failed` produces one `auth_retry`
  fallback to the next credential;
- the second non-streaming attempt uses the next credential as both request and
  model credential;
- streaming post-refresh `upstream_auth_failed` before stream start produces one
  `auth_retry` fallback to the next credential;
- streaming auth failure after stream start does not fallback;
- quota retry and availability retry behavior are not reclassified as
  `auth_retry`.

Then run:

```sh
if rg 'provider adapter is not implemented' internal/server -n; then
  echo 'old provider adapter message remains in live server code' >&2
  exit 1
fi
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
cat >"$tmp/config.toml" <<EOF2
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
EOF2
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
timeout 4s script -q -e -c "stty cols 140 rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage.out" || true
rg "api|providers|usage|logs" "$tmp/manage.out" >/dev/null
```

During diff review, explicitly verify:

- auth retry classification is separate from quota and availability helpers;
- stream auth retry is gated on no committed stream response;
- fallback events keep reason `auth_retry`;
- model credential alignment changes only on auth fallback;
- no Responses, Anthropic, provider adapter, TUI, storage, config, or
  management files changed.

## Acceptance

- Auth retry fallback is explicit and auditable as `auth_retry`.
- Same-provider credential fallback keeps request and model credentials aligned.
- Streaming never falls back after response commitment.
- Existing quota and availability retry semantics are preserved.
- Focused smoke, compile, vet, serve smoke, manage smoke, whitespace checks, and
  three implementation reviews pass.

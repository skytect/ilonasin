# Plan 193: Provider Unimplemented Error Language

## Context

`docs/ilonasin-architecture.md` describes a production local daemon with clear
OpenAI-compatible request errors and clean provider-routing boundaries.

The live chat, responses, and Anthropic message routes still return the
implementation-era phrase `provider credential type is not implemented in this
slice` when a configured provider cannot serve a request because chat,
credential style, or placeholder support is unavailable. That phrase exposes
plan history through the API and is no longer accurate production language.

## Scope

Normalize the live API error text for unsupported provider capabilities while
preserving all existing status codes, error classes, metadata recording, logging
event names, routing behavior, credential resolution, and adapter selection.

## Plan

1. Add a server-local constant for the unsupported provider capability message.
2. Replace the stale `in this slice` message in:
   - `internal/server/chat_route.go`
   - `internal/server/responses_route.go`
   - `internal/server/anthropic_route.go`
3. Keep existing `provider_unimplemented` classes and HTTP 501 status codes.
4. Do not change provider registry, credential service, adapters, request
   validation, logging, or management/TUI code.
5. Add a temporary in-package route smoke for a configured provider instance
   whose capabilities intentionally fail the `provider_unimplemented` branch.
   Assert the live handler response no longer contains `in this slice` while
   still returning HTTP 501 and `provider_unimplemented`.
6. Add source checks that no live `internal/` code returns `in this slice`.

## Verification

Run:

```sh
if rg -n 'in this slice' internal cmd; then
  exit 1
fi
go test ./...
go vet ./...
git diff --check
tmp=$(mktemp -d)
tmpbin="$tmp/bin"
mkdir -p "$tmpbin"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
port=$(python - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
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
token_json="$(curl --silent --fail --unix-socket "$sock" \
  --header 'Content-Type: application/json' \
  --data '{"label":"smoke"}' \
  http://ilonasin/_ilonasin/manage/local-tokens)"
token="$(printf '%s' "$token_json" | jq -r '.token')"
token_id="$(printf '%s' "$token_json" | jq -r '.metadata.id')"
curl --silent --show-error --fail-with-body \
  --header "Authorization: Bearer $token" \
  --header 'Content-Type: application/json' \
  --data '{"model":"codex/gpt-5.5","messages":[{"role":"user","content":"hi"}]}' \
  "http://127.0.0.1:$port/v1/chat/completions" >/dev/null || true
curl --silent --fail --unix-socket "$sock" \
  --header 'Content-Type: application/json' \
  --data "{\"id\":$token_id}" \
  http://ilonasin/_ilonasin/manage/local-tokens/disable >/dev/null
timeout 3s script -q -e -c "stty cols 120 rows 40; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null || true
```

Also run a temporary `internal/server` in-package route smoke, then remove it
before commit. The smoke should construct a `Server` with a fake
`ProviderRegistry` containing an instance with `Chat=false`, call
`handleChatCompletions` through `Handler()` with a verified fake local token,
and assert:

- status is `501`,
- `.error.code` is `provider_unimplemented`,
- `.error.message` does not contain `in this slice`.

## Non-Goals

- No new provider support.
- No TUI changes.
- No DTO, storage, config, or auth changes.
- No permanent tests.

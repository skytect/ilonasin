# 202 Chat Request Builder

## Context

`docs/ilonasin-architecture.md` keeps routing, credential resolution, provider
adapter calls, and metadata recording as separate boundaries. The streaming and
non-streaming chat execution paths currently repeat the same
`provider.ChatRequest` struct literal in every initial attempt and OAuth retry
attempt.

That repetition is small, but it is exactly the provider-adapter boundary:
configured provider instance, resolved upstream model, normalized request,
selected bearer credential, and model-discovery credential. Keeping the shape in
one helper reduces the chance that a future retry path accidentally omits one
credential field or changes the model/credential pairing differently between
streaming and non-streaming paths.

## Goal

Centralize construction of provider chat requests without changing execution
behavior, retry/fallback behavior, credential refresh behavior, response wire
shape, IO logging, or metadata.

## Scope

1. Add a helper in `internal/server/chat_helpers.go` such as
   `providerChatRequest(instance, addr, req, credential, modelCredential)`.
2. Replace the repeated `provider.ChatRequest{...}` literals in:
   - `executeNonStreamingChat`
   - `handleStreamingChat`
3. Preserve exact values passed to adapters:
   - `Instance`
   - `UpstreamModel`
   - `Request`
   - `Credential` converted through `providerChatCredential`
   - `ModelCredential`
4. Do not change auth refresh conditions, credential replacement logic,
   attempt counters, retry/fallback decisions, quota observation generation,
   health recording, response writing, stream sink behavior, IO logging,
   metadata recording, storage, management, TUI, or public endpoints.

## Non-Goals

- No behavior change.
- No execution-loop unification.
- No new provider feature.
- No storage, config, or management change.
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

- provider instance;
- upstream model from `routing.ModelAddress.ProviderModelID`;
- OpenAI chat request value;
- converted chat credential fields from the selected bearer credential;
- model credential identity and bearer data.

Remove the temporary smoke before commit.

During diff review, explicitly verify that:

- both execution loops still call adapters at the same points;
- auth refresh branches still mutate `credential` and `modelCredential` in the
  same order;
- attempt counters and fallback decisions are unchanged;
- no response writer, stream sink, IO logging, health, quota, metadata, storage,
  management, or TUI code changed.

## Acceptance

- Provider chat request construction is centralized.
- Streaming and non-streaming adapter call semantics are unchanged.
- Compile, vet, serve smoke, manage smoke, focused helper smoke, and whitespace
  checks pass.

# 217 Request Metadata Endpoint Split

## Context

`docs/ilonasin-architecture.md` treats the request metadata ledger as a core
side-plane for endpoint, routing, usage, latency, retry, fallback, and
credential metadata. Endpoint labels are safe metadata values and are used by
Chat Completions, Responses, Anthropic Messages compatibility, management
snapshot sanitization, and TUI display sanitization.

Plans 210 through 216 split request metadata option sanitization, image
counting, throughput math, quota observations, token-limit extraction,
sanitization, and finalization into focused files. `request_metadata.go` now
contains metadata endpoint constants and the two base constructors. Endpoint
labels are a distinct metadata vocabulary concern and can be isolated without
changing behavior.

The worktree currently contains unrelated uncommitted changes in
`internal/server/chat_nonstream.go`, `internal/server/chat_stream.go`, and
`internal/server/credentials.go`. This slice must not modify or stage those
files.

## Goal

Move request metadata endpoint label constants into a focused file without
changing any endpoint string, call site, route behavior, storage behavior,
management output, or TUI output.

## Scope

1. Add `internal/server/request_metadata_endpoints.go`.
2. Move these constants from `request_metadata.go` into the new file:
   - `metadataEndpointChatCompletions = "chat_completions"`;
   - `metadataEndpointResponses = "responses"`;
   - `metadataEndpointAnthropicMessages = "anthropic_messages"`.
3. Preserve exact string values.
4. Keep all call sites unchanged in chat, Responses, and Anthropic route code.
5. Do not change metadata base construction, finalization, option
   sanitization, image counting, token-limit extraction, quota observations,
   throughput math, route handlers, provider adapters, storage, management,
   TUI, config, IO logging, schema, or public route shape.
6. Do not modify or stage unrelated dirty files.

## Non-Goals

- No behavior change.
- No schema change.
- No endpoint renaming.
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

Also run a temporary focused in-package smoke proving the three constants keep
their exact string values and that `responsesRequestMetadataBase` still records
`metadataEndpointResponses`.

Remove any temporary smoke before commit.

During diff review, explicitly verify that:

- moved constants are behavior-equivalent;
- all `metadataEndpoint*` call sites still compile and use the same names;
- management/TUI display sanitizers remain compatible with the unchanged
  string values;
- no route, provider, storage, management, TUI, config, schema, IO logging, or
  unrelated dirty files changed.

## Acceptance

- Request metadata endpoint labels live in
  `request_metadata_endpoints.go`.
- Endpoint string values and metadata outputs are unchanged.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass, or any failure is proven to come from unrelated pre-existing dirty
  work.

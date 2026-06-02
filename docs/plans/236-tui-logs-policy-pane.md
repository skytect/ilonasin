# 236 TUI Logs Policy Pane

## Context

`docs/ilonasin-architecture.md` treats observability as metadata-first and
privacy-sensitive. The TUI top-level `logs` section already has request
metadata, fallback metadata, and retention/IO policy content, but the third pane
is titled `retention` and the empty states still spend rows on prose such as
`No request metadata.` and `No fallback metadata.`.

The user wants the control plane to be compact and visual, with logs organized
as metadata plus IO visibility rather than a long text page.

## Goal

Make the logs section clearly present metadata and IO policy as first-class
compact status surfaces, without changing logging, storage, server behavior, or
management DTOs.

## Scope

1. Keep the top-level `logs` tab and the existing pane-local scrolling model.
2. Rename the third logs pane from `retention` to a metadata/IO policy title
   while keeping the `logsPanePruning` ID and pane order unchanged.
3. Keep requests and fallbacks as repeated cards when rows exist.
4. Replace request and fallback empty-state prose with compact status cards that
   report zero rows and reinforce metadata-only visibility.
5. Update the policy pane banner/cards so it reads as:
   - metadata ledger;
   - IO capture policy;
   - retention/pruning.
6. Preserve the existing `capture_io` display state. Do not change IO logging
   enablement, file writing, scrubbing, retention, pruning behavior, or config.
7. Do not add management DTOs, routes, schema changes, provider changes, server
   changes, or permanent tests.
8. Do not touch unrelated concurrent work.

## Non-Goals

- No IO log browser.
- No change to `capture_io` semantics.
- No server-side logging changes.
- No new management endpoint.
- No new Bubble Tea dependency.

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
curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null
for cols in 80 120 160; do
  timeout 4s script -q -e -c "stty cols $cols rows 32; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage-$cols.out" || true
  rg "api|providers|usage|logs" "$tmp/manage-$cols.out" >/dev/null
done
```

Also run a temporary focused render smoke, then remove it before commit. It
should render the logs tab at 80, 120, and 160 columns and assert:

- the third logs pane title contains metadata/IO policy language, not only
  retention;
- empty request/fallback states render as cards or chips, not `No request
  metadata.` or `No fallback metadata.`;
- the logs view includes metadata ledger, IO capture policy, and pruning
  signals;
- seeded unsafe content markers such as `raw_payload_secret`,
  `request_body_secret`, `response_body_secret`, `sse_chunk_secret`,
  `tool_argument_secret`, and `tool_result_secret` do not appear in the logs
  render. Do not fail on safe metadata labels such as `chat_completions`.

## Acceptance

- Logs reads as metadata plus IO policy, not a prose-heavy retention page.
- Empty logs states are compact visual status surfaces.
- No logging/storage/server/config/provider behavior changes.
- Compile, vet, serve/manage smoke, focused render smoke, whitespace checks,
  and implementation review pass.

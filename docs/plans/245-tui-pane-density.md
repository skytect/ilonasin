# 245 TUI Pane Density

## Context

The TUI already has the right top-level control-plane sections: `api`,
`providers`, `usage`, and `logs`. It also already has adaptive columns and
pane-local scrolling. The remaining issue is information density and visual
shape inside those panes.

The user wants less overview-style text, less whole-screen scrolling, visible
account identity labels, and a clearer split between local API management,
upstream provider management, usage/quota/performance, and metadata/IO logs.

## Goal

Make the existing section panes more compact and scannable without changing the
daemon API, storage, routing, provider behavior, logging policy, or management
DTOs.

## Scope

1. Keep the four top-level tabs exactly as `api`, `providers`, `usage`, and
   `logs`.
2. Keep the existing adaptive pane layout and pane-local scrolling model.
3. Reduce prose in the API summary pane:
   - keep local surfaces visible: Chat Completions, Responses, Anthropic
     Messages, and Anthropic count tokens;
   - keep downstream local token counts visible;
   - avoid repeating upstream provider explanations in the API pane.
4. Make provider instance rendering denser:
   - use compact rows for configured provider instances instead of larger
     repeated cards;
   - keep auth capabilities, route capability, discovery state, and base URL
     visible.
5. Make request metadata rendering denser:
   - use compact repeated rows for recent requests;
   - retain endpoint, status, time, model, credential, fallback, token, cache,
     and latency cues;
   - token cues means usage counts only: prompt, completion, total, reasoning,
     and cache token counts. Do not render local token fragments or token-like
     identifiers in request metadata rows;
   - keep unsafe data redacted by using existing display helpers.
6. Keep cards and bars where they add value:
   - subscription account and pool windows;
   - token mix;
   - cache and reasoning rates;
   - latency/performance aggregates;
   - metadata/IO policy state.
7. Preserve visible safe email/display labels in OAuth/account panes by not
   changing their existing account identity helpers.
8. Add only small TUI rendering helpers where they reduce duplicated row
   formatting.
9. Do not add new management fields, management routes, database schema, config
   mutation, route behavior, provider behavior, subscription keepalive behavior,
   or permanent tests.

## Non-Goals

- No new Bubble Tea or Bubbles dependency.
- No rewrite to Bubbles `viewport`.
- No new navigation model.
- No Anthropic route work.
- No usage or quota math changes.
- No log browsing for raw IO.

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
  rg "Chat Completions|Responses|Anthropic Messages|count_tokens" "$tmp/manage-$cols.out" >/dev/null
  rg "downstream|local tokens|upstream|OAuth|fallback|metadata|IO" "$tmp/manage-$cols.out" >/dev/null
done
```

Also run a temporary focused render smoke and remove it before commit. It should
construct a `Model` with seeded provider instances, request metadata, safe
email/display labels, and unsafe marker strings, render narrow and wide views,
and assert:

- provider instances are compact rows, not full cards;
- request metadata is compact rows, not full cards;
- compact provider rows include provider ID, type, auth capability, discovery
  state, route state, and base URL;
- compact request rows include endpoint, status, relative time, model,
  credential display, fallback count, usage token counts, cache rate, and
  latency;
- local token fragments do not appear in request metadata rows;
- usage visuals still show bars where they matter;
- section panes still scroll independently;
- unsafe raw content, full tokens, request IDs, raw provider account IDs,
  prompts, completions, request bodies, response bodies, raw payloads, raw
  streams, tool arguments, and tool results are not rendered. Safe display
  labels and email identities remain visible where the management snapshot
  already exposes them.

## Acceptance

- The TUI keeps the four requested sections.
- The UI uses screen-sized independently scrollable panes.
- API is local route and downstream token focused.
- Providers is upstream provider, key, OAuth account, and fallback focused.
- Usage keeps token, quota, cache, and performance visuals.
- Logs becomes denser metadata/IO visibility rather than card-heavy prose.
- Compile, vet, build, serve smoke, and manage wide/narrow smoke pass.

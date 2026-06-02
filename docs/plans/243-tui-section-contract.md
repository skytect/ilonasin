# 243 TUI Section Contract

## Context

The TUI already has the intended top-level sections:

- `api`;
- `providers`;
- `usage`;
- `logs`.

It also has pane-local scrolling and adaptive pane columns. The remaining issue
is that some panes still communicate the old mixed overview shape through
generic pane names and prose-heavy summary cards. The user wants the control
plane organized around:

- API surfaces and downstream local key management;
- upstream provider key/OAuth/fallback management;
- usage, quota, cache, token, and performance visuals;
- metadata and IO logging state.

This slice should tighten the section contract in the TUI without adding new
management DTOs or changing daemon behavior.

## Goal

Make the existing four TUI sections read as explicit screen-sized control-plane
areas with compact pane titles and denser summary content.

## Scope

1. Keep top-level tabs as `api`, `providers`, `usage`, and `logs`.
2. Keep pane-local scrolling and adaptive columns unchanged.
3. Rename pane titles so each section communicates its ownership:
   - API: local APIs and downstream keys;
   - providers: upstream providers, upstream keys, OAuth accounts, fallback
     config;
   - usage: token/performance, subscription quota, health/quota;
   - logs: request metadata, fallback metadata, metadata/IO policy.
4. Split provider fallback config out of the upstream API-key pane into its own
   provider pane. Keep provider actions tab-scoped and reachable from any
   providers pane.
5. Rework the API summary pane into a compact route matrix for:
   - Chat Completions;
   - Responses;
   - Anthropic Messages, including count-token capability.
6. Keep downstream local-token counts visible in the API section and keep
   upstream key/OAuth language out of API actions except as a short boundary.
7. Add a compact providers summary row that distinguishes config-defined
   provider instances, upstream API keys, OAuth accounts, and fallback groups.
8. Prefer chips, status badges, and short rows over explanatory prose. Do not
   turn every item into a larger card.
9. Preserve existing TUI actions, key routing, selection behavior, scroll
   behavior, redaction, and sanitizer boundaries.
10. Do not change management DTOs, management routes, server routes, provider
   adapters, storage, config mutation behavior, logging policy, subscription
   keepalive, or public API behavior.
11. Do not add permanent tests.

## Non-Goals

- No new data in the management snapshot.
- No new Bubble Tea dependency.
- No usage or subscription math changes.
- No new route support.
- No IO-log browser.
- No broad visual redesign of account, usage, quota, or request cards.

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
  rg "downstream|local tokens|upstream|OAuth|fallback" "$tmp/manage-$cols.out" >/dev/null
done
```

Also run a temporary focused render smoke and remove it before commit. It should
render API, providers, usage, and logs at narrow and wide widths and assert:

- API route names are visible;
- API local-token counts are visible;
- provider summary row distinguishes instances, upstream keys, OAuth accounts,
  and fallback groups;
- fallback config renders in a separate provider pane from upstream API keys;
- old top-level labels `overview` and `observability` are absent, and
  `accounts` appears only as provider/OAuth/subscription account content, not
  as a top-level section;
- unsafe markers such as bearer tokens, full local tokens, raw payload/body
  labels, prompts, completions, tool arguments, tool results, and request IDs
  are absent.

## Acceptance

- The TUI still uses the four top-level sections and pane-local scrolling.
- API visibly owns local API surfaces and downstream tokens.
- Providers visibly owns upstream providers, keys, OAuth accounts, and fallback.
- Usage and logs keep their current visual content and pane-local scrolling.
- No daemon API, storage, provider, route, config, logging, or subscription
  behavior changes.
- Compile, vet, whitespace, serve/manage smoke, focused render smoke, and
  implementation review pass.

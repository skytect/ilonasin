# 223 TUI Control Plane Tabs

## Context

`docs/ilonasin-architecture.md` defines `ilonasin manage` as a first-class
local control plane for provider instances, credentials/accounts, OAuth/API-key
flows, usage, health, metadata-only requests, latency, fallback events, and
local API token management.

The current TUI still exposes the older tabs:

- `overview`;
- `accounts`;
- `observability`;
- `help`.

That shape forces unrelated control-plane concerns into long mixed pages. The
next target discussed for the TUI is a clearer operational grouping:

- `api`: public API surfaces and downstream local token management;
- `providers`: upstream keys, OAuth accounts, provider instances, model cache,
  and fallback configuration;
- `usage`: token usage, subscription quota, health/quota, and performance;
- `logs`: recent metadata rows, fallback events, telemetry pruning, and later
  IO-log visibility.

This slice is the foundation for later pane-local scrolling and denser visual
work. It should rebucket existing render blocks and key routing without trying
to redesign every component at once.

The worktree currently contains unrelated uncommitted auth-retry changes in:

- `internal/server/chat_nonstream.go`;
- `internal/server/chat_stream.go`;
- `internal/server/credentials.go`.

This slice must not modify or stage those files.

## Goal

Replace the old TUI tab model with API, providers, usage, and logs sections
while preserving existing management operations and metadata-only privacy
behavior.

## Scope

1. Replace `tabOverview`, `tabAccounts`, `tabObservability`, and `tabHelp`
   with:
   - `tabAPI`;
   - `tabProviders`;
   - `tabUsage`;
   - `tabLogs`.
2. Update tab labels to:
   - `api`;
   - `providers`;
   - `usage`;
   - `logs`.
3. Add or update section renderers:
   - API renders daemon bind/API surface summary and `writeLocalTokens`;
   - providers renders provider instances, model cache, upstream credentials,
     fallback policies, and OAuth/provider accounts;
   - usage renders usage metrics, health/quota, and subscription usage;
   - logs renders recent requests, fallback events, and pruning status.
4. Preserve existing render helper functions where practical. This is a
   rebucketing slice, not a visual rewrite.
5. Update key routing:
   - `1` through `4` jump to API, providers, usage, logs;
   - local token actions (`n`, `d`) work on API;
   - up/down local-token selection moves to API;
   - upstream/API-key/OAuth/fallback actions (`a`, `x`, `l`, `o`, `r`, `f`,
     `F`) work on providers;
   - subscription refresh (`u`) works on usage;
   - prune (`p`) works on logs.
6. Update footer/help/status text to match the new sections, including
   transient token/OAuth messages.
7. Keep the TUI daemon-backed. Do not introduce direct `config.toml` mutation
   or new direct SQLite mutation.
8. Do not change management DTOs, management routes, storage, provider
   adapters, server routes, config, IO logging policy, schema, or public API
   behavior.
9. Do not modify or stage unrelated dirty files.

## Non-Goals

- No pane-local scrolling yet.
- No full visual redesign.
- No new management API endpoints.
- No IO-log browser yet.
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

Temporary focused smoke:

- render the model at width 140 and assert the tab bar contains `api`,
  `providers`, `usage`, and `logs`;
- select each tab and assert the expected section heading appears;
- assert stale labels `overview`, `accounts`, and `observability` do not appear
  in the tab bar;
- exercise key routing enough to prove `1` through `4` select the new tabs.
- exercise key gating enough to prove `n`/`d` apply only on API,
  `a`/`x`/`l`/`o`/`r`/`f`/`F` apply only on providers, `u` applies only on
  usage, and `p` applies only on logs.
- prove up/down on API changes local-token selection.
- assert `help` is no longer a top-level tab and equivalent key guidance
  remains visible in footer or rendered guidance text.
- assert stale labels `accounts`, `overview`, and `observability` are absent
  from tab bar, footer, help/guidance text, and status text.

Remove temporary smoke files before commit.

## Acceptance

- The TUI top-level sections are API, providers, usage, and logs.
- Existing local token, upstream credential, OAuth, fallback, usage refresh,
  and pruning actions remain reachable from the appropriate new sections.
- Diff review confirms `validActiveTab`, `clampScrolls`, `scrollOffsets`,
  `downAction`, and `upAction` are updated for the new tab set and provider
  selection behavior.
- The first screen is closer to the target control-plane information
  architecture without introducing new storage, config, provider, or route
  behavior.
- Focused smoke, compile, vet, serve smoke, manage smoke, and whitespace checks
  pass.

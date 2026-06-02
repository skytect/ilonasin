# 240 TUI Provider API Density

## Context

`docs/ilonasin-architecture.md` treats `ilonasin manage` as a first-class
Bubble Tea/Lipgloss control plane. Recent slices already moved the TUI to the
right top-level sections:

- `api`: exposed local API surfaces and downstream client token management;
- `providers`: upstream provider instances, API keys, OAuth accounts, provider
  account identities, and fallback configuration;
- `usage`: token usage, subscription quota, health, quota, and performance;
- `logs`: metadata rows, fallback events, and metadata/IO policy.

The active pane renderer already uses screen-sized panes with pane-local
scrolling. Usage is also relatively visual now: token mix bars, cache/reasoning
gauges, subscription bars, and empty-state cards are present. The remaining
visible issue in this slice is that API and provider panes still fall back to
plain prose such as `No local API tokens.`, `No provider instances.`, `No
upstream credentials.`, `No OAuth accounts.`, and `No provider accounts.`.

This slice should make those panes denser and more visual while preserving the
current section model and existing management operations.

## Goal

Replace API/provider prose empty states with compact status cards and tighten
identity/provider rows so the dashboard reads as operational state rather than a
text report.

## Scope

1. Keep the top-level sections as `api`, `providers`, `usage`, and `logs`.
2. Keep the current pane-local scrolling model and adaptive pane layout.
3. In the API local-token pane:
   - replace `No local API tokens.` with a compact status card;
   - keep token creation/disable actions and selection behavior unchanged;
   - keep safe token fragments only, never full local tokens.
4. In provider instance and upstream credential panes:
   - replace prose empty states with compact status cards;
   - keep existing provider cards, API-key cards, model-cache rendering, and
     fallback rendering behavior otherwise unchanged;
   - include model-cache and credential-group empty states because they render
     inside the same provider panes;
   - keep downstream local API tokens visibly separate from upstream provider
     credentials.
5. In OAuth/provider account panes:
   - replace prose empty states with compact status cards;
   - keep email/display labels visible when the management snapshot exposes safe
     labels;
   - keep unsafe account-like labels redacted through existing display helpers.
6. Use existing visual helpers such as `renderEmptyMetricCard`,
   `renderMetricAccentCard`, `metricLine`, `metricChip`, `statusBadge`, and
   time/identity helpers where practical.
7. Preserve metadata-only privacy rules. Do not render prompts, completions,
   request bodies, response bodies, raw provider payloads, raw SSE chunks, tool
   arguments/results, full bearer tokens, full provider request IDs, full
   account IDs, local client tokens, upstream API keys, OAuth tokens, cookies,
   authorization codes, device codes, or code verifiers.
8. Do not change management DTOs, management routes, storage, provider
   adapters, server routes, config, logging behavior, subscription keepalive,
   pane navigation, or public API behavior.
9. Do not add permanent tests.

## Non-Goals

- No new TUI tab model.
- No new pane layout engine.
- No usage chart redesign in this slice.
- No IO-log browser.
- No management API additions.
- No server-side credential or provider behavior changes.

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
should seed a `Model` with empty and populated API/provider rows and assert:

- the old prose strings are absent:
  - `No local API tokens.`;
  - `No provider instances.`;
  - `No upstream credentials.`;
  - `No cached models.`;
  - `No credential group metadata.`;
  - `No OAuth accounts.`;
  - `No provider accounts.`;
- empty API/provider states render compact status cards or chips;
- all four top-level sections still render;
- safe account email/display labels remain visible when seeded;
- unsafe labels containing token, `acct_`/`acct-`, raw payload,
  prompt/completion body, request ID, SSE chunk, tool argument, or tool result
  markers are redacted;
- rendered output at 80, 120, and 160 columns has no line wider than the view
  after stripping ANSI escapes.

Remove temporary smoke files and artifacts before commit.

## Acceptance

- API and provider panes no longer use prose empty-state rows.
- Empty and populated API/provider states render as compact visual dashboard
  elements.
- OAuth and provider account emails/display labels remain visible when safe.
- Downstream local tokens and upstream provider credentials remain clearly
  separated.
- No management, storage, server, provider, config, logging, subscription,
  public API, or navigation behavior changes.
- Compile, vet, whitespace checks, serve/manage smoke, focused render smoke,
  and implementation reviews pass.

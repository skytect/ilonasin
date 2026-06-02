# 233 TUI Adaptive Pane Layout

## Context

`docs/ilonasin-architecture.md` treats `ilonasin manage` as a first-class
Bubble Tea/Lipgloss control plane. Recent TUI slices already moved the top-level
sections to `api`, `providers`, `usage`, and `logs`, and added pane-local
scrolling. The remaining visible issue is layout density on wide terminals:
three-pane sections still render as a left column with two stacked panes and a
right column with one tall pane. That wastes horizontal space, creates avoidable
pane scrolling, and makes the UI look more like stacked text than a control
plane.

The current TUI should keep its existing content organization:

- `api`: local surfaces and downstream token management;
- `providers`: provider instances, upstream keys, OAuth accounts, fallback;
- `usage`: token usage/quota, subscription limits, performance, health;
- `logs`: metadata rows, fallback events, retention.

This slice should improve the pane layout engine only. It should not change
management DTOs, server behavior, storage, provider routing, Anthropic support,
subscription keepalive, IO logging policy, or config mutation behavior.

## Goal

Use available horizontal space more effectively by rendering dashboard panes in
adaptive columns, while preserving pane-local scrolling and the existing TUI
content.

## Scope

1. Replace the fixed two-column pane split with an adaptive column planner:
   - one column for narrow terminals;
   - two columns for medium terminals;
   - up to three columns for wide terminals when the active section has at least
     three panes and each pane can keep a useful minimum width.
2. Keep pane-local scrolling as the only scroll model.
3. Make the scroll placement calculation use the same adaptive planner as the
   renderer, so focused-pane scrolling stays accurate.
4. Compute scroll max and content height from the same placement-specific inner
   width that `renderPane` uses. Width-aware content such as card grids and
   gauges must not be rendered at a generic body width for scroll math.
5. Keep pane chrome compact:
   - pane title remains one line;
   - focused pane remains visually obvious;
   - scroll marker remains bounded and does not push content wider than the
     pane.
6. Keep existing section content and actions unchanged.
7. Preserve management API boundaries:
   - no direct SQLite writes added;
   - no `config.toml` mutation;
   - no new routes or DTO fields.
8. Preserve privacy boundaries:
   - no prompts, completions, request bodies, response bodies, raw stream
     chunks, tool payloads, full tokens, full account IDs, or provider request
     IDs in TUI output.
9. Do not modify or stage concurrent plan `300` work.
10. Do not add permanent tests.

## Non-Goals

- No new TUI dependency.
- No content redesign inside usage/account cards.
- No subscription pool math changes.
- No route, provider, auth, storage, logging, or management DTO changes.
- No permanent test files.

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
for cols in 80 120 180; do
  timeout 4s script -q -e -c "stty cols $cols rows 34; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null >"$tmp/manage-$cols.out" || true
  rg "api|providers|usage|logs" "$tmp/manage-$cols.out" >/dev/null
done
```

Also run a temporary focused TUI render smoke and remove it before commit. It
should render seeded models at widths around 80, 120, and 180 columns and assert:

- 80 columns uses one pane column;
- 120 columns uses two pane columns;
- 180 columns uses three pane columns for three-pane sections;
- pane placement used for scroll math matches the rendered column count;
- scroll max/content height are computed from the placement-specific inner width
  used by the rendered pane at each tested width;
- no rendered line exceeds the requested width after stripping terminal
  control sequences;
- safe account email/display labels still render where seeded;
- unsafe markers such as `bearer`, `sk-`, `iln_`, `access_token`,
  `refresh_token`, `id_token`, `raw`, `payload`, `prompt body`,
  `completion body`, `request_id`, `tool argument`, and `tool result` do not
  appear.

## Acceptance

- Wide `manage` views use horizontal space more effectively with up to three
  dashboard pane columns.
- Pane-local scroll behavior remains accurate at narrow, medium, and wide
  widths.
- Existing actions and section content remain unchanged.
- Compile, vet, whitespace, serve smoke, manage smoke, focused render smoke,
  and senior implementation reviews pass.

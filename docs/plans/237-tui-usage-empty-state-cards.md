# 237 TUI Usage Empty State Cards

## Context

The usage tab is meant to summarize token usage, subscription quota, health, and
performance in compact panes. Several empty states still render as prose:

- `No usage metadata.`
- `No latency metadata.`
- `No stream metadata.`
- `No subscription usage snapshots.`
- `No health metadata.`
- `No quota metadata.`

That keeps the tab feeling like a text report instead of an operational control
surface. The logs tab already moved empty states to compact status cards; usage
should follow that direction.

## Goal

Replace usage-tab prose empty states with compact visual status cards while
preserving all existing usage, quota, subscription, health, and performance
behavior.

## Scope

1. Keep the existing `usage` tab and pane layout unchanged.
2. Replace empty-state prose in `usage_metrics.go`, `usage_health.go`, and
   `usage_subscription.go` with compact cards using existing TUI helpers.
3. Preserve existing cards when rows exist.
4. Show zero counts and meaningful metadata-only state in each empty card:
   - token usage;
   - latency/performance;
   - streams;
   - subscription account snapshots;
   - health events;
   - quota blocks.
5. Keep subscription pools and keepalive rendering unchanged except where the
   account snapshot empty state needs a compact visual.
6. Do not change management DTOs, storage, server routes, provider behavior,
   config, subscription keepalive behavior, IO logging, or schema.
7. Do not add permanent tests.
8. Do not touch unrelated concurrent work.

## Non-Goals

- No new usage calculations.
- No server-side subscription changes.
- No changes to quota aggregation or reset semantics.
- No new Bubble Tea dependency.
- No broad TUI redesign.

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
should render the usage tab at 80, 120, and 160 columns and assert:

- the six old empty-state prose strings are absent;
- compact cards for token usage, performance, streams, subscription accounts,
  health, and quota are visible;
- existing usage pane IDs and order remain unchanged;
- unsafe fake values seeded into usage, health, quota, or subscription labels
  are redacted by existing display helpers.

## Acceptance

- The usage tab empty states are visual and compact.
- Existing populated usage, subscription, health, quota, latency, and stream
  row rendering remains intact.
- No server, config, storage, provider, management DTO, logging, schema, or
  keepalive behavior changes.
- Compile, vet, serve/manage smoke, focused render smoke, whitespace checks,
  and implementation review pass.

# 182 TUI Usage Gauge Boundaries

## Context

The subscription dashboard work made `internal/tui/visual_helpers.go` grow into
a mixed bag of identity formatting, metric chips, progress bars, risk labels,
card layout, and Lipgloss styles. It also rendered each account window with
separate used and remaining bars, even though they describe the same 100%
window, and rendered pool average-used bars even though the pool card should
show aggregate remaining capacity only.

The architecture treats the TUI as a first-class Bubble Tea/Lipgloss surface,
but the implementation is easier to maintain when visual primitives have clear
local boundaries and the rendered gauges map cleanly to the data being shown.
This slice follows the existing TUI render split pattern from plans 143 through
157 while making the small gauge semantics correction requested by the user.

## Goal

Split TUI visual helper primitives into focused files and simplify subscription
usage gauges so each window has one bar.

After this slice:

- identity and account display helpers live together,
- metric chips and account metadata text helpers live together,
- usage gauges, bars, percentages, and risk labels live together,
- card and section layout helpers live together,
- Lipgloss style definitions live together,
- `visual_helpers.go` no longer mixes unrelated helper families,
- account window cards render one usage bar with used and left percentages in
  the header,
- pool window cards render only total remaining capacity and one pool remaining
  bar,
- subscription cards are more compact and reset/observed timestamps render in
  the system time zone with human-readable labels.

## Scope

1. Move style variables from `internal/tui/visual_helpers.go` to
   `internal/tui/visual_styles.go`.
2. Move account identity helpers to `internal/tui/visual_identity.go`.
3. Move account metadata, status badge, and metric chip helpers to
   `internal/tui/visual_text.go`.
4. Move percentage, bar, gauge, and risk helpers to
   `internal/tui/visual_gauges.go`.
5. Move card, section banner, and grid layout helpers to
   `internal/tui/visual_cards.go`.
6. Change account gauges to remove the redundant remaining bar.
7. Change pool gauges to remove average-used rendering from pool cards.
8. Compact subscription account cards by removing redundant metadata and reset
   text.
9. Change TUI time display helpers to render local-time, human-readable
   timestamps.
10. Keep all helpers package-private and preserve existing call sites where the
   semantics still match.
11. Do not change management DTOs, storage, provider adapters, routes, config,
   key handling, or Bubble Tea update flow.
12. Do not add dependencies or permanent tests.

## Out of Scope

- New colors, animations, tabs, or interactions.
- Converting the broader observability request, usage, latency, health, quota,
  and fallback text sections into visual dashboards. That should be a follow-up
  slice.
- New route fields or storage migrations.
- Further splitting non-TUI packages in this slice.
- Changing privacy rules for account display labels.

## Implementation Steps

1. Create the focused `visual_*.go` files and move helper families into them.
2. Simplify `usageGaugeBlock` to one usage bar.
3. Simplify `poolGaugeBlock` to one summative remaining bar and no displayed
   average.
4. Use compact local-time labels for subscription observed and reset times.
5. Delete or empty `visual_helpers.go` once all helpers have clear homes.
6. Run `gofmt`.
7. Review the diff before smoke checks.
8. Run compile, vet, daemon route checks, normal and narrow manage PTY smokes,
   and whitespace checks.

## Smoke Checks

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
tmp="$(mktemp -d)"
pid=""
cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$tmp" "$tmpbin"
}
trap cleanup EXIT
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
cfg="$tmp/config.toml"
cat >"$cfg" <<'EOF'
[server]
bind = "127.0.0.1:0"
[providers.codex]
type = "codex"
[providers.deepseek]
type = "deepseek"
[providers.openrouter]
type = "openrouter"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$cfg" >"$tmp/serve.log" 2>&1 &
pid="$!"
for _ in $(seq 1 80); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/snapshot >/dev/null &&
    curl --silent --fail --unix-socket "$sock" http://ilonasin/_ilonasin/manage/subscription-usage >/dev/null; then
    break
  fi
  sleep 0.1
done
if [ -z "${sock:-}" ]; then
  echo "management socket not found"
  cat "$tmp/serve.log"
  exit 1
fi
db="$tmp/home/ilonasin.sqlite"
sqlite3 "$db" <<EOF
INSERT INTO provider_credentials(id, provider_instance_id, kind, label, created_at, updated_at)
VALUES
  (101, 'codex', 'oauth', 'alice', datetime('now'), datetime('now')),
  (102, 'codex', 'oauth', 'bob', datetime('now'), datetime('now'));
INSERT INTO subscription_usage_snapshots(
  observed_at, provider_instance_id, credential_id, account_display_label,
  plan_label, limit_id, limit_name, plan_type, reached_type,
  primary_used_percent, primary_window_minutes, primary_reset_at,
  secondary_used_percent, secondary_window_minutes, secondary_reset_at,
  source, error_class, stale
) VALUES
  (datetime('now'), 'codex', 101, 'alice@example.com', 'Plus', 'codex', 'Codex', 'plus', '',
   42.5, 300, datetime('now', '+2 hours'), 18.0, 10080, datetime('now', '+3 days'), 'seed', '', 0),
  (datetime('now'), 'codex', 102, 'bob@example.com', 'Pro', 'codex', 'Codex', 'pro', '',
   87.0, 300, datetime('now', '+1 hours'), 66.0, 10080, datetime('now', '+2 days'), 'seed', '', 0);
EOF
set +e
printf '\t\tq' | timeout 3s script -q -e -c \
  "sh -c 'stty cols 100 rows 80; exec env ILONASIN_HOME=\"$tmp/home\" \"$tmpbin/ilonasin\" manage --config \"$cfg\"'" \
  "$tmp/manage-normal.typescript" >/dev/null
normal_status="$?"
printf '\t\tq' | timeout 3s script -q -e -c \
  "sh -c 'stty cols 50 rows 80; exec env ILONASIN_HOME=\"$tmp/home\" \"$tmpbin/ilonasin\" manage --config \"$cfg\"'" \
  "$tmp/manage-narrow.typescript" >/dev/null
narrow_status="$?"
set -e
for status in "$normal_status" "$narrow_status"; do
  if [ "$status" -ne 0 ] && [ "$status" -ne 124 ]; then
    cat "$tmp"/manage-*.typescript
    exit "$status"
  fi
done
for capture in "$tmp/manage-normal.typescript" "$tmp/manage-narrow.typescript"; do
  grep -q "Subscription usage" "$capture"
  grep -q "Codex subscription limits" "$capture"
  grep -q "Subscription pools" "$capture"
  grep -q "alice@example.com" "$capture"
  grep -q "bob@example.com" "$capture"
  grep -Eq "usage|use" "$capture"
  grep -Eq "pool remaining|remaining|rem" "$capture"
  grep -Eq "reset [0-9]{2}:[0-9]{2}|reset [A-Z][a-z]{2} [0-9]{2}" "$capture"
done
if rg -n "account remaining|avg used" "$tmp"/manage-*.typescript; then
  cat "$tmp"/manage-*.typescript
  exit 1
fi
git diff --check
```

Also run a source-layout guard:

```sh
test -f internal/tui/visual_styles.go
test -f internal/tui/visual_identity.go
test -f internal/tui/visual_text.go
test -f internal/tui/visual_gauges.go
test -f internal/tui/visual_cards.go
test ! -f internal/tui/visual_helpers.go
rg -n "cardStyle|heroStyle" internal/tui/visual_styles.go
rg -n "accountIdentity|highlightedIdentity" internal/tui/visual_identity.go
rg -n "metricChip|accountMeta" internal/tui/visual_text.go
rg -n "usageGaugeBlock|poolGaugeBlock" internal/tui/visual_gauges.go
rg -n "renderSectionBanner|renderCardGrid" internal/tui/visual_cards.go
! rg -n "\"ilonasin/internal/(provider|storage|management|credentials|config)\"|bubbletea|log/slog" internal/tui/visual_*.go
```

## Acceptance

- The TUI visual helpers are grouped by responsibility.
- Account window cards do not render duplicate used and remaining bars.
- Pool cards render summative remaining capacity, not average-used bars.
- Subscription timestamps are compact local-time labels rather than raw UTC
  RFC3339 strings.
- Public behavior and management API behavior are unchanged.
- Compile, vet, serve route smoke, manage PTY smoke, layout guard, and
  whitespace checks pass.
- Existing unrelated files are not staged or committed.

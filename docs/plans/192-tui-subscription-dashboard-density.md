# 192 TUI Subscription Dashboard Density

## Goal

Make the existing `ilonasin manage` subscription usage dashboard more compact,
more visual, and semantically clearer while staying inside the current daemon
management snapshot boundary.

This continues the architecture target that `ilonasin manage` is a polished
daily control plane for provider, credential, usage, and health state.

## Ground Truth

- `docs/ilonasin-architecture.md` says the TUI should be visually polished and
  useful for repeated daily operation.
- The TUI must use the daemon-owned management API boundary for mutable
  operations and must not edit `config.toml`.
- Subscription usage data already arrives through `ManagementSnapshotResponse`
  and `SubscriptionUsageResponse`; this slice should improve rendering and
  rendering semantics without adding direct SQLite access from the TUI.
- Existing subscription aggregate windows expose summative percent-point
  fields. The TUI should present pooled capacity as summative remaining and
  used capacity, not averages.
- Pool summative semantics are management-owned. The TUI must render
  `SubscriptionUsagePoolWindow.TotalUsedPercentPoints`,
  `TotalRemainingPercentPoints`, `TotalCapacityPercentPoints`, and
  `EarliestResetAt`; it must not re-derive pool totals, capacity, or reset
  semantics from account counts, averages, minimums, or stale fields.
- Existing account rows include safe account display labels. Email-like labels
  should be first-class visible identifiers when available.

## Scope

1. Keep the existing observability tab and subscription section.
2. Improve account cards:
   - put the visible account identity or email in the card title;
   - avoid duplicating identity text when it is already the title;
   - keep provider, credential, plan, limit, source, observed age, and
     freshness as compact chips;
   - keep one bar per window showing used versus remaining as a single visual.
3. Improve pool cards:
   - remove average language from the visible pool surface;
   - show summative used and remaining percent-points from existing
     `SubscriptionUsagePoolWindow` totals against total pooled capacity;
   - make the reset label explicitly earliest reset;
   - keep stale count visible but secondary.
4. Improve time presentation:
   - render resets in local system time with a short relative label and a local
     clock hint;
   - keep compact behavior on narrow terminals.
5. Reuse or extend existing Bubble Tea/Lipgloss helpers in `internal/tui`;
   avoid introducing a parallel rendering framework.
6. Do not change subscription keepalive behavior or provider semantics.
7. Do not add permanent tests.

## Non-Goals

- No new management route.
- No direct TUI SQLite reads or writes.
- No changes to `config.toml`.
- No subscription keepalive scheduling changes.
- No provider request, credential, or quota behavior changes.
- No plan 300 logging work.

## Implementation

1. Update `internal/tui/observability_subscription.go` to make the subscription
   page denser:
   - account title uses `accountIdentity`;
   - identity line appears only when it adds information beyond the title;
   - account card metadata uses chip-style provider, credential, plan, limit,
     source, observed age, and freshness fields;
   - pool card metadata and bars emphasize total used and total remaining
     percent-points.
2. Update `internal/tui/visual_gauges.go` so usage windows are one balanced
   used/remaining bar with compact labels, not separate visual concepts.
   Do this through `usageGaugeBlock`, `poolGaugeBlock`, or a new subscription
   helper; do not change global `percentBar` semantics used by other
   observability views.
3. Change the pool gauge helper signature to receive used, remaining, and
   capacity percent-points from `SubscriptionUsagePoolWindow`; render both used
   and remaining explicitly.
4. Add small local-time helpers in `internal/tui/display.go` or nearby TUI
   display helpers for concise reset labels.
   - account windows should render `reset <relative> <HH:MM> local`;
   - pool windows should render `earliest reset <relative> <HH:MM> local`;
   - narrow terminals may compact to the relative part.
5. Keep DTO fields stable. No DTO additions are expected for this slice; the
   existing pool window totals already carry the summative semantics needed.

## Verification

Run:

```sh
set -euo pipefail
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
git diff --check
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
cat >"$tmp/config.toml" <<EOF
[server]
bind = "127.0.0.1:0"

[paths]
data_dir = "$tmp/data"
log_dir = "$tmp/logs"
cache_dir = "$tmp/cache"

[logging]
level = "info"
format = "json"
outputs = ["file"]
capture_io = false

[subscription_keepalive]
enabled = false

[providers.codex]
type = "codex"
EOF
ILONASIN_HOME="$tmp/home" "$tmpbin/ilonasin" serve --config "$tmp/config.toml" &
pid="$!"
trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; rm -rf "$tmp" "$tmpbin"' EXIT
for _ in $(seq 1 100); do
  sock="$(find "$tmp/home/run" -type s -name 'manage-*.sock' -print 2>/dev/null | head -n 1 || true)"
  if [ -n "$sock" ] && curl --silent --fail --unix-socket "$sock" \
    http://ilonasin/_ilonasin/manage/health >/dev/null; then
    break
  fi
  sleep 0.1
done
test -n "${sock:-}"
timeout 3s script -q -e -c "stty cols 140 rows 45; exec env ILONASIN_HOME=$tmp/home $tmpbin/ilonasin manage --config $tmp/config.toml" /dev/null || true
kill "$pid"
wait "$pid" 2>/dev/null || true
rm -rf "$tmp" "$tmpbin"
```

Also run a temporary in-package render smoke and remove it before commit. The
smoke must seed a `tui.Model` with:

- fixed local-aware `now` time `2026-06-01T21:00:00+08:00`;
- account `alex@example.com`, fresh, source `codex`, observed at now,
  5h window used `25`, remaining `75`, reset at `2026-06-01T22:00:00+08:00`,
  weekly used `40`, remaining `60`, reset at `2026-06-03T09:30:00+08:00`;
- account `backup@example.com`, stale, source `codex`, 5h used `80`,
  remaining `20`, weekly used `50`, remaining `50`;
- one pool window for `5h` with two accounts, total used `105.0`, total
  remaining `95.0`, total capacity `200.0`, earliest reset at
  `2026-06-01T22:00:00+08:00`;
- one pool window for `weekly` with total used `90.0`, total remaining
  `110.0`, total capacity `200.0`, earliest reset at
  `2026-06-03T09:30:00+08:00`.

The smoke must assert rendered output contains:

- `alex@example.com`;
- `backup@example.com`;
- `source codex`;
- `observed now`;
- exactly one rendered account window line per seeded account/window label;
- `used 105.0`;
- `left 95.0`;
- `cap 200.0`;
- `earliest reset in 1h 22:00 local`;
- `earliest reset in 1d 09:30 local`;
- no `average`, `avg`, `minimum`, or `lowest` wording in the subscription
  section.

The render smoke must be temporary and removed before commit.

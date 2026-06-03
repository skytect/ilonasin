# 437 TUI Shared Meter Rows

## Context

The TUI already has gauges, bars, chips, cards, and independently scrollable
panes. Several panes still express related totals as mostly text, which makes
future visual polish harder to keep consistent.

## Goal

Add a small shared TUI rendering primitive for compact meter rows so later UI
slices can replace text-heavy summaries with consistent visual rows.

## Scope

1. Add a shared helper in `internal/tui/visual_gauges.go` for one-line meter
   rows with this contract:
   - callers pass a sanitized label, a pre-rendered bar string, a sanitized
     primary value string, the target line width, and optional already-rendered
     trailing chips;
   - the helper does not calculate percentages or choose gauge semantics;
   - callers keep using existing `percentBar`, `remainingBar`, or
     `balancedUsageBar` helpers to build the bar;
   - the helper uses existing ANSI-aware wrapping helpers.
2. Use the new helper in `compactRateBars` in `internal/tui/metric_visual.go`,
   preserving the displayed rate label, bar, and percent text for cache hit,
   cache miss, cache write, and reasoning rate rows.
3. Keep existing `percentBar`, `remainingBar`, `balancedUsageBar`,
   `usageGaugeBlock`, and `poolGaugeBlock` behavior unchanged.
4. Keep TUI data read-only through existing management snapshots.
5. Do not change management DTOs, storage, provider behavior, daemon routes,
   config, key handling, pane layout, or quota math.
6. Do not add permanent tests.

## Out Of Scope

- No changes to subscription account or pool quota math.
- No changes to token mix segment calculations.
- No changes to latency duration bar calculations.
- No new card, pane, or tab layout behavior.

## Verification

Run:

```sh
gofmt -w internal/tui/visual_gauges.go internal/tui/metric_visual.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary focused render check, then remove it before commit:

- seed usage rows with cache hit, cache miss, cache write, and reasoning rates;
- render Usage at narrow and wide widths;
- assert the migrated rate rows still include the same labels, bars, and percent
  text;
- assert stripped output lines fit the target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO
   capture disabled, keepalive disabled, and configured DeepSeek/Codex provider
   instances.
3. Verify management health and snapshot over the management socket.
4. Run bounded `ilonasin manage` at narrow and wide terminal widths.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- A reusable compact meter-row helper exists for upcoming TUI slices.
- `compactRateBars` uses the helper without dropping cache hit, cache miss,
  cache write, or reasoning rate labels, bars, or percent values.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

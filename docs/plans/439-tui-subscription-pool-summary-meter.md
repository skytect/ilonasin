# 439 TUI Subscription Pool Summary Meter

## Context

Subscription pool rows already use summative quota values, but the top pooled
summary is still chip-heavy text. The user specifically asked for pooled usage
to show summative remaining rather than averages and for more visual quota
presentation.

## Goal

Make the subscription pool summary visually show summed used and remaining
capacity while preserving existing summative pool math.

## Scope

1. Update `internal/tui/usage_subscription.go` and shared gauge helpers only if
   needed.
2. Keep subscription pools summative only:
   - `TotalUsedPercentPoints`;
   - `TotalRemainingPercentPoints`;
   - `TotalCapacityPercentPoints`;
   - account count;
   - stale count;
   - earliest reset.
3. Replace the chip-heavy `subscriptionPoolSummaryLine` with a compact visual
   summary using the first available pool-window family already selected by
   `firstSubscriptionPoolWindowTotal`.
4. Include the earliest reset for that selected window family when present.
5. Preserve the detailed per-pool `poolGaugeBlock` rows and all account rows,
   including wrapped email/display labels.
6. Do not add averages, lowest-remaining labels, new quota calculations, new
   management DTO fields, storage changes, provider changes, route changes,
   config changes, pane layout changes, or key handling changes.
7. Do not add permanent tests.

## Verification

Run:

```sh
gofmt -w internal/tui/usage_subscription.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run a temporary focused render check, then remove it before commit:

- seed subscription pool aggregates with summed used, remaining, capacity,
  account count, stale count, and reset data;
- render the Usage quota pane at narrow and wide widths;
- assert the pooled summary contains summative used, left, capacity, account,
  stale, and earliest reset values;
- assert no `avg` or `lowest` wording appears;
- assert rendered lines fit the target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO
   capture disabled, keepalive disabled, and configured DeepSeek/Codex provider
   instances.
3. Verify management health and snapshot over the management socket.
4. Run bounded `ilonasin manage` at narrow and wide terminal widths.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- The pooled subscription summary is visual and summative.
- Existing detailed quota rows and account identities remain visible.
- No runtime behavior outside TUI rendering changes.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

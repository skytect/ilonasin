# 276 TUI Usage Compact Visuals

## Goal

Make the `usage` section denser and more visual within the existing
screen-sized pane dashboard.

The current TUI already uses the intended top-level sections:

- API: local API surfaces and downstream key management.
- Providers: upstream provider keys, OAuth accounts, provider inventory, and
  fallback configuration.
- Usage: token usage, quota, health, and performance.
- Logs: request/fallback metadata plus IO and pruning policy.

This slice keeps that organization and tightens the remaining usage panes so
common state is visible with fewer rows and less explanatory text.

## Scope

1. Keep top-level tabs, pane IDs, pane-local scrolling, key handling, and action
   routing unchanged.
2. Cap implementation to render-only edits in:
   - `internal/tui/usage_metrics.go`;
   - `internal/tui/usage_subscription.go`;
   - `internal/tui/usage_health.go`;
   - small shared visual helpers, only if needed for clipped compact rows.
3. Recompose the Usage metrics body:
   - keep token mix, cache rate, reasoning rate, latency, TTFT, and throughput
     visible;
   - keep every stream row visible, including completion status, stream count,
     and chunk count, but place it under the performance area instead of using
     a large standalone section;
   - use compact rows and bars instead of section prose.
4. Recompose the Subscription quota body:
   - keep subscription account emails/display labels visible through existing
     sanitized management DTO labels;
   - keep provider, credential, plan/limit, source, observed time, error, and
     stale/fresh state visible for account rows;
   - show one used/remaining bar per account window;
   - show pooled windows as summative only: summed used percent-points, summed
     remaining percent-points, capacity percent-points, account count, stale
     count, and earliest reset;
   - keep reset labels human-readable in local time.
5. Recompose Health and Quota rows only enough to reduce row count while keeping
   provider/model, event/source, status, credential label, and reset/retry time.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging policy, subscription keepalive, config, or public API behavior
  changes.
- No direct SQLite or `config.toml` mutation from the TUI.
- No overview tab, nested tabs, or layout engine rewrite.
- No edits to `internal/tui/panes.go`, model state, key/action routing,
  management conversion, DTOs, data fetching, provider adapters, storage, or
  quota aggregation helpers.
- No quota math changes and no average pool labels.
- No raw API keys, OAuth tokens, bearer tokens, account IDs, request IDs,
  prompts, completions, request bodies, response bodies, raw SSE chunks, tool
  arguments, or tool results rendered.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused render smoke, then remove it before commit:

- seed usage rows, latency rows, stream rows, subscription account rows,
  subscription pools, health rows, quota rows, safe email-like labels, and
  unsafe marker strings;
- render Usage at 80, 120, 160, and 220 columns plus a short-height view;
- assert token mix bars, cache/reasoning bars, latency bars, subscription
  account bars, summative pool bars, health rows, and quota rows are visible;
- assert pane order and pane IDs stay unchanged for Usage;
- assert pane-local scroll behavior still works: overflowing focused panes have
  scroll markers, offsets clamp, and short-height renders do not overflow;
- assert the Usage refresh action `u` remains scoped to subscription refresh;
- assert every seeded stream row still renders completion status, stream count,
  and chunk count;
- assert pooled quota labels are summative and do not mention averages;
- assert exact seeded values from `TotalUsedPercentPoints`,
  `TotalRemainingPercentPoints`, `TotalCapacityPercentPoints`, `AccountCount`,
  `StaleCount`, and `EarliestResetAt` render instead of account averages;
- assert safe email-like labels render and unsafe marker strings are redacted;
- assert provider/model labels and quota/error labels pass through existing TUI
  sanitizers;
- assert stripped output lines fit target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health over the management socket.
4. Run `manage` under a short timeout at 80, 120, 160, and 220 columns and
   verify API, providers, usage, and logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Usage fits more information into the same screen-sized panes.
- Token, cache, reasoning, latency, subscription, health, and quota state is
  represented with compact rows and bars.
- Subscription pools remain summative only.
- Email-like account labels remain visible where safely exposed.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

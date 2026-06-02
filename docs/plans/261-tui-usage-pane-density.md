# 261 TUI Usage Pane Density

## Goal

Make the `usage` section read like a compact screen-sized dashboard, not a
stack of repeated text cards, without changing daemon behavior.

The current TUI already has the correct top-level sections and pane-local
scrolling:

- API: local API surfaces and downstream keys.
- Providers: upstream key management, OAuth accounts, provider inventory, and
  fallback configuration.
- Usage: token usage, subscription quota, performance, health, and quota.
- Logs: request/fallback metadata plus IO capture and pruning policy.

This slice focuses only on the remaining density gap in Usage.

## Scope

1. Keep top-level tabs as `api`, `providers`, `usage`, and `logs`.
2. Keep existing Usage pane IDs and action routing:
   - `tokens + performance`
   - `quota`
   - `health + quota`
3. Recompose `tokens + performance` so it uses compact visual rows instead of
   a sequence of large repeated cards:
   - provider token rows with request count, total tokens, cost, token mix bar,
     cache hit/miss/write rates, and reasoning rate;
   - provider performance rows with latency, upstream latency, TTFT, and output
     throughput bars;
   - stream rows as compact status rows.
4. Keep `quota` visual and clear:
   - account rows must keep safe email-like display labels visible through
     existing identity helpers;
   - each quota window must use one combined used/remaining bar;
   - pooled rows must stay summative: summed used percent-points, summed
     remaining percent-points, total capacity percent-points, account count,
     stale count, and earliest reset.
5. Recompose `health + quota` as compact event rows rather than repeated cards,
   preserving health event class, status, credential label, retry time, quota
   source, count, and reset time.
6. Limit implementation to TUI files, expected touch points:
   - `internal/tui/usage_metrics.go`
   - `internal/tui/usage_subscription.go`
   - `internal/tui/usage_health.go`
   - existing visual helper files only if a small clipping or row helper is
     needed.
7. Keep times human readable and based on local system time through existing
   time helpers.
8. Use existing Bubble Tea/Lip Gloss render helpers where possible:
   `metricLine`, chips, badges, token mix bars, rate bars, latency bars, and
   quota gauges.
9. Add only small TUI helpers if they reduce duplication or make clipping more
   reliable.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging policy, keepalive behavior, config, or action-routing changes.
- No direct SQLite or `config.toml` mutation from the TUI.
- No raw API keys, OAuth tokens, bearer tokens, full account IDs, request IDs,
  prompts, completions, request bodies, response bodies, raw SSE chunks, tool
  arguments, or tool results rendered.
- No permanent tests.
- Do not add an overview tab or nested tabs.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused render smoke, then remove it before commit:

- seed token usage rows, latency rows, stream rows, subscription accounts,
  subscription pools, health rows, quota rows, safe email-like labels, and
  unsafe marker strings;
- render Usage at 80, 120, 160, and 220 columns, plus a short-height view;
- assert Usage pane IDs and order remain `usagePaneMetrics`,
  `usagePaneSubscriptions`, and `usagePaneHealth`;
- seed overflowing content in all three Usage panes and assert pane focus
  cycling, pane scroll maximums, and pane-local scroll markers still work in the
  short-height view;
- assert `u` refresh remains a Usage-scoped action and does not affect API,
  Providers, or Logs action routing;
- assert safe email-like labels render;
- assert unsafe marker strings remain redacted when seeded in fields that
  render in Usage, including provider IDs, model IDs, subscription account
  labels, limit names/IDs, credential labels, health/quota event classes, and
  error classes;
- assert token mix bars, cache/reasoning rate bars, latency bars, compact health
  rows, compact quota rows, subscription account bars, and summative pool bars
  render;
- assert pool usage renders summed used, remaining, and capacity values, not
  averages;
- assert each quota window has one used/remaining bar;
- assert stripped output lines fit target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Usage keeps the existing pane structure but is visibly denser.
- Token, cache, reasoning, performance, stream, subscription, health, and quota
  state are shown with compact visual rows.
- Subscription pools remain summative and account labels remain visible when
  safely exposed.
- Pane-local scrolling still handles overflow.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

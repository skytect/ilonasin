# 267 TUI Dashboard Pane Composition

## Goal

Make the `usage` section of `ilonasin manage` read more like a compact
control-plane dashboard within the existing top-level sections:

- API: local OpenAI Chat Completions, OpenAI Responses, Anthropic Messages, and
  downstream client token management.
- Providers: upstream provider inventory, upstream API keys, OAuth/provider
  accounts, and fallback configuration.
- Usage: token usage, subscription quota, health/quota, and performance.
- Logs: request/fallback metadata plus IO capture and pruning policy.

The TUI already has these four sections and pane-local scrolling. This slice
should refine Usage body rendering only, not add another navigation model or
redesign every section at once.

## Current Evidence

- `internal/tui/control_sections.go` already exposes `api`, `providers`,
  `usage`, and `logs` through pane lists.
- `internal/tui/panes.go` already implements focused pane navigation,
  independent pane scroll offsets, clipping, and scroll markers.
- API has a `surfaces` pane and `downstream keys` pane.
- Providers currently combines provider instances and model cache in one
  `inventory` pane and keeps upstream keys, OAuth accounts, and fallback in
  separate panes.
- Usage currently combines token usage, performance, and streams in one pane,
  with quota and health/quota separate.
- Logs currently keeps request metadata, fallback metadata, and IO/pruning
  separate.
- Usage body renderers still have the most important density pressure because
  they need to show token mix, cache/reasoning rates, latency, subscription
  windows, health, and quota in screen-sized panes.

## Scope

1. Keep top-level tabs as `api`, `providers`, `usage`, and `logs`.
2. Keep pane-local focus, scrolling, clipping, scroll markers, and section action
   routing unchanged.
3. Keep API, Providers, and Logs pane composition unchanged in this slice.
   Providers must continue to show provider inventory, model cache, upstream
   keys, OAuth accounts, provider accounts, and fallback configuration.
4. Recompose Usage body density without changing DTOs:
   - use compact visual rows for token mix, cache rates, reasoning rate,
     latency, TTFT, and throughput;
   - keep subscription account windows as one used/remaining bar per window;
   - keep pooled subscription windows summative only: summed used
     percent-points, summed remaining percent-points, summed capacity
     percent-points, account count, stale count, and earliest reset;
   - do not label pooled values as averages or a single shared account quota.
5. Keep Logs rendering unchanged except for smoke coverage. The TUI may render
   IO capture policy/status, but must not render captured prompts,
   completions, request/response bodies, raw SSE chunks, tool arguments, or tool
   results even when IO capture is enabled.
6. Prefer rows, chips, bars, and small strips over cards. Use cards only for
   empty states or genuinely grouped repeated objects.
7. Keep human-readable times on existing local-time formatting helpers.
8. Limit likely implementation touch points to Usage render files:
   - `internal/tui/usage_metrics.go`
   - `internal/tui/usage_subscription.go`
   - `internal/tui/usage_health.go`
   - visual helper files only for small clipping or compact-row helpers.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging policy, subscription keepalive, config, or action-routing behavior
  changes.
- No direct SQLite or `config.toml` mutation from the TUI.
- No overview tab, nested tabs, or Bubble Tea navigation rewrite.
- No changes to provider fallback semantics or quota math.
- No Providers, API, Logs, action routing, update-key, or pane-layout changes
  unless a smoke failure proves a narrowly required helper fix.
- No raw API keys, OAuth tokens, bearer tokens, full account IDs, request IDs,
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

- seed API tokens, provider instances, model cache rows, upstream keys, OAuth
  credentials, provider accounts, fallback policies, usage rows, latency rows,
  stream rows, subscription accounts, subscription pools, health rows, quota
  rows, request rows, fallback log rows, pruning state, safe email-like labels,
  and unsafe marker strings;
- render API, Providers, Usage, and Logs at 80, 120, 160, and 220 columns, plus
  a short-height view;
- assert pane IDs and pane order stay stable for the four sections;
- assert focused pane cycling, pane scroll maxima, and pane-local scroll markers
  still work with overflowing pane content;
- assert API shows the three API families and downstream key management;
- assert Providers shows provider inventory, upstream keys, OAuth accounts,
  provider accounts, model cache, and fallback configuration;
- assert Usage shows token mix bars, cache/reasoning rate bars, latency bars,
  subscription account bars, summative pool bars, health rows, and quota rows;
- assert `u` remains active only on Usage for subscription refresh and `p`
  remains active only on Logs for pruning;
- assert Logs shows request metadata, fallback metadata, IO capture policy, and
  pruning policy;
- assert safe email-like labels render and unsafe marker strings are redacted;
- assert subscription pools render summed used, remaining, and capacity values,
  not averages, with one used/remaining bar per quota window;
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

- The visible TUI organization remains API, Providers, Usage, and Logs.
- Each section uses bounded screen-sized panes with pane-local scrolling for
  overflow.
- Common state is shown with compact rows, chips, bars, and policy strips
  instead of long prose or unnecessary repeated cards.
- Usage remains visual and pooled quota remains summative.
- Email-like account labels remain visible where safely exposed.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

# 280 TUI Control Plane Information Architecture

## Goal

Make `ilonasin manage` read like a compact control plane instead of a tabbed
text report, while keeping the existing four top-level sections:

- API: exposed local APIs plus downstream key management.
- Providers: upstream keys, OAuth/provider accounts, provider inventory, and
  fallback configuration.
- Usage: token/quota usage plus performance.
- Logs: metadata ledgers plus IO capture and pruning policy.

The current TUI already has pane-local focus and scrolling. This slice should
refine composition and density inside those panes, not add a new navigation
model.

## Scope

1. Keep top-level sections as `api`, `providers`, `usage`, and `logs`.
2. Keep pane-local scrolling, clipping, focus, and section action routing.
3. Rebalance pane titles and body composition so common information appears in
   screen-sized bounded panes, while preserving existing pane IDs, pane order,
   and `maxDashboardPanes` limits:
   - API keeps local API surfaces and downstream keys;
   - Providers separates runtime inventory/model cache from upstream key,
     OAuth/provider account, and fallback configuration panes;
   - Usage keeps token/performance, subscription quota, and health/quota panes;
   - Logs separates request metadata, fallback metadata, and IO/pruning policy.
4. Tighten body rendering where existing rows are still report-like:
   - use compact rows, chips, bars, and policy strips;
   - use cards only for empty states or genuinely grouped repeated items;
   - keep email-like account labels visible where management DTOs expose safe
     display labels;
   - keep pooled quota bars summative, not average-based.
5. Keep every rendered DTO string on existing sanitizer/display helpers such as
   `safeDisplay`, `safeChromeDisplay`, `credentialDisplay`, fragment chips, and
   account identity helpers.
6. Limit likely implementation files to:
   - `internal/tui/control_sections.go`
   - `internal/tui/providers_instances.go`
   - `internal/tui/providers_model_cache.go`
   - `internal/tui/log_requests.go`
   - `internal/tui/log_fallbacks.go`
   - `internal/tui/log_pruning.go`
   - visual helpers only if needed for clipping or compact rows.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging policy, subscription keepalive, config, or action behavior changes.
- No direct SQLite or `config.toml` mutation from the TUI.
- No overview tab, nested tabs, or Bubble Tea navigation rewrite.
- No pane ID renumbering, pane order changes, action-routing changes, or
  exceeding `maxDashboardPanes`.
- No raw API keys, OAuth tokens, bearer tokens, full account IDs, request IDs,
  prompts, completions, request bodies, response bodies, raw SSE chunks, tool
  arguments, tool results, IO log contents, or raw payload file paths rendered.
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

- render API, Providers, Usage, and Logs at 80, 120, 160, and 220 columns plus
  a short-height view;
- assert API shows OpenAI Chat Completions, OpenAI Responses, Anthropic
  Messages, count tokens, and downstream key counts;
- assert Providers shows provider inventory, model cache, upstream keys, OAuth
  accounts, provider account labels, and fallback groups;
- assert Usage shows token mix bars, cache/reasoning bars, performance metrics,
  account quota bars, summative pool quota bars, health rows, and quota rows;
- assert Logs shows request metadata, fallback metadata, IO capture policy, and
  pruning policy;
- assert safe email-like labels render and unsafe marker strings are redacted;
- assert unsafe full account ID markers and request ID markers are redacted;
- assert the common API, provider, usage, and log state appears in the first
  visible screen at 80 columns and short height where possible;
- assert focused-pane navigation and pane-local scroll keys change only the
  focused pane offset;
- assert action routing remains scoped: downstream token actions on API,
  upstream key/OAuth/fallback actions on Providers, subscription refresh on
  Usage, and pruning on Logs;
- assert ANSI-aware rendered line widths fit the target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- The visible TUI organization matches API, Providers, Usage, and Logs.
- Panes stay bounded with pane-local scrolling for overflow.
- Common state is shown with compact rows, chips, bars, and policy strips.
- Pooled quota remains summative and account emails remain visible where safely
  exposed.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

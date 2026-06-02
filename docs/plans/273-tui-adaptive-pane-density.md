# 273 TUI Adaptive Pane Density

## Goal

Make `ilonasin manage` use wide terminals more effectively while preserving the
current control-plane sections:

- API: OpenAI Chat Completions, OpenAI Responses, Anthropic Messages, and
  downstream key management.
- Providers: provider inventory, upstream keys, OAuth/provider accounts, model
  cache, and fallback configuration.
- Usage: token usage, quota, subscription pools, health, and performance.
- Logs: request/fallback metadata plus IO capture and pruning policy.

The TUI already has these top-level sections and pane-local scrolling. This
slice should make pane composition denser and less vertically wasteful, not add
another navigation model.

## Current Evidence

- `internal/tui/control_sections.go` already defines `api`, `providers`,
  `usage`, and `logs` panes matching the target information architecture.
- `internal/tui/panes.go` already clips pane content and keeps independent
  scroll offsets per pane.
- `internal/tui/panes.go` caps pane columns at three and uses
  `minPaneColumnWidth = 58`, so a providers section with four panes can never
  display all four panes side by side even on very wide terminals.
- Existing usage renderers already use token mix bars, cache/reasoning bars,
  single used/remaining quota bars, and summative pool bars.

## Scope

1. Keep top-level tabs as `api`, `providers`, `usage`, and `logs`.
2. Keep pane IDs, pane order, focused-pane navigation, pane-local scrolling,
   clipping, and section-scoped action routing unchanged.
3. Update the adaptive pane layout so wide terminals can use up to four pane
   columns when a section has four panes.
4. Reduce the minimum pane column threshold conservatively so 160-column and
   220-column terminals show more simultaneous panes while 80-column terminals
   remain single-column and readable.
   Expected column counts are one column at 80 columns, two columns at 120
   columns, three columns at 160 columns, and up to four columns at 220 columns
   when the active section has four panes.
5. Keep pane body changes narrow and density-oriented:
   - shorten pane titles only where it improves fit;
   - prefer existing rows, chips, bars, and compact strips;
   - do not introduce large repeated cards.
6. Preserve human-readable local-time formatting and visible safe email-like
   account labels where the management DTO already exposes them.

## Boundaries

- No management API, storage, schema, provider, server route, Anthropic,
  logging policy, subscription keepalive, config, or DTO changes.
- No direct SQLite or `config.toml` mutation from the TUI.
- No overview tab, nested tabs, or full Bubble Tea navigation rewrite.
- No increase above `maxDashboardPanes = 4`.
- No changes to provider fallback semantics, quota math, credential refresh,
  request routing, or unsupported-field validation.
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

- render API, Providers, Usage, and Logs at 80, 120, 160, and 220 columns, plus
  a short-height view;
- assert layout column counts are 1, 2, 3, and up to 4 respectively for these
  widths when enough panes exist;
- assert pane IDs and pane order stay stable for all sections;
- assert 80-column layout remains one column;
- assert wide layouts use more than three columns when a section has four panes
  and enough width;
- assert pane-local scroll offsets and scroll markers still work;
- assert API shows the three API families and downstream key management;
- assert Providers shows provider inventory, model cache, upstream keys, OAuth
  accounts, provider accounts, and fallback configuration;
- assert Usage shows token mix bars, cache/reasoning rate bars, latency bars,
  one used/remaining bar per quota window, summative pool bars, health rows, and
  quota rows;
- assert Logs shows request metadata, fallback metadata, IO capture policy, and
  pruning policy;
- assert safe email-like labels render and unsafe marker strings are redacted;
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

- Wide terminals can show four section panes side by side when practical.
- Narrow terminals remain readable and use pane-local scrolling.
- The visible organization remains API, Providers, Usage, and Logs.
- Common state is shown with compact rows, chips, bars, and policy strips
  instead of whole-view scrolling or unnecessary card stacks.
- Usage remains visual and pooled quota remains summative.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

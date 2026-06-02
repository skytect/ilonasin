# 265 TUI Pane Body Density

## Goal

Make `ilonasin manage` feel more like a dense screen-sized control plane by
removing one repeated visual pattern: oversized nested section banners inside
already-framed panes.

The existing TUI already has the intended top-level sections:

- API: local API surfaces plus downstream key management.
- Providers: upstream key management, OAuth/provider accounts, provider
  inventory, and fallback configuration.
- Usage: token/quota usage plus performance.
- Logs: metadata and IO/pruning policy.

It also already has pane-local focus and scrolling. This slice should improve a
single density pattern without changing daemon behavior or broad pane
composition.

## Scope

1. Keep top-level tabs as `api`, `providers`, `usage`, and `logs`.
2. Keep pane IDs, pane focus, pane-local scroll state, and section action
   routing unchanged.
3. Add one small TUI-only helper for compact in-pane subheads that preserves
   clipping and existing sanitization.
4. Replace nested `renderSectionBanner` calls with the compact subhead helper
   only in selected panes that currently stack multiple subsections inside one
   framed pane:
   - Providers inventory pane: `Provider instances` plus `Model cache`;
   - Providers OAuth pane: `OAuth accounts` plus `Provider accounts`;
   - Usage tokens/performance pane: `Token usage`, `Performance`, and
     `Streams`;
   - Usage quota pane: `Subscription pools` and `Subscription keepalive`;
   - Usage health pane: `Health` plus `Quota`;
   - Logs IO/pruning pane: `Metadata and IO`.
5. Keep existing row content, bars, cards, pane order, and pane grouping
   otherwise unchanged.
6. Preserve safe email-like account labels where existing management DTOs expose
   them through sanitized display helpers.
7. Keep times human readable and based on existing local-time helpers.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging policy, keepalive behavior, config, or action-routing changes.
- No direct SQLite or `config.toml` mutation from the TUI.
- No overview tab, nested tabs, or Bubble Tea navigation rewrite.
- No changes to `internal/tui/*_actions.go`, `internal/tui/update_keys.go`,
  `internal/tui/control_key_actions.go`, or `internal/tui/panes.go`.
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

- seed API tokens, provider instances, upstream keys, OAuth accounts, provider
  accounts, fallback rows, usage rows, latency rows, stream rows, subscription
  accounts, subscription pools, health rows, quota rows, request rows,
  fallback log rows, pruning state, safe email-like labels, and unsafe marker
  strings;
- render API, Providers, Usage, and Logs at 80, 120, 160, and 220 columns,
  plus a short-height view;
- assert pane IDs and pane order stay stable;
- assert pane focus cycling, pane scroll maxima, and pane-local scroll markers
  still work with overflowing pane content;
- assert Providers shows provider instances/model cache, upstream keys, OAuth
  accounts, provider accounts, and fallback configuration;
- assert Usage shows token mix bars, cache/reasoning rate bars, latency bars,
  subscription account bars, summative pool bars, health rows, and quota rows;
- assert Logs shows request metadata, fallback metadata, IO capture policy, and
  pruning policy;
- assert safe email-like labels render and unsafe marker strings remain
  redacted;
- assert subscription pools render summed used, remaining, and capacity values,
  not averages, with one used/remaining bar per quota window;
- assert stripped output lines fit target widths.
- assert the diff is limited to this plan and TUI render/helper files, with no
  action, update-key, pane-layout, daemon/API/storage/provider/config changes.

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
- Selected nested subsection headers are compact in-pane subheads, not
  oversized section banners.
- Pane-local scrolling still handles overflow.
- Usage remains visual and pooled quota remains summative.
- Email-like account labels remain visible where safely exposed.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

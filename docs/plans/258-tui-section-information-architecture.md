# 258 TUI Section Information Architecture

## Goal

Make the existing `ilonasin manage` section layout match the intended control
plane organization more directly, without changing daemon behavior:

- API: local API surfaces plus downstream client token management.
- Providers: upstream API keys, OAuth/provider accounts, provider instances,
  model cache, and fallback groups.
- Usage: quota/token usage and performance.
- Logs: request/fallback metadata plus IO capture and pruning policy.

The TUI already has these top-level sections and pane-local scrolling. This
slice should refine pane grouping and compact visual summaries, not introduce a
new navigation model.

## Scope

1. Keep top-level tabs as `api`, `providers`, `usage`, and `logs`.
2. Keep pane-local scroll state and focused-pane navigation.
3. Update pane titles and body grouping so:
   - API has an explicit `surfaces` pane for OpenAI Chat Completions,
     OpenAI Responses, Anthropic Messages, and downstream key counts;
   - API keeps local client tokens in its own pane;
   - Providers keeps provider/runtime inventory, upstream keys, OAuth/provider
     accounts, and fallback configuration in bounded panes;
   - Usage keeps token/quota/performance content visual and compact;
   - Logs makes metadata rows and IO/pruning policy distinct, compact panes.
4. Limit implementation to `internal/tui/control_sections.go` plus narrowly
   required existing body helpers. This is an information-architecture pass,
   not a broad visual-density rewrite.
5. Prefer compact rows, chips, and bars over large repeated cards only where
   the surrounding helpers already support it.
6. Keep safe email-like account labels visible where existing management DTOs
   expose them.
7. Improve pane density only with TUI helpers that preserve clipping and line
   width behavior.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging policy, config, or action-routing behavior changes.
- No direct SQLite or `config.toml` mutation from the TUI.
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

- render API, Providers, Usage, and Logs at 80, 120, 160, and 220 columns,
  including a short 80x12 view to catch clipped titles and pane overflow;
- assert API shows the three local API surfaces and downstream key counts,
  treating Anthropic count tokens as part of the Anthropic Messages surface;
- assert Providers includes provider instances, upstream keys, OAuth accounts,
  provider accounts, and fallback groups;
- assert Usage still shows token mix bars, cache/reasoning rates,
  subscription account bars, and summative pool bars;
- assert Logs separates request/fallback metadata from IO/pruning policy;
- assert pane IDs and pane order stay stable for API, Providers, Usage, and
  Logs;
- assert existing section-scoped actions still route to the intended panes:
  downstream token actions on API, upstream API-key/OAuth/fallback actions on
  Providers, subscription refresh on Usage, and pruning on Logs;
- assert safe email-like labels render and unsafe marker strings are redacted;
- assert stripped output lines fit the target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify the four-section chrome
   renders.
5. Remove all temporary artifacts.

## Acceptance

- The visible TUI organization matches API, Providers, Usage, and Logs as
  described above.
- Common state is visible in screen-sized panes, with pane-local scrolling for
  overflow rather than whole-view report scrolling.
- Usage remains visual and pooled quota remains summative.
- Email-like account labels remain visible where safely exposed.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

# 252 TUI Split Pane Visual Density

## Goal

Continue the TUI polish from plan 249 without changing daemon behavior:

1. Keep the top-level sections as API, providers, usage, and logs.
2. Make each section feel like a screen-sized dashboard with independently
   scrollable panes, not one long text report.
3. Reduce duplicate text and use compact visual rows where they carry more
   information than prose.
4. Keep safe account display labels and email-like labels visible where the
   management snapshot already exposes them.

## Scope

1. Pane organization:
   - keep the current four tabs and pane-local scroll state;
   - bias wide terminals toward fewer columns with more horizontal space per
     pane, so route, provider, account, and usage rows have room to breathe;
   - keep narrow terminals usable with the existing focus and scroll keys.
2. API pane:
   - keep the three offered API families visible: OpenAI-compatible chat,
     Responses, and Anthropic-compatible Messages;
   - show Anthropic count tokens as part of the Anthropic family, not as a
     duplicated fourth route family;
   - keep downstream local token management separate from upstream provider
     credentials.
3. Providers pane:
   - keep upstream provider instances, upstream API keys, OAuth accounts, and
     fallback policy in separate panes;
   - keep OAuth account email/display labels visible through existing sanitized
     identity helpers.
4. Usage pane:
   - keep token usage, cache/reasoning rates, subscription quota, health, quota,
     and performance visible;
   - make subscription pools clearly summative: summed used percent-points,
     summed remaining percent-points, total capacity percent-points, account
     count, stale count, and earliest reset;
   - keep compact pooled headlines grouped by quota window, so 5h and weekly
     limits are never merged into one apparent quota;
   - use one used/remaining bar per quota window.
5. Logs pane:
   - keep metadata request rows, fallback rows, IO policy, and pruning;
   - keep raw IO out of the UI unless existing policy explicitly allows safe
     metadata display.
6. Add only small TUI helpers where they make the display clearer. Do not add
   new daemon routes, DTOs, database schema, provider behavior, logging policy,
   config mutation, or permanent tests.

## Non-Goals

- No overview tab.
- No Bubble Tea navigation rewrite.
- No new dependency.
- No provider, Anthropic, subscription keepalive, or logging behavior changes.
- No raw prompt, completion, request body, response body, SSE chunk, tool
  argument, tool result, bearer token, OAuth token, API key, full account ID, or
  request ID rendering.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary daemon and TUI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, logging
   capture disabled, keepalive disabled, and a couple of provider instances.
3. Run `manage` under 80, 120, 160, and 220 columns.
4. Confirm all four sections render and pane-local scroll markers still work
   when the terminal is short.
5. Confirm the pane layout uses wider panes on 120+ column terminals rather
   than returning to narrow four-column dashboards.
6. Remove all temporary artifacts.

Also run a temporary focused render smoke, then remove it before commit. Seed a
`Model` with:

- safe email-like subscription and OAuth display labels;
- unsafe identity marker strings;
- subscription pool windows with multiple account capacity;
- request metadata rows.

Assert:

- safe email-like labels render;
- unsafe identity markers are redacted;
- the API pane shows the three API families without a duplicate Anthropic count
  route family;
- pool usage renders summative used, remaining, and capacity values;
- usage windows use one combined used/remaining bar;
- output lines fit 80, 120, 160, and 220 column views after ANSI stripping.
- provider API-key rows, fallback rows, usage/performance rows, and request
  metadata rows fit their pane widths after ANSI stripping.
- pane layout column counts or pane widths match the intended wide-pane bias at
  the smoke widths.

## Acceptance

- The TUI keeps API, providers, usage, and logs as the only top-level sections.
- The dashboard uses independently scrollable panes within one screen-sized
  view.
- Pooled subscription usage is clearly summative, not averaged.
- OAuth/subscription email labels are visible when safely exposed by the daemon.
- API/provider/usage/logs panes are denser and less text-heavy.
- Compile, vet, build, daemon smoke, manage smoke, focused render smoke, and
  implementation reviews pass.

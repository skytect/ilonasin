# 103 TUI Tabs And Scroll

## Context

`ilonasin manage` currently renders one long text document. On real terminals
the useful sections overflow, the operator cannot scroll to hidden content, and
unrelated domains are mixed together. This blocks the architecture goal of a
polished management TUI and makes the richer Plan 102 observability metadata
hard to use.

The architecture says the TUI is a Bubble Tea/Lipgloss local control plane. It
must talk to the daemon-owned management API for mutable operations and must not
edit `config.toml`.

## Goal

Make the management TUI usable on normal terminals by adding a tabbed layout and
scrollable content area. This is the foundation for later visual polish,
quota-focused views, log views, and performance charts.

## Scope

1. Add layout state to `tui.Model`.
   - Track terminal width and height from `tea.WindowSizeMsg`.
   - Track active tab.
   - Track a scroll offset per tab or a single active scroll offset if that
     keeps the first slice smaller.
2. Split the current monolithic render into tabs.
   - `overview`: providers, model cache, pruning status.
   - `accounts`: local API tokens, upstream credentials, OAuth accounts,
     provider accounts, and credential groups.
   - `observability`: recent requests, usage totals, latency, stream, health,
     quota, and fallback metadata.
   - `help`: key bindings and transient messages.
3. Make content scrollable.
   - Reserve fixed header, tab bar, status, and footer rows.
   - Clip the active tab body to the available viewport height.
   - Support `pageup/pagedown`, `home/end`, and mouse wheel if Bubble Tea sends
     wheel key events in the current version.
   - On non-accounts tabs, support `up/down` and `j/k` for scrolling.
   - On the accounts tab, preserve `up/down` and `j/k` for the existing local
     token and OAuth selection behavior. Use `pageup/pagedown`, `home/end`, and
     mouse wheel for accounts-tab scrolling in this slice.
   - Keep mutating keys scoped to their relevant tab: account and credential
     mutations only on `accounts`, telemetry pruning only on `observability`,
     and global navigation/quit keys everywhere.
4. Keep rendering safe.
   - Continue using the existing display sanitizers.
   - Do not render prompts, completions, raw request/response bodies, raw SSE,
     tool arguments, tool results, bearer tokens, full account IDs, full request
     IDs, or provider payloads.
   - Remove the existing full local-token reveal from the TUI. After creating a
     local token, render only safe metadata such as local token ID, prefix, and
     last four characters.
5. Keep behavior narrow.
   - Do not add log ingestion, quota probing, provider balance checks, config
     editing, charts, or new storage tables in this slice.
   - Do not add permanent tests or check files.
   - Do not add a new TUI dependency unless the implementation becomes simpler
     and clearly lower risk than manual viewport clipping.
   - Do not redesign local-token creation beyond removing full-token rendering.
     Token generation stays a management API operation.

## Acceptance

- `ilonasin manage` opens to a tabbed interface instead of one overflowing text
  page.
- Content longer than the terminal height can be scrolled.
- Tab switching does not lose current snapshot data or selected OAuth/local
  token state.
- Account mutating keys still work only through the management API clients and
  only from the `accounts` tab. Telemetry pruning works only from the
  `observability` tab.
- The TUI still does not mutate `config.toml`.
- A rendered TUI smoke is captured at small and normal terminal sizes to verify
  that text is clipped instead of overflowing and that the footer remains
  visible.
- Privacy scan over rendered TUI output, including after local-token creation,
  finds no prompt markers, completion markers, raw bodies, raw SSE, tool
  argument markers, tool result markers, bearer tokens, full account IDs, full
  local tokens, or provider request IDs.
- `find . -name '*_test.go' -type f -print` confirms no permanent tests were
  added.
- `go test ./...` passes as a compile/package check.
- `go vet ./...` passes.
- A fresh binary builds.
- Direct short-lived `ilonasin serve` and `ilonasin manage` smokes run against a
  disposable home.

## Review Questions

1. Is it better to use a small manual viewport for this slice, or should we add
   the Bubbles viewport dependency now?
2. Are the initial tab boundaries right, or should quota get its own tab before
   a broader observability revamp?
3. No open behavior question remains for mutating keys in this slice: account
   shortcuts are scoped to `accounts`, pruning is scoped to `observability`, and
   navigation/quit stays global.
4. No open privacy question remains for full local-token reveal in this slice:
   remove it from rendered TUI output.

# 224 TUI Local Panes

## Goal

Make `ilonasin manage` read as a screen-sized control plane instead of one long
scrolling document. Keep the `api`, `providers`, `usage`, and `logs` sections
from plan 223, but split each section into multiple local panes with their own
scroll state.

## Constraints

- Preserve the daemon-backed management boundary.
- Do not mutate `config.toml` from the TUI.
- Do not add permanent tests.
- Keep the current management DTOs and existing actions.
- Avoid turning every block into a large card. Use compact tables, gauges, and
  bounded panes where they carry the information better.

## Implementation

1. Add a small pane model to the TUI state:
   - focused pane per top-level tab,
   - scroll offset keyed by tab and pane,
   - pane focus navigation with `[` and `]`, plus mouse wheel and page keys
     routed to the focused pane.
2. Add a local pane renderer:
   - screen-sized body below the tab bar,
   - two-column layout when terminal width allows it,
   - stacked layout for narrow terminals,
   - fixed pane heights with clipped content and a compact scroll marker.
3. Rebucket tab bodies into panes:
   - `api`: surfaces/bind, local API tokens, guidance,
   - `providers`: provider instances/model cache, upstream API keys/fallbacks,
     OAuth and provider accounts,
   - `usage`: token/cost/cache/latency, subscription quota pools/accounts,
     health/quota and keepalive,
   - `logs`: recent request metadata, fallback events, pruning.
4. Keep existing row renderers where they are already useful, but add compact
   pane-specific summaries so common state is visible without whole-page
   scrolling.
5. Update footer/help copy for pane focus and local scrolling.

## Verification

- Inspect the changed code before running checks.
- Run `git diff --check`.
- Run `go test ./...` as a compile/package check.
- Run `go vet ./...`.
- Build `cmd/ilonasin`.
- Start a temp daemon and smoke `ilonasin manage` in a PTY with a wide terminal.

## Review Notes

- Check that old whole-tab scroll state is no longer the primary path for these
  dashboard tabs.
- Check that actions remain scoped to the intended top-level section.
- Check that pane rendering does not introduce provider/server dependencies.

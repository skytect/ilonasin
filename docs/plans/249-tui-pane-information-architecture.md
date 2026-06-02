# 249 TUI Pane Information Architecture

## Goal

Make `ilonasin manage` fit the current product shape without adding daemon/API behavior:

1. API: local routes plus downstream local key management.
2. Providers: upstream provider instances, API keys, OAuth accounts, and fallback configuration.
3. Usage: token/quota usage plus performance.
4. Logs: request/fallback metadata plus IO logging and pruning policy.

## Changes

1. Keep the four existing top-level tabs and remove any need for an overview tab.
2. Make pane content more compact:
   - prefer rows and visual bars over prose,
   - use cards only for repeated objects or genuinely grouped state,
   - keep pool subscription usage summative only.
     Pool windows must show summed used percent-points, summed remaining
     percent-points, summed capacity percent-points, account count, stale
     count, and earliest reset. Do not relabel this as an average or shared
     single-account quota.
3. Improve pane-level scrolling:
   - each pane remains independently scrollable,
   - each tab should fit into a single screen-sized dashboard with minimal whole-view scrolling.
   - avoid changing `panes.go` layout and scroll math in this slice unless a
     focused smoke proves pane-local offsets, scroll markers, and clipping.
4. Make account identity visible where available, including email-like labels,
   using only sanitized management DTO display labels through existing TUI
   sanitizers. Do not render raw upstream account IDs or secret-like labels.
5. Keep local time formatting human readable and based on `time.Local`.

## Boundaries

- No config mutation from TUI.
- No management API or DTO changes. Record missing fields as follow-up only.
- No provider/routing/Anthropic behavior changes in this slice.
- No permanent tests.

## Verification

1. Build and package check:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
2. TUI smoke:
   - build a temporary `ilonasin` binary,
   - start `serve` with temporary `ILONASIN_HOME`,
   - smoke `manage` under at least 80, 120, 160, and 220 columns,
   - include one short-height pane-scroll render case,
   - confirm API, providers, usage, and logs panes render.
3. Focused temporary render smoke:
   - seed TUI model rows for usage, subscription accounts, subscription pools,
     safe email-like labels, and unsafe identity markers,
   - assert safe labels render, unsafe markers remain redacted, pool summed
     values render, and pane order remains API/providers/usage/logs.
   - assert token/raw/payload/prompt/body/SSE/tool markers do not appear when
     they are only present in unsafe identity fields.
4. Remove all temporary smoke artifacts.

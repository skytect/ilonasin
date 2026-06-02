# Plan 247: TUI section density

## Goal

Improve the existing API, providers, usage, and logs dashboard shape without
changing daemon behavior. This slice is limited to TUI render density: use more
horizontal space for pane-local views, and make the usage/log panes more visual
and compact where they currently read as long text blocks.

## Constraints

- Do not change provider routing, route behavior, SQLite schema, or management
  API contracts.
- Keep the TUI as a management API client. Do not add direct config or SQLite
  mutations.
- Keep IO logging policy explicit and metadata-only by default.
- Do not remove the existing four tabs. The useful change is pane content and
  density, not another navigation model.
- Touch `internal/tui` only. Do not edit `internal/management`,
  `internal/server`, `internal/storage`, `internal/provider`, or config files.
- Usage changes are render-only and must use existing management snapshot
  fields. Do not change subscription DTOs, quota aggregation, or accounting
  math.
- Do not add permanent tests.

## Implementation

1. Retune pane layout density only.
   - Allow very wide terminals to use more horizontal space for four-pane tabs.
   - Preserve per-pane scrolling and focus behavior.
   - Avoid making every section a card; use rows for compact inventories and
     cards only for grouped state.

2. Tighten existing API/providers labels where useful.
   - Keep the three user-facing API families visible: Chat Completions,
     Responses, and Anthropic.
   - Keep downstream key management under API.
   - Keep upstream provider instances, upstream keys, OAuth accounts, and
     fallback config under providers.
   - Do not redesign every provider pane in this slice.

3. Rework usage summary rendering.
   - Use visual bars for token mix, cache rates, quota remaining, and basic
     latency/throughput shape.
   - Pool rows should emphasize the existing summed remaining/capacity fields,
     not averages or lowest-account semantics.
   - Keep account-level quota details available in their own pane.

4. Rework logs summary rendering.
   - Separate request metadata, fallback metadata, and IO policy.
   - Turn high-volume request metadata into compact, scannable rows with token
     mix and timing chips.
   - Keep capture policy prominent.

## Verification

- `find . -name '*_test.go' -type f -print`
- `git diff --check`
- `go test ./...`
- `go vet ./...`
- `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
- Temporary focused render smoke with seeded labels/markers:
  - all four tabs render at 80, 120, 160, and very wide columns,
  - pane-local scroll markers still appear when content overflows,
  - safe account display labels remain visible,
  - no raw prompt/body/tool/token/request-id marker renders,
  - no non-`internal/tui` code changes are present.
- Start a temporary daemon and smoke `ilonasin manage` at representative widths
  including 80, 120, 160, and 220 columns.

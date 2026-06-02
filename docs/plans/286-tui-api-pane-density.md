# 286 TUI API Pane Density

## Goal

Make the `api` section feel like a compact local API control surface instead
of a text list. Keep downstream local API token management distinct from
upstream provider credentials.

## Scope

1. Keep the top-level tabs and API pane IDs unchanged.
2. Keep API as the home for local OpenAI-compatible surfaces and downstream
   ilonasin client tokens.
3. Reduce the API surface pane into denser rows:
   - one compact runtime/key status strip;
   - one row per served API family;
   - routes visible when width allows;
   - capabilities rendered as compact chips, not explanatory prose.
4. Compact local token rows:
   - keep selected-row cursor behavior;
   - keep token fragments only, never full token values;
   - keep created/disabled times human-readable through existing helpers;
   - at wider widths join metadata onto the primary row;
   - at narrower widths avoid blank lines and excessive wrapping.
5. Keep newly-created token reveal limited to the existing fragment display.

## Boundaries

- No management API, DTO, storage, schema, provider, server route,
  subscription, logging, config, or action behavior changes.
- No pane ID, tab, layout, navigation, or action-routing changes.
- No raw local tokens, upstream keys, OAuth tokens, bearer tokens, prompts,
  completions, bodies, raw SSE chunks, tool arguments, tool results, full
  account IDs, request IDs, or payload paths rendered.
- No permanent tests.

## Implementation

Touch only:

- `internal/tui/control_sections.go`
- `internal/tui/api_local_tokens.go`

Use existing visual helpers such as `metricLine`, `metricChip`,
`apiChromeChip`, `statusBadge`, `fragmentChip`, `timeChip`, and
`optionalTimeChip`. Add at most a tiny local helper if needed to remove blank
lines in compact token rows.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused render smoke, then remove it before commit:

- seed local tokens with enabled, disabled, selected, and revealed states;
- render API at 80, 120, 160, and 220 columns;
- assert stripped rendered lines fit the requested width;
- assert Chat Completions, Responses, Anthropic Messages, model discovery,
  local token counts, and downstream key controls remain visible;
- assert the selected local token cursor remains visible after compaction;
- assert API pane IDs/order stay stable and `n`/`d` still target downstream
  local token actions on the API tab only;
- assert no full token or unsafe secret markers render.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health and snapshot over the management socket.
4. Run `manage` under short timeouts at narrow and wide terminal sizes.
5. Verify API, providers, usage, and logs chrome renders.
6. Remove all temporary artifacts.

## Acceptance

- API pane stays within the four-section dashboard.
- API surface rows are compact and route-focused.
- Downstream local token management stays visible and separate from upstream
  provider credential management.
- No secrets or raw IO content are rendered.
- Compile, vet, focused render smoke, daemon/manage smoke, senior plan review,
  and senior implementation review pass.

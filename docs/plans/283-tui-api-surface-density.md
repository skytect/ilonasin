# 283 TUI API Surface Density

## Goal

Make the `api` section of `ilonasin manage` clearly show the three first-class
client API families and downstream key management without turning the pane into
a route list or prose block.

The architecture describes `ilonasin manage` as a polished local control plane,
not a debug panel. Recent TUI slices already established the four top-level
sections and pane-local scrolling. This slice tightens the API pane content
only.

## Scope

1. Keep top-level sections, pane IDs, pane order, pane-local scrolling, and
   key/action routing unchanged.
2. Update the API `surfaces` pane so it presents exactly three first-class API
   families:
   - OpenAI Chat Completions;
   - OpenAI Responses;
   - Anthropic Messages.
3. Keep Anthropic Count Tokens visible as a compact capability of the Anthropic
   API family, not as a fourth surface.
4. Keep model discovery visible as a compact `/v1/models` capability of the
   OpenAI-compatible API area.
5. Keep downstream local API token management visible in the API area:
   - surface pane shows a compact key-management status/action strip;
   - downstream keys pane remains the detailed token list and mutation surface.
6. Prefer compact rows, chips, and small visual state over cards or explanatory
   text.
7. Limit implementation to `internal/tui/control_sections.go` unless readback
   proves a tiny helper extraction is needed.

## Boundaries

- No management DTO, daemon API, storage, route, provider, credential, or config
  changes.
- No new TUI navigation model, tab, pane ID, pane order, or scroll math changes.
- No direct SQLite access changes.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary render smoke, then remove it before commit:

- seed a TUI model with runtime bind and local tokens;
- render the API tab at representative widths such as 80, 120, 160, and 220;
- assert every rendered line fits within the requested width at each size;
- assert the pane contains API, providers, usage, and logs chrome through the
  full view smoke path;
- assert the API surface pane shows OpenAI Chat, OpenAI Responses, and
  Anthropic Messages;
- assert exactly three first-class surface rows/families are present;
- assert `/v1/models` remains visible as model discovery capability;
- assert Anthropic Count Tokens is represented as a count-token capability, not
  as a fourth first-class surface row/family;
- assert downstream key status/actions remain visible.
- assert API pane IDs and order remain `apiPaneSummary`, then `apiPaneTokens`;
- assert `n` and `d` still target downstream key management behavior, including
  focusing the downstream keys pane when invoked from another section.
- review final diff scope and confirm pane ID/order files, scroll math, and
  action-routing files were not changed.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- API surface pane reads as three first-class API families plus downstream key
  status.
- Count-tokens compatibility remains visible without inflating the surface
  count.
- Model discovery remains visible as an OpenAI-compatible API capability.
- Downstream key management remains visible and actionable.
- Pane IDs, pane order, focus, scrolling, and action routing are unchanged.
- Temporary smoke and direct compile/vet/daemon/manage checks pass.
- Three senior plan reviews and three senior implementation reviews pass.

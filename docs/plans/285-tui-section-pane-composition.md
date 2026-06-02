# 285 TUI Section Pane Composition

## Goal

Make `ilonasin manage` read as four compact control-plane sections, not a
debug report or overview page:

- API: the three client API families plus downstream key management.
- Providers: upstream key management, OAuth accounts, provider inventory, and
  fallback metadata/groups.
- Usage: token/quota usage plus performance.
- Logs: metadata ledgers plus IO capture and pruning policy.

The existing pane dashboard already supports bounded, independently scrollable
views. This slice should lean into that model: keep multiple screen-sized panes
inside each section and reduce prose inside the panes.

## Scope

1. Keep the top-level tabs as `api`, `providers`, `usage`, and `logs`.
2. Keep pane-local focus, scrolling, clipping, action routing, pane IDs, and
   `maxDashboardPanes` unchanged.
3. Recompose pane titles and body density so the four sections map directly to
   the management model:
   - API shows OpenAI Chat Completions, OpenAI Responses, Anthropic Messages,
     model discovery, and downstream key state.
   - Providers shows configured provider inventory, model cache, upstream API
     keys, OAuth accounts, safe account labels, and fallback metadata/groups.
   - Usage shows token mix, cache/reasoning counts, quota/subscription windows,
     pool summative remaining, latency, TTFT, throughput, health, and quota
     blocks.
   - Logs shows metadata-only requests, fallback metadata, IO capture policy,
     and pruning controls.
4. Prefer compact rows, chips, bars, and small grouped strips over explanatory
   sentences.
5. Use cards only for repeated grouped items or empty states. Avoid wrapping
   every row in a card.
6. Keep safe email-like account labels visible where management DTOs expose
   them.
7. Keep pooled subscription windows summative only. Do not display averages.
8. Render reset/retry/observed timestamps in local time and human-readable
   relative form using existing time helpers.
9. Keep fallback rows visibly operator/display metadata. Do not make the
   Providers pane imply cross-provider, cross-model, or user-configured serving
   fallback semantics beyond the existing same-provider credential pool.

## Boundaries

- No overview tab or overview pane.
- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging policy, subscription keepalive, config, or action behavior changes.
- No direct SQLite or `config.toml` mutation from the TUI.
- No new global scrolling model, nested tabs, or Bubble Tea navigation rewrite.
- No pane ID renumbering, pane order changes, or action-routing changes.
- No raw API keys, OAuth tokens, bearer tokens, full account IDs, request IDs,
  prompts, completions, request bodies, response bodies, raw SSE chunks, tool
  arguments, tool results, IO log contents, or raw payload file paths rendered.
- No permanent tests.

## Implementation Slice

Keep this slice intentionally narrow. Actual render edits should be limited to
provider and log row density only:

- `internal/tui/providers_upstreams.go`
- `internal/tui/providers_oauth.go`
- `internal/tui/providers_fallback.go`
- `internal/tui/log_requests.go`
- `internal/tui/log_fallbacks.go`
- `internal/tui/log_pruning.go`

Do not change pane layout, pane IDs, tab labels, action routing, model state,
sanitizers, management DTOs, storage, provider behavior, or shared visual
helpers unless a tiny named compact render helper is proven necessary during
implementation review.

Do not touch Usage quota math or management DTOs in this slice. Usage already
has summative pool rendering and local relative reset labels; this pass may
only verify that behavior.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused render smoke, then remove it before commit:

- seed API tokens, providers, upstream keys, OAuth rows, provider accounts,
  fallback policies, usage rows, latency rows, stream rows, subscription rows,
  pool rows, health rows, quota rows, request rows, fallback rows, pruning state,
  safe email-like labels, and unsafe marker strings;
- render API, Providers, Usage, and Logs at 80, 120, 160, and 220 columns;
- assert every stripped rendered line fits the target width;
- assert each section shows its intended management content in bounded panes;
- assert pane-local scroll offsets clamp and scroll markers appear only for
  overflowing panes;
- assert downstream token actions stay on API, upstream key/OAuth/fallback
  actions stay on Providers, subscription refresh stays on Usage, and pruning
  stays on Logs;
- assert pooled quota labels are summative and do not mention averages;
- assert safe email-like account labels render and unsafe secret/account/request
  markers are redacted.
- assert Logs renders IO capture policy only, not IO log paths, prompt/body
  snippets, response snippets, raw SSE chunks, tool arguments, or tool results;
- include both `capture_io=false` and `capture_io=true` runtime states in the
  render smoke so enabled IO policy remains status-only and does not render raw
  IO paths or contents;

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health over the management socket.
4. Run `manage` under short timeouts at narrow and wide terminal sizes.
5. Verify API, providers, usage, and logs chrome renders.
6. Remove all temporary artifacts.

## Acceptance

- The TUI has no overview section and the four top-level sections match the
  requested information architecture.
- Each section uses bounded panes with independent scrolling for overflow.
- Common state is visible with compact rows, chips, bars, and policy strips.
- Not everything is a card.
- Pooled subscription usage remains summative only.
- Safe account email labels remain visible.
- Compile, vet, focused render smoke, daemon/manage smoke, senior plan review,
  and senior implementation review pass.

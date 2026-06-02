# 262 TUI Model Cache Density

## Goal

Finish the remaining Providers inventory density gap without changing daemon
behavior.

The Providers section already uses compact rows for provider instances,
upstream API keys, OAuth accounts, provider accounts, and fallback groups. The
model cache subsection still renders plain dash-prefixed text with UTC timestamp
strings. That makes the inventory pane feel less like the rest of the polished
Bubble Tea/Lip Gloss dashboard.

## Scope

1. Keep top-level tabs and provider pane IDs unchanged.
2. Keep model-cache data source unchanged: existing `management.ModelMetadata`
   rows in the management snapshot.
3. Keep model-cache aggregation by provider instance, but carry the latest
   `UpdatedAt` as `time.Time` rather than a preformatted UTC string.
4. Render populated model-cache summaries as compact metric rows:
   - provider instance ID,
   - model count,
   - last updated in local human-readable time,
   - source/status chips.
5. Preserve the existing empty-state card.
6. Use existing sanitizers and visual helpers only.
7. Limit implementation to `internal/tui/providers_model_cache.go` unless a
   plan review finding requires a tiny helper change.

## Boundaries

- No management API, DTO, storage, schema, provider, server route, Anthropic,
  logging, config, or action-routing changes.
- No direct SQLite or `config.toml` mutation from TUI.
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

Run a temporary focused in-package render smoke, then remove it before commit:

- seed provider instances and model metadata rows for multiple providers;
- seed out-of-order model metadata timestamps for the same provider and assert
  the latest `UpdatedAt` is selected;
- include unsafe marker strings in provider instance IDs and model metadata
  provider IDs;
- render Providers at 80, 120, 160, and 220 columns;
- assert safe provider IDs render and unsafe provider IDs are redacted;
- assert model-cache rows render as compact metric rows, not dash-prefixed
  prose;
- assert model-cache timestamps use local relative time formatting;
- assert provider pane IDs and order are unchanged;
- assert existing provider action routing remains unchanged, or verify no
  provider action files were touched;
- assert stripped output lines fit target widths.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify the management health endpoint over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Model cache rows use compact dashboard styling consistent with Providers.
- Local relative time is used for model-cache updates.
- Provider pane identity, pane order, key routing, and daemon-backed boundaries
  remain unchanged.
- Compile, vet, focused render smoke, serve smoke, manage smoke, senior plan
  review, and senior implementation review pass.

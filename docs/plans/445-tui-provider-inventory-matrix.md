# 445 TUI Provider Inventory Matrix

## Context

The Providers tab already has panes for runtime, upstream keys, OAuth accounts,
and pool groups. The runtime pane still separates provider instance rows from
model-cache rows, so the user has to scan multiple blocks to answer basic
questions:

- which provider has models cached;
- which provider has upstream credentials;
- which provider has OAuth credentials and captured account identity;
- which provider capabilities are available.

The architecture requires a polished management TUI backed by management
snapshots only. This slice improves provider rendering without changing daemon
state, management DTOs, credentials, storage, config, or provider behavior.

## Goal

Render provider instances as a compact inventory matrix where each provider row
shows capability coverage plus related model, key, OAuth, and account counts.

## Scope

1. Update `internal/tui/providers_instances.go`.
2. Add small provider-summary helpers that derive per-provider counts from the
   existing in-memory TUI snapshot fields:
   - model cache count;
   - upstream credential count;
   - OAuth credential count;
   - provider account count.
3. Update the provider runtime summary to include total models, upstream
   credentials, OAuth credentials, and accounts.
4. Update `providerInstanceRow` to render a compact row with:
   - provider ID, type, auth style, base host;
   - capability glyphs or chips for chat, models, API key, OAuth, and refresh;
   - related model, key, OAuth, and account counts.
5. Keep the existing separate model-cache block for now, but make it a
   secondary detail rather than the only way to see model counts.
6. Keep provider pane counts, pane layout, keyboard/mouse actions, management
   DTOs, storage, config, provider behavior, routing, logging, and credential
   mutation behavior unchanged.
7. Do not add permanent tests.

## Verification

Use temporary focused render checks, then remove them before commit:

- provider rows render at widths 70, 100, and 140 without line overflow;
- providers with no related rows display zero counts;
- providers with models, API keys, OAuth credentials, and accounts display the
  expected counts;
- long safe provider IDs and base hosts wrap rather than forcing overflow;
- no provider secrets or OAuth token material is displayed.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health and
snapshot over the Unix socket, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- Provider runtime rows show related model, key, OAuth, and account inventory
  without requiring a separate block scan.
- Existing provider data and management behavior are unchanged.
- The TUI still fits narrow and wide terminal smoke runs.
- No permanent tests are added.

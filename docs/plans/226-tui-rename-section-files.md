# 226 TUI Rename Section Files

## Goal

Rename TUI source files whose names still describe removed legacy sections.
After plans 223-225, the active TUI sections are `api`, `providers`, `usage`,
and `logs`, but several files still use `accounts`, `overview`, or
`observability` names. This slice removes that stale vocabulary from file
boundaries without changing behavior.

## Current Evidence

- Active sections are composed from `control_sections.go` into dashboard panes.
- `overview.go`, `accounts.go`, `observability.go`, and old viewport code were
  removed in plan 225.
- Remaining files with stale section names are still active helpers:
  - `account_actions.go`
  - `account_api_key_actions.go`
  - `account_fallback_actions.go`
  - `account_local_token_actions.go`
  - `account_upstream_actions.go`
  - `accounts_local_tokens.go`
  - `accounts_upstreams.go`
  - `accounts_oauth.go`
  - `accounts_fallback.go`
  - `overview_model_cache.go`
  - `overview_providers.go`
  - `observability_metrics.go`
  - `observability_subscription.go`
  - `observability_health.go`
  - `observability_requests.go`
  - `observability_fallbacks.go`
  - `observability_pruning.go`
  - `observability_visual.go`
  - `observability_actions.go`

## Implementation

1. Move API-token rendering to an API-named file.
2. Move provider-instance, model-cache, upstream-credential, OAuth, and fallback
   renderers to provider-named files.
3. Move usage metric, subscription, and health/quota renderers to usage-named
   files.
4. Move request, fallback, pruning, and log action helpers to log-named files.
5. Rename observability visual helpers to neutral dashboard/metric visual names
   if they are shared between usage and logs.
6. Rename stale action-dispatch symbols such as `updateAccountKey` and
   `updateObservabilityKey` to names that match their current API/provider and
   usage/log responsibilities.
7. Keep package name, domain data names, user-visible domain text, and behavior
   unchanged unless a symbol name
   still contains stale section vocabulary.

## Verification

- Inspect the diff before running checks.
- Run `fd`/`git diff --name-status` for stale section filenames in active TUI
  source.
- Run targeted `rg` checks for stale section/action symbols such as
  `updateAccountKey`, `updateObservabilityKey`, and `observability*` helpers.
- Do not treat legitimate domain terms such as OAuth accounts, provider
  accounts, account metadata, and subscription account counts as stale.
- Run `git diff --check`.
- Run `go test ./...`.
- Run `go vet ./...`.
- Build `cmd/ilonasin`.
- Start a temp daemon and smoke `ilonasin manage` in a PTY.

## Boundaries

- Do not touch `internal/server/*` dirty files.
- Do not change management DTOs, storage, provider adapters, routing, config, or
  TUI behavior.
- Do not add permanent tests.

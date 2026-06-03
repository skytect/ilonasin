# 373 TUI Provider Pool Group Naming

## Context

Plans 365 through 372 moved provider credential group metadata away from
fallback-policy semantics and exposed `credential_pool_groups` in the
management snapshot. The provider tab still uses fallback naming for the pane
ID, pane title, and body helper:

- `providersPaneFallback`;
- title `fallback groups`;
- `providerFallbackBody`.

That pane renders credential pool groups, not fallback event metadata or policy
controls. The logs tab still has legitimate fallback event metadata and should
keep fallback terminology.

## Goal

Rename the provider tab's credential pool group pane internals and title to
match the current management model.

## Scope

1. Rename `providersPaneFallback` to a credential-pool-group pane constant.
2. Rename the provider pane title from `fallback groups` to `pool groups`.
3. Rename `providerFallbackBody` to a credential pool group body helper.
4. Update call sites and navigation references.
5. Keep logs fallback event terminology unchanged.
6. Do not change management DTOs, snapshot JSON, storage, serving routing,
   credential resolution, quota handling, provider adapters, config, logging,
   or subscription usage.

## Verification

Run:

```sh
rg -n "providersPaneFallback|providerFallbackBody|fallback groups" internal/tui
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Provider tab naming consistently says credential pool groups.
- Fallback event log naming remains unchanged.
- No behavior outside TUI naming changes.

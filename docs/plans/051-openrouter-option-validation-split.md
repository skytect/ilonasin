# 051 OpenRouter Option Validation Split

## Context

`internal/provider/http_chat.go` now contains the shared HTTP adapter plus a
large block of OpenRouter request-option validators. The architecture says
provider adapters own provider-specific behavior, but the current file is doing
too many jobs at once. Recent slices added many OpenRouter-only options under
the explicit `provider_options.openrouter` escape hatch, so the validation
cluster is now large enough to deserve its own provider package file.

This slice is a behavior-preserving refactor. It should make the provider
boundary easier to maintain without changing the local API surface.

## Scope

1. Move OpenRouter-specific option validation helpers from
   `internal/provider/http_chat.go` into a new file in the same package,
   `internal/provider/openrouter_options.go`.
2. Keep function names, validation rules, accepted values, returned error
   strings, and forwarding behavior unchanged.
3. Keep the shared adapter flow, Codex behavior, DeepSeek behavior, model
   discovery, cost parsing, stream parsing, request marshaling, credential
   resolution, routing, storage, TUI, and smoke harness behavior unchanged.
4. Do not add request fields, provider options, capability flags, migrations,
   persistence, or permanent tests.
5. Keep raw request bodies, response bodies, provider payloads, prompts,
   completions, raw SSE chunks, tool data, bearer tokens, provider request IDs,
   account IDs, balances, and credits out of storage and local errors exactly
   as before.

## Implementation

1. Create `internal/provider/openrouter_options.go`.
2. Move these helpers as an intact cluster:
   - `validateOpenRouterOptions`
   - `validateOpenRouterReasoning`
   - `validateOpenRouterModelList`
   - `isOpenRouterModelSlug`
   - `validateOpenRouterCacheControl`
   - `validateOpenRouterProvider`
   - `validateOpenRouterProviderSlugList`
   - `isOpenRouterProviderSlug`
   - `validateOpenRouterQuantizations`
   - `isOpenRouterQuantization`
   - `validateOpenRouterProviderSort`
   - `isOpenRouterProviderSortCriterion`
   - `isOpenRouterProviderSortPartition`
   - `validateOpenRouterMaxPrice`
   - `validateOpenRouterPerformancePreference`
   - `isOpenRouterPositivePreferenceNumber`
   - `isOpenRouterMaxPrice`
   - `safeOpenRouterBoundedNumberToken`
   - `isOpenRouterReasoningEffort`
   - `isPositiveJSONInteger`
3. Leave the JSON schema response-format helper in `http_chat.go` for this
   slice because it is currently coupled to the shared `response_format`
   validation path. A future slice can split response-format validation if that
   remains useful.
4. Run `gofmt` on touched Go files.
5. Manually review the diff before running checks.

## Smoke Checks

Run these direct checks before code review:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
rm -rf "$tmp" "$tmpbin"
```

`go test ./...` is only a compile/package check. No permanent test files will
be added.

## Review Questions

1. Is this the right-sized refactor, or should any OpenRouter helpers stay in
   `http_chat.go` to preserve readability?
2. Does splitting only OpenRouter option validation improve the adapter
   boundary without creating a misleading abstraction?
3. Are compile, vet, build, and existing smoke checks enough for a
   behavior-preserving move with no logic edits?

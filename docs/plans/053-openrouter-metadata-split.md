# 053 OpenRouter Metadata Split

## Context

Plans 051 and 052 moved OpenRouter option validation and Codex `/responses`
handling out of `internal/provider/http_chat.go`. The shared HTTP chat adapter
still contains OpenRouter-only metadata helpers:

- cost parsing from OpenRouter chat completion usage,
- cost parsing from OpenRouter stream chunks,
- decimal-to-microunit conversion for OpenRouter credit units,
- OpenRouter `/models` `supported_parameters` to sanitized capability flags.

The architecture says provider adapters own provider-specific behavior and that
model discovery metadata must stay sanitized. This is a behavior-preserving
split that makes the remaining shared HTTP adapter easier to review.

## Scope

1. Move OpenRouter metadata helpers from `internal/provider/http_chat.go` into
   a new same-package file, `internal/provider/openrouter_metadata.go`.
2. Keep all function names, cost parsing behavior, rounding behavior,
   overflow handling, capability flag names, sorting, and ignored unsupported
   parameters unchanged.
3. Keep `CompleteChat`, `StreamChat`, and `normalizeModels` call sites
   unchanged.
4. Keep OpenRouter request validation, DeepSeek behavior, Codex behavior,
   streaming transport, model endpoint selection, storage, routing, TUI, and
   smoke harness behavior unchanged.
5. Do not add provider features, persistence, migrations, or permanent tests.

## Implementation

1. Create `internal/provider/openrouter_metadata.go`.
2. Move this OpenRouter-specific cluster intact:
   - `openRouterCostMicrounitsFromChatCompletion`
   - `openRouterCostMicrounitsFromStreamChunk`
   - `openRouterCostMicrounitsFromUsage`
   - `openRouterCostMicrounitsFromRawCost`
   - `openRouterCreditMicrounits`
   - `decimalDigits`
   - `pow10`
   - `openRouterCapabilityFlags`
3. Leave generic HTTP adapter, shared stream helpers, URL helpers, generic model
   normalization, and generic safe model metadata helpers in `http_chat.go`.
4. Run `gofmt` on touched Go files.
5. Manually review the diff before smoke checks. The Go diff should be a move
   plus import cleanup only.

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

1. Is OpenRouter metadata parsing the right-sized next split after option
   validation and Codex response handling?
2. Should `openRouterCapabilityFlags` move with cost parsing, or should model
   discovery get its own later split?
3. Are compile, vet, build, and CLI smoke checks enough for this
   behavior-preserving extraction?

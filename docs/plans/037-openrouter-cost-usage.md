# 037 OpenRouter Cost Usage

## Goal

Extract OpenRouter's documented request-level `usage.cost` scalar into the
existing `cost_microunits` metadata field as OpenRouter credit microunits.

## Sources

- `docs/ilonasin-architecture.md`
- `docs/openrouter-api.md`
- `docs/deepseek-openrouter-comparison.md`
- `docs/deepseek-api.md`
- `docs/plans/019-usage-metadata-ledger.md`
- OpenRouter OpenAPI `ChatUsage`

## Scope

1. Treat OpenRouter `usage.cost` as a cost in OpenRouter credits and store it
   as integer credit microunits: `round(cost * 1_000_000)`.
2. Extract only the safe scalar at `usage.cost`.
3. Ignore absent, `null`, non-number, negative, non-finite, or overflowing
   cost values by recording zero.
4. Ignore `usage.cost_details`, `usage.is_byok`, generation IDs, account IDs,
   balances, credit totals, route details, and all unknown usage fields.
5. Cover both non-streaming chat completion responses and streaming final usage
   chunks.
6. Keep the existing request/stream response bodies behavior unchanged.
7. Do not add database columns, migrations, permanent tests, or billing endpoint
   calls.

## Non-Goals

- Do not query `/credits`, `/key`, `/activity`, `/generation`, or any account
  usage endpoint.
- Do not persist raw provider usage JSON or cost details.
- Do not estimate costs from pricing tables.
- Do not add response-cache header telemetry in this plan.

## Implementation Plan

1. Add a deterministic provider-gated cost parser in
   `internal/provider/http_chat.go` that accepts a JSON number and converts
   OpenRouter credits to microunits using integer-safe rounding.
2. Apply the parser only when the provider type is `openrouter`.
3. Wire `usage.cost` into `openai.Usage.CostMicrounits` for OpenRouter
   non-streaming and stream usage only.
4. Update serve-check fake OpenRouter responses to include:
   - exact valid `usage.cost` cases such as `0.001234 -> 1234`
   - a rounding boundary such as `0.0000005 -> 1`
   - marker-bearing `cost_details` and unknown cost fields that must not be
     stored or printed
   - invalid cost variants including `null`, string, object, array, negative,
     overflowing exponent values such as `1e309`, and hostile huge exponents
     such as `1e1000000000`
5. Add smoke assertions for OpenRouter non-stream and stream cost rows.
6. Preserve existing DeepSeek and Codex zero-cost expectations, including fake
   DeepSeek/Codex responses that contain `usage.cost` or marker-bearing cost
   fields.
7. Assert marker-bearing ignored cost fields are absent from SQLite,
   `manage --check` output, `serve --check` output, local errors, and
   normalized streamed client output. Non-streaming upstream response bodies
   remain unchanged in this plan.
8. Run:
   - `find . -name '*_test.go' -type f -print`
   - `git diff --check`
   - `go test ./...`
   - `go vet ./...`
   - `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check`
   - `ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check`

## Review Questions

1. Is `usage.cost` sufficiently documented now to relax plan 019's empty cost
   extraction allowlist for OpenRouter only?
2. Is rounding OpenRouter credits to nearest microunit the right deterministic
   storage behavior?
3. Are zero-on-invalid semantics safer than failing otherwise valid provider
   responses?

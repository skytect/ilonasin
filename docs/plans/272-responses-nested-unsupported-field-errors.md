# 272 Responses Nested Unsupported Field Errors

## Goal

Make the remaining generic OpenAI Responses nested unsupported-field diagnostics
reported by the current code scan name fields deterministically, matching the
architecture requirement that unsupported fields return clear errors and are not
silently forwarded.

## Context

- `docs/ilonasin-architecture.md` requires strict local API subsets and clear
  unsupported-field errors.
- Recent slices fixed top-level Responses, Anthropic, and Chat Completions
  diagnostics.
- Responses still has generic nested diagnostics in:
  - `parseResponsesContent`,
  - `responsesToolsToChatTools` for non-Codex function tools,
  - `ResponsesRequest.ToChatCompletionRequest` for `text`.
- This slice does not broadly tighten Responses input item extra keys because
  Codex raw input forwarding is intentionally preserved during compatibility
  work.

## Plan

1. Reuse the package-local sorted unsupported-key helper already added in
   `internal/openai` where raw JSON maps are used.
2. Update `parseResponsesContent` so content item unsupported fields are named:
   - `input[n].content[m].<field> is unsupported`.
3. Preserve content item type-selection order: decode required content item
   `type` before applying type-specific allowlists.
4. Add a sorted unsupported-key helper for `map[string]any`, or otherwise keep
   `decodeJSONObjectUseNumber` value behavior intact while checking keys
   deterministically.
5. Update non-Codex Responses function tool conversion to name unsupported
   function tool fields:
   - `tools[n].<field> is unsupported`.
6. Preserve current tool conversion order: decode `tools[n].type` first, skip
   non-function tool families for non-Codex providers, and only apply the
   unsupported-key allowlist to non-Codex `type: "function"` tools.
7. Preserve Codex raw tool passthrough behavior, Codex raw input passthrough
   behavior, and non-Codex JSON Schema `parameters` pass-through. Do not recurse
   into schema internals.
8. Update Responses `text` conversion on the Codex conversion path so
   unsupported fields are named:
   - `text.<field> is unsupported`.
9. Preserve current non-Codex behavior where `text`, `reasoning`, and
   `service_tier` are rejected wholesale before inspecting nested fields.
10. Preserve request decoding, provider routing, metadata recording, and response
    shapes.
11. Do not add permanent tests. Use temporary decoder/conversion or route smokes
   if useful, then remove them before commit.

## Verification

1. `find . -name '*_test.go' -type f -print`
2. `git diff --check`
3. `go test ./...`
4. `go vet ./...`
5. Use temporary smokes for Responses content, tool conversion, and `text`
   nested unknown fields, including at least one multi-unknown-field case to
   prove sorted deterministic selection.
6. Include smokes proving:
   - content item missing `type` keeps the existing type error before
     unsupported-key diagnostics,
   - non-Codex function tool `parameters` preserves arbitrary nested schema
     keys and numeric values,
   - Codex raw tool passthrough is unchanged,
   - non-Codex skipped non-function tools remain skipped,
   - Codex-path `text.<field>` is named while non-Codex `text` remains rejected
     at the provider-boundary level.
7. Build `ilonasin`, start a disposable daemon, smoke management health and
   snapshot routes, hit `/v1/responses` with an unsupported nested field, then
   run `ilonasin manage` in a short PTY.

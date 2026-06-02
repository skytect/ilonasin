# 270 Anthropic Unsupported Field Errors

## Goal

Make Anthropic Messages request validation name unsupported nested fields
deterministically, matching the architecture requirement that unsupported fields
return clear errors instead of being silently forwarded.

## Context

- `docs/ilonasin-architecture.md` requires strict local API subsets and clear
  errors for unsupported fields.
- `internal/anthropic/types.go` already rejects unsupported top-level fields by
  key, but several nested structures return generic errors such as
  `messages[0] contains unsupported fields`.
- The Anthropic route is now a first-class local API surface, so nested
  diagnostics should be as clear as the OpenAI Responses top-level diagnostic.

## Plan

1. Add a small deterministic helper in `internal/anthropic/types.go` that
   returns the first unsupported key from a raw JSON object after sorting keys.
2. Use it for Anthropic top-level request fields and nested request validation
   for:
   - message objects,
   - content blocks,
   - image source objects,
   - tool definitions,
   - object-form `tool_choice`.
3. Preserve content block type-selection order: decode required content block
   `type` before applying type-specific allowlists, so a block with no `type`
   still reports the missing `type` error.
4. Preserve pass-through object behavior for accepted objects such as
   `cache_control`, `thinking`, `context_management`, and `output_config`.
5. Preserve all accepted fields, request conversion behavior, route behavior,
   response shapes, and metadata recording.
6. Do not add permanent tests. Use temporary smoke tests or direct route checks
   if useful, then remove them before commit.

## Verification

1. `find . -name '*_test.go' -type f -print`
2. `git diff --check`
3. `go test ./...`
4. `go vet ./...`
5. Build `ilonasin`, start a disposable daemon, smoke management health and
   snapshot routes, then run `ilonasin manage` in a short PTY.
6. Use temporary decoder smokes for both Messages and Count Tokens requests:
   - `bogus_top`,
   - `messages[0].bogus`,
   - `messages[0].content[0].bogus` for text, image, tool_use, and tool_result
     blocks where applicable,
   - `messages[0].content[0].source.bogus`,
   - `tools[0].bogus`,
   - `tool_choice.bogus`.
7. Include at least one multi-unknown-field case to prove sorted deterministic
   selection, and one accepted `cache_control` case to prove no pass-through
   regression. Include a content block missing `type` plus an unsupported field
   to prove the type-selection order is preserved.

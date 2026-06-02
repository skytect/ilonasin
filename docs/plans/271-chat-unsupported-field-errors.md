# 271 Chat Unsupported Field Errors

## Goal

Make OpenAI Chat Completions validation name unsupported nested fields
deterministically, matching the architecture requirement that unsupported fields
return clear errors and are not silently forwarded.

## Context

- `docs/ilonasin-architecture.md` requires strict local API subsets and clear
  unsupported-field errors.
- Chat top-level validation already sorts unknown fields, but several nested
  validators still return generic errors such as `tools[0] contains unsupported
  fields` or `messages[0] contains unsupported fields`.
- Recent slices fixed similar diagnostics for Responses and Anthropic surfaces.

## Plan

1. Add a small package-local helper in `internal/openai` for sorted
   unsupported-key selection over `map[string]json.RawMessage`.
2. Scope the helper to Chat request validation paths. Do not change Responses
   parsing behavior in this slice.
3. Use the helper in Chat Completions nested validation for both
   `DecodeChatCompletion` raw validation and the later
   `ChatCompletionRequest.Validate()` path:
   - `tools[n]`,
   - `tools[n].function`,
   - object-form `tool_choice`,
   - `tool_choice.function`,
   - message objects in `messages`,
   - assistant `messages[n].tool_calls[m]`,
   - assistant tool call `function`,
   - user content parts and `image_url` objects,
   - `stream_options`.
4. Preserve and expose nested content-part errors from `validateRawUserContent`;
   do not collapse them back to a generic `messages[n].content[m] is invalid`.
5. Preserve role-first message validation: decode/validate `messages[n].role`
   before role-specific unsupported-key checks.
6. Use content-part type-selection order: decode required `content[m].type`
   before type-specific allowlists, so missing `type` remains a missing-type
   error even if another unsupported key is present.
7. Keep assistant tool-call allowlist checking before required-field checking,
   matching the existing order, but name the unsupported field deterministically.
8. Preserve JSON Schema pass-through for `tools[n].function.parameters`; do not
   recurse into or restrict schema internals.
9. Preserve all accepted fields, request unmarshalling, provider routing,
   metadata recording, and response shapes.
10. Do not add permanent tests. Use temporary decoder or route smokes if useful,
   then remove them before commit.

## Verification

1. `find . -name '*_test.go' -type f -print`
2. `git diff --check`
3. `go test ./...`
4. `go vet ./...`
5. Use temporary smokes for `DecodeChatCompletion` and
   `ChatCompletionRequest.Validate()` representative nested unknown fields,
   including a multi-unknown-field case to prove sorted deterministic selection.
6. Include smokes for:
   - role-specific message fields,
   - user content part unknown fields with the final nested key visible,
   - content part missing `type` plus unknown field,
   - text/image content conflicts,
   - `image_url` unknown fields,
   - `tools[n]` and `tools[n].function`,
   - `tool_choice` and `tool_choice.function`,
   - assistant tool call unknown fields,
   - `stream_options` unknown fields,
   - accepted `tools[n].function.parameters` schema internals.
7. Build `ilonasin`, start a disposable daemon, smoke management health and
   snapshot routes, hit `/v1/chat/completions` with an unsupported nested field,
   then run `ilonasin manage` in a short PTY.

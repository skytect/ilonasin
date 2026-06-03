# 413 Request Image Count Boundary

## Context

Whole-codebase review found boundary drift in
`internal/server/request_metadata_images.go`: the server owns image-counting
semantics for Chat, Responses, Anthropic, and raw Codex Responses input. The
architecture says provider/API request-shape packages should own typed request
semantics, while server metadata assembly should stay thin.

Current server helpers know these local request details:

- Chat message content parts use OpenAI `image_url` parts.
- Responses input content uses `input_image` parts.
- Anthropic messages use content blocks with `type == "image"`.
- Chat requests created from Responses conversion may carry raw Codex Responses
  input items that need the same `input_image` counting semantics.

## Goal

Move request image-count semantics out of `internal/server` and into the
packages that own those request shapes, with no behavior change.

## Scope

1. Add OpenAI-owned image counting helpers.
   - `ChatRequestImageCount(req ChatCompletionRequest) int`.
   - `ResponsesRequestImageCount(req ResponsesRequest) int`.
   - Keep raw Codex Responses input counting private inside `internal/openai`.
2. Add Anthropic-owned image counting helper.
   - `RequestImageCount(req Request) int`.
3. Update server metadata builders to call those package-owned helpers.
   - Chat early and normal metadata.
   - Responses early and normal metadata.
   - Anthropic early and count-tokens metadata.
4. Delete `internal/server/request_metadata_images.go`.
5. Do not change request parsing, validation, conversion, metadata field names,
   storage schema, management DTOs, TUI rendering, routing, quota behavior,
   provider adapters, or logging policy.
6. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- Chat image count covers OpenAI Chat `image_url` parts.
- Chat image count ignores invalid/unparseable Chat content exactly as the
  current server helper did.
- Chat image count includes raw Codex Responses input `input_image` parts.
- Chat image count ignores malformed raw Codex Responses input exactly as the
  current server helper did.
- Responses image count covers typed `input_image` parts.
- Anthropic image count covers content blocks with `type == "image"`.
- Server metadata builders populate the same image counts through the new
  helpers.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/openai ./internal/anthropic ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Server no longer owns request-shape image parsing/counting logic.
- OpenAI and Anthropic packages own image counts for their local request shapes.
- Metadata image counts are behavior-preserving.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

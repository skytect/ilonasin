# 335 Anthropic Chat Conversion Boundary

## Context

`docs/ilonasin-architecture.md` expects provider-specific parsing and
conversion behavior to live behind clear adapter/request boundaries. Recent
Anthropic slices split affinity, content parsing, tool parsing, response
parsing, count-token DTOs, and option decoding out of `types.go`.

`internal/anthropic/types.go` still owns both request decoding and conversion
from Anthropic Messages to the strict OpenAI Chat request used by the shared
server/provider path. Conversion is a cohesive boundary and can move without
changing request validation or provider behavior.

## Scope

1. Keep this slice limited to `internal/anthropic` and this plan.
   - Do not touch server, provider, storage, management, TUI, config, logging,
     routing, metadata, or app files.
2. Add `internal/anthropic/chat_conversion.go`.
3. Move Anthropic-to-Chat conversion logic out of `types.go`:
   - `Request.ToChatCompletion`
   - `userMessageToChat`
   - `assistantMessageToChat`
   - `rawJSONString`
   - `mustJSON`
4. Keep request parsing helpers in `types.go`:
   - `decodeRequest`
   - `firstUnsupportedAnthropicField`
   - `decodeMessages`
   - `decodeSystem`
   - `blocksText`
   - `isJSONString`
   - `isJSONObject`
5. Preserve behavior exactly.
   - system blocks still join text with blank lines;
   - user text-only blocks still become a string Chat message;
   - user multimodal blocks still become Chat content arrays;
   - tool results still become Chat tool messages and validate against prior
     tool-use IDs;
   - assistant tool-use blocks still become OpenAI-style tool calls;
   - assistant tool-use-only content still serializes as JSON null;
   - `providerType == "codex"` still suppresses Anthropic sampling/max-token
     forwarding while preserving tools/tool_choice behavior;
   - `AffinityKey` still comes from `anthropicAffinityKey`;
   - no raw prompts, tool arguments, tool results, request bodies, provider
     payloads, or affinity keys are newly stored, logged, rendered, or exposed.
6. Do not add permanent tests or new abstractions.

## Out Of Scope

- Changing Anthropic request parsing or validation.
- Changing Count Tokens behavior.
- Changing tool parsing or tool schema conversion.
- Changing provider adapter behavior.
- Moving JSON shape helpers that are still used by parsers.
- Server/provider/storage/management/TUI changes.

## Verification

Use a temporary focused Anthropic package test, then remove it before commit,
covering:

- system string and text blocks convert to Chat system messages with the same
  joined text behavior;
- user text-only content converts to a string Chat message;
- user mixed text/image content converts to a Chat content array;
- assistant text and tool-use content converts to Chat assistant content and
  tool calls;
- assistant tool-use-only content is JSON null;
- tool_result without matching prior tool_use still errors;
- valid tool_use followed by matching tool_result produces the same Chat tool
  message;
- `providerType == "codex"` still omits `max_tokens`, `temperature`, `top_p`,
  `top_k`, and `stop`;
- `providerType == "codex"` still preserves converted `tools` and
  `tool_choice`;
- non-Codex conversion still forwards Anthropic sampling/max-token/stop fields;
- converted `PresentFields` still includes `model` and `messages`, plus
  non-Codex `max_tokens`, sampling, and `stop` fields when present;
- converted `PresentFields` still includes `tools` and `tool_choice` when
  present;
- converted `AffinityKey` is still populated from Anthropic metadata.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/anthropic
go test ./...
go vet ./...
```

The `find` output may include unrelated pre-existing permanent tests; this
slice must not add any.

Build `cmd/ilonasin`, start `ilonasin serve` with temporary `ILONASIN_HOME` and
`[server] bind = "127.0.0.1:0"`, check `/_ilonasin/manage/health` over the
management socket, run a short `ilonasin manage` TUI smoke, then clean up.

## Acceptance

- Anthropic-to-OpenAI Chat conversion lives in a focused package file.
- `types.go` remains responsible for request DTOs, top-level request
  orchestration, and message/system parsing.
- Anthropic Messages validation, conversion behavior, metadata privacy, and
  IO logging behavior are unchanged.

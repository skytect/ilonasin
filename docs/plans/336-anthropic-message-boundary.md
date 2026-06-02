# 336 Anthropic Message Boundary

## Context

`docs/ilonasin-architecture.md` expects strict request parsing with clear,
modular boundaries before routing and provider adaptation. Recent Anthropic
slices split affinity, content blocks, options, tools, responses, count-token
types, and Chat conversion out of `types.go`.

`internal/anthropic/types.go` now mostly owns DTOs, error envelopes, top-level
request decode orchestration, message/system parsing, and shared JSON shape
helpers. Message and system parsing are a cohesive request-parser boundary and
can move without changing accepted request shapes.

## Scope

1. Keep this slice limited to `internal/anthropic` and this plan.
   - Do not touch server, provider, storage, management, TUI, config, logging,
     routing, metadata, or app files.
2. Add `internal/anthropic/messages.go`.
3. Move message/system parsing helpers out of `types.go`:
   - `decodeMessages`
   - `decodeSystem`
4. Keep shared JSON shape and top-level helpers in `types.go`:
   - `decodeRequest`
   - `firstUnsupportedAnthropicField`
   - `blocksText`
   - `isJSONString`
   - `isJSONObject`
5. Preserve behavior exactly.
   - missing `messages` still errors as `messages is required`;
   - malformed messages arrays still include the wrapped JSON error;
   - empty messages arrays still error as `messages must not be empty`;
   - unsupported message fields still report deterministic nested field names;
   - unsupported message roles still report `messages[n].role is unsupported`;
   - system message content and top-level `system` still require text-only
     blocks;
   - string system content and text block arrays remain accepted;
   - text-block joins still use blank lines;
   - content-block parsing behavior is unchanged.
6. Do not add permanent tests or new abstractions.

## Out Of Scope

- Changing Anthropic content-block parsing.
- Changing Chat conversion behavior.
- Changing Count Tokens behavior.
- Moving top-level request orchestration.
- Moving JSON shape helpers used across affinity, content, tools, and message
  parsing.
- Server/provider/storage/management/TUI changes.

## Verification

Use a temporary focused Anthropic package test, then remove it before commit,
covering:

- missing `messages` error;
- malformed `messages` array error includes the existing wrapped JSON message;
- empty `messages` error;
- unsupported message field error is deterministic;
- unsupported role error;
- valid top-level system string and system text block array;
- invalid top-level system image block error;
- valid `role: "system"` message with text content;
- invalid `role: "system"` message with image content error;
- `blocksText` remains available to content parsing and Chat conversion and
  still joins text blocks with blank lines.
- Count Tokens keeps parity for missing and empty `messages`, deterministic
  `messages[0].bogus`, and invalid top-level system image block.

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

- Anthropic message/system parsing lives in a focused package file.
- `types.go` remains responsible for DTOs, error envelopes, top-level request
  decode orchestration, and shared package-level JSON shape helpers.
- Anthropic request validation, Count Tokens validation, conversion behavior,
  metadata privacy, and IO logging behavior are unchanged.

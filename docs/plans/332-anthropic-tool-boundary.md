# 332 Anthropic Tool Boundary

## Context

`docs/ilonasin-architecture.md` expects strict local request parsing and clear
unsupported-field errors before routing or provider adaptation. Plan 331 moved
Anthropic content-block decoding out of `types.go` and intentionally left tool
definition parsing, cache-control parsing, and tool-choice parsing for a later
focused slice.

`internal/anthropic/types.go` still owns top-level request decoding, conversion
to OpenAI Chat, tool definition parsing, tool choice parsing, cache-control
validation, and Chat tool conversion. Tool-specific parsing and conversion can
be split without changing behavior.

## Scope

1. Keep this slice limited to `internal/anthropic` and this plan.
   - Do not touch server, provider, storage, management, TUI, config, logging,
     routing, or metadata files.
2. Add `internal/anthropic/tools.go`.
   - Move `decodeTools`;
   - move `decodeToolChoice`;
   - move `chatTools`.
3. Keep top-level request orchestration in `types.go`.
   - `decodeRequest` still calls `decodeTools` and `decodeToolChoice`;
   - `Request.ToChatCompletion` still calls `chatTools`.
   - Keep shared `decodeCacheControl` in `types.go` for now because it is used
     by top-level requests, content blocks, and tools.
4. Preserve behavior exactly.
   - tool definitions still reject unsupported fields with deterministic names;
   - `description` remains optional string;
   - `input_schema` still must be valid JSON;
   - tool `cache_control` still uses the existing shared helper that requires
     `type: "ephemeral"` and otherwise preserves existing object behavior;
   - tool choice still accepts string or object `auto` only;
   - converted Chat tools remain OpenAI function tools with the same shape;
   - no raw tool arguments, tool results, request bodies, or provider payloads
     are newly stored, logged, rendered, or exposed.
5. Do not add permanent tests or abstractions.

## Verification

Use a temporary focused Anthropic package test, then remove it before commit,
covering:

- valid tool with `name`, `description`, `input_schema`, and `cache_control`;
- deterministic `tools[0].bogus is unsupported` error;
- missing `tools[0].input_schema` error;
- valid string `tool_choice: "auto"`;
- valid object `tool_choice: { "type": "auto" }`;
- unsupported object tool-choice field error;
- unsupported tool-choice type error;
- converted Chat tool shape is unchanged.
- Count Tokens decode parity for at least one tool-definition error and one
  tool-choice error.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/anthropic
go test ./...
go vet ./...
```

Build `cmd/ilonasin`, start `ilonasin serve` with temporary `ILONASIN_HOME` and
`[server] bind = "127.0.0.1:0"`, check `/_ilonasin/manage/health` over the
management socket, run a short `ilonasin manage` TUI smoke, and clean up.

Verify the staged patch in an isolated worktree from `HEAD` before commit.

## Acceptance

- Anthropic tool parsing/conversion lives in a focused package file.
- `types.go` remains responsible for top-level request orchestration.
- Anthropic tool validation and conversion behavior is unchanged.

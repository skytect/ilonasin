# 331 Anthropic Content Boundary

## Context

`docs/ilonasin-architecture.md` expects a strict local request parser with
clear unsupported-field errors, and it keeps local API parsing separate from
routing and provider adapters. `internal/anthropic/types.go` still mixes
top-level Messages request decoding, system/message orchestration, content
block parsing, tool parsing, cache-control parsing, conversion to OpenAI Chat,
and JSON utility helpers.

Plan 270 tightened Anthropic nested-field errors. This slice preserves that
behavior while moving content-block parsing into its own package file.

## Scope

1. Keep this slice limited to `internal/anthropic` and this plan.
   - Do not touch server, provider, storage, management, TUI, config, logging,
     routing, or metadata files.
2. Add `internal/anthropic/content.go`.
   - Move `decodeContent`;
   - move `decodeContentBlock`;
   - move `decodeImageURL`;
   - move `decodeToolResultContent`.
3. Leave request-level orchestration in `types.go`.
   - Keep `decodeMessages` in `types.go`;
   - keep `decodeSystem` in `types.go`;
   - keep tool definition parsing, cache-control parsing, and tool choice
     parsing in `types.go` for a later focused slice.
4. Preserve behavior exactly.
   - Unsupported field names remain deterministic and specific;
   - missing `type` still wins before type-specific unsupported fields;
   - accepted `cache_control` fields remain accepted;
   - tool-use input still requires valid JSON object input;
   - tool-result content still accepts string or text-block array;
   - no raw request content is newly stored, logged, rendered, or forwarded.
5. Do not add permanent tests or abstractions.

## Verification

Use a temporary focused Anthropic package test, then remove it before commit,
covering:

- string content decoding;
- text block decoding with `cache_control`;
- image URL decoding;
- tool-use input object validation;
- tool-result text-block content;
- deterministic unsupported nested field error;
- missing content block `type` precedence.
- Count Tokens decode parity for a moved content-block path, including one
  nested unsupported-field error and missing `type` precedence.

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

- Anthropic content block parsing lives in a focused package file.
- `types.go` is smaller and remains responsible for top-level request
  orchestration.
- Anthropic request validation behavior is unchanged.

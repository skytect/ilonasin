# 334 Anthropic Option Boundary

## Context

`docs/ilonasin-architecture.md` expects strict local request parsing and clean
provider-adapter boundaries. The Anthropic package has already split
metadata-affinity, content-block parsing, response parsing, count-tokens types,
and tool parsing out of `types.go`.

`internal/anthropic/types.go` still owns top-level request orchestration,
message/system parsing, OpenAI Chat conversion, top-level request value
decoders, and shared option decoders. The request value decoders and shared
option decoders are now the next cohesive boundary: some are top-level request
helpers, while cache-control is shared by top-level request fields and tools.
Content blocks currently allow `cache_control` fields but do not decode or
preserve them; this slice preserves that behavior.

## Scope

1. Keep this slice limited to `internal/anthropic` and this plan.
   - Do not touch server, provider, storage, management, TUI, config, logging,
     routing, metadata, or app files.
2. Add `internal/anthropic/options.go`.
3. Move request value decoders and shared option decoders out of `types.go`:
   - `decodeRequiredString`
   - `decodePositiveInt`
   - `decodeOptionalFloat`
   - `decodeCacheControl`
   - `decodeThinking`
   - `decodeContextManagement`
   - `decodeOutputConfig`
   - `decodeRequiredRawString`
4. Preserve behavior exactly.
   - Top-level `cache_control` and tool `cache_control` continue to require
     `type: "ephemeral"` and preserve extra object fields.
   - Content-block `cache_control` remains allowed but not decoded or
     preserved.
   - `thinking.type` continues to accept only `adaptive`, `enabled`, and
     `disabled`.
   - `context_management` and `output_config` continue to require objects and
     preserve their decoded object values.
   - Required string, positive integer, and optional float error messages stay
     byte-for-byte equivalent.
5. Keep `firstUnsupportedAnthropicField`, `decodeMessages`, `decodeSystem`, and
   Chat conversion helpers in `types.go` for now.
6. Do not add permanent tests or new abstractions.

## Out Of Scope

- Changing accepted Anthropic request fields.
- Changing Count Tokens behavior.
- Moving Chat conversion helpers.
- Moving message/system parsing.
- Forwarding Anthropic-only options upstream.
- Server/provider/storage/management/TUI changes.

## Verification

Use a temporary focused Anthropic package test, then remove it before commit,
covering:

- top-level `cache_control` valid object with extra fields is preserved;
- content-block `cache_control` remains accepted without being decoded or
  preserved;
- tool `cache_control` still accepts valid ephemeral objects and preserves
  extra fields;
- unsupported cache-control type still reports the same field-specific error;
- `thinking.type` valid and unsupported values keep current behavior;
- `context_management` and `output_config` require objects;
- required `model`, optional count-tokens `max_tokens` parsing, and invalid
  `temperature` errors keep current behavior.
- Count Tokens keeps parity for valid top-level `cache_control`, unsupported
  `thinking.type`, object-required `context_management` or `output_config`, and
  omitted `max_tokens`.

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
management socket, run a short `ilonasin manage` TUI smoke, then clean up.

## Acceptance

- Shared Anthropic option/value decoding lives in a focused package file.
- `types.go` remains responsible for top-level request orchestration,
  message/system parsing, and Chat conversion.
- Anthropic request validation, Count Tokens validation, provider conversion,
  metadata privacy, and IO logging behavior are unchanged.

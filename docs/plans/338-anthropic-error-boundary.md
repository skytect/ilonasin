# 338 Anthropic Error Boundary

## Context

`docs/ilonasin-architecture.md` expects provider-specific response and error
shapes to stay behind provider/API boundaries, with clear local validation and
normalization before routing. Recent Anthropic slices split request decoding,
message parsing, content parsing, options, tools, responses, count-token DTOs,
affinity, and Chat conversion out of `types.go`.

`internal/anthropic/types.go` now owns DTOs, Anthropic error envelopes, error
factory helpers, and shared package parser helpers. Error envelopes and
status-to-Anthropic-error-type mapping are a distinct response boundary and can
move without changing server behavior. This is a small terminal cleanup for the
current Anthropic `types.go` split, leaving that file as DTOs plus shared
parser helpers rather than a behavioral improvement.

## Scope

1. Keep this slice limited to `internal/anthropic` and this plan.
   - Do not touch server, provider, storage, management, TUI, config, logging,
     routing, metadata, or app files.
2. Add `internal/anthropic/errors.go`.
3. Move Anthropic error envelope types and helpers out of `types.go`:
   - `ErrorEnvelope`
   - `ErrorBody`
   - `Error`
   - `ErrorForStatus`
   - `ErrorWithType`
4. Keep request/response DTOs and shared parser helpers in their current files.
5. Preserve behavior exactly.
   - `Error(message)` still returns top-level `type: "error"` and nested
     `type: "invalid_request_error"`;
   - `ErrorForStatus` still maps `401` and `403` to
     `authentication_error`;
   - `ErrorForStatus` still maps `404` to `not_found_error`;
   - `ErrorForStatus` still maps `429` to `rate_limit_error`;
   - `ErrorForStatus` still maps `>=500` to `api_error`;
   - all other statuses still map to `invalid_request_error`;
   - messages are preserved unchanged;
   - JSON field names and envelope shape are unchanged.
6. Do not add permanent tests or new abstractions.

## Out Of Scope

- Changing server route error writing.
- Changing status codes or error messages.
- Changing OpenAI error envelopes.
- Changing Anthropic response conversion.
- Moving request DTOs, parser helpers, or count-token DTOs.

## Verification

Use a temporary focused Anthropic package test, then remove it before commit,
covering:

- `Error` envelope shape and default type;
- `ErrorWithType` preserves explicit type and message;
- `ErrorForStatus` mappings for `400`, `401`, `403`, `404`, `429`, `500`, and
  another non-5xx status;
- JSON marshaling still emits top-level `type` and nested `error.type` and
  `error.message` fields.
- temporary route/auth envelope parity for representative Anthropic Messages
  and Count Tokens paths:
  - local auth `401`;
  - invalid request `400`;
  - provider-not-configured `404`;
  - feasible `5xx` preflight or upstream failure path;
  - assert status plus Anthropic JSON envelope shape and error type.

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

- Anthropic error envelope construction lives in a focused package file.
- `types.go` remains responsible for request DTOs and shared parser helpers.
- Server Anthropic error response behavior, metadata privacy, and IO logging
  behavior are unchanged.

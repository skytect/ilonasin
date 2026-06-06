# 509 Codex Prompt Cache Key

## Context

Plan 499 found that Codex request building always sends a generated
`ids.ThreadID` as upstream `prompt_cache_key`. The Responses decoder already
parses and safety-filters a client-provided `prompt_cache_key`, but conversion
uses it only for local credential affinity.

`docs/ilonasin-architecture.md` and `docs/codex-compatibility-audit.md` both
document safe body `prompt_cache_key` as the preferred Codex cache-locality
signal when present. Generated local thread IDs should remain a fallback for
clients that omit the field.

## Goal

Forward safe client-provided Responses `prompt_cache_key` to Codex upstream
requests, using generated thread IDs only when no safe client key is present.

## Scope

1. Add a Responses-specific internal `ChatCompletionRequest` field for Codex
   upstream prompt-cache propagation.
2. Carry the already-sanitized Responses `PromptCacheKey` through
   `ResponsesRequest.ToChatCompletionRequest` into that internal field.
3. Update Codex Responses marshaling so `codexResponsesRequest.PromptCacheKey`
   uses the Responses-specific internal field when present, otherwise
   `ids.ThreadID`.
4. Preserve generated Codex transport headers (`session-id`, `thread-id`,
   `x-client-request-id`, `x-codex-window-id`) and generated fallback behavior.
5. Preserve local affinity priority, metadata handling, storage, management
   APIs, routing, TUI, logging, provider auth, and IO logging behavior.
6. Do not add permanent tests.

## Out Of Scope

- Using `client_metadata` values as upstream `prompt_cache_key`.
- Changing Chat Completions `prompt_cache_key` behavior.
- Changing Codex header generation.
- Changing credential affinity safety filters.

## Verification

Use a temporary focused harness, then remove it before commit, to verify:

- Responses conversion carries a safe top-level `prompt_cache_key` into
  the Responses-specific internal `ChatCompletionRequest` field.
- Codex marshaling forwards that Responses-specific internal value as JSON
  `prompt_cache_key`.
- Codex marshaling falls back to generated `ids.ThreadID` when the request key
  is absent.
- Unsafe or invalid prompt-cache values remain filtered out by the existing
  Responses decoder path.
- Chat Completions `PromptCacheKey` does not affect Codex Responses
  `prompt_cache_key` in this slice.

Run:

```sh
rg -n 'prompt_cache_key|PromptCacheKey|ThreadID|marshalCodexResponsesRequest|ToChatCompletionRequest' internal/openai/responses.go internal/openai/types.go internal/provider/codex_responses_request.go internal/provider/codex_responses.go internal/provider/codex_responses_stream.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smoke:

1. Build a temporary `ilonasin` binary.
2. Start `ilonasin serve` with isolated `ILONASIN_HOME`, temporary config,
   temporary SQLite, IO capture disabled, and keepalive disabled.
3. Verify management health and snapshot over the Unix management socket.
4. Run bounded `ilonasin manage` at 80 and 140 columns under a pseudo-terminal.
5. Remove all temporary files and terminate the daemon.

## Acceptance

- Safe Responses `prompt_cache_key` reaches Codex upstream request JSON.
- Generated thread IDs remain the fallback upstream `prompt_cache_key`.
- Local affinity and generated transport header behavior are preserved.
- Chat Completions `prompt_cache_key` behavior is unchanged.
- No permanent tests are added.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

## Implementation Record

- Added an internal `CodexPromptCacheKey` field to `ChatCompletionRequest`.
- Set that field only during Responses-to-chat conversion from the already
  safety-filtered Responses `PromptCacheKey`.
- Updated Codex Responses request marshaling to prefer `CodexPromptCacheKey`
  for JSON `prompt_cache_key`, falling back to the generated thread ID.
- Preserved generated Codex transport headers, local affinity priority, storage,
  management APIs, routing, TUI, logging, provider auth, and IO logging
  behavior.

## Verification Record

- Senior plan review: two reviewers reported no findings; one reviewer found
  that using `ChatCompletionRequest.PromptCacheKey` would also change Chat
  Completions Codex behavior, so the plan was corrected to use a
  Responses-specific internal field before implementation.
- Temporary focused harnesses: passed for Responses conversion carrying a safe
  prompt-cache key, unsafe prompt-cache filtering, Codex marshaling forwarding
  `CodexPromptCacheKey`, generated thread ID fallback, and Chat Completions
  `PromptCacheKey` not affecting Codex Responses JSON `prompt_cache_key`.
  Temporary harnesses were removed before commit.
- `rg -n 'prompt_cache_key|PromptCacheKey|ThreadID|marshalCodexResponsesRequest|ToChatCompletionRequest|CodexPromptCacheKey' internal/openai/responses.go internal/openai/types.go internal/provider/codex_responses_request.go internal/provider/codex_responses.go internal/provider/codex_responses_stream.go`:
  passed.
- `git diff --check`: passed.
- `find . -name '*_test.go' -type f -print`: passed, no files found.
- `go test ./...`: passed as a compile/package check; all packages reported no
  test files.
- `go vet ./...`: passed.
- Temporary `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`: passed.
- `ilonasin serve` smoke: passed with isolated `ILONASIN_HOME`, temporary
  config, free local bind port, IO capture disabled, keepalive disabled, and
  management health plus snapshot checked over the Unix socket.
- `ilonasin manage` smoke: passed at 80 and 140 columns under a pseudo-terminal.
- Senior implementation review: initial review found duplicate Scope numbering;
  numbering was corrected. The other two reviewers reported no findings.
- Cleanup: temporary home, binary, config, harnesses, terminal captures, marker
  files, and daemon process were removed.

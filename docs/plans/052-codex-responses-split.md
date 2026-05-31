# 052 Codex Responses Split

## Context

After plan 051, OpenRouter option validation lives in its own provider package
file. `internal/provider/http_chat.go` still contains the shared HTTP chat
adapter plus the full Codex `/responses` implementation:

- Codex non-streaming `/responses` request construction and SSE aggregation,
- Codex streaming `/responses` normalization into OpenAI-compatible chat
  chunks,
- Codex usage parsing,
- Codex stream chunk marshaling and unsupported tool-event detection.

The architecture says provider adapters own provider-specific behavior. Keeping
this Codex-specific block inside the shared HTTP chat flow makes the provider
adapter harder to review and extend. This slice is a behavior-preserving split.

## Scope

1. Move Codex `/responses` helpers from `internal/provider/http_chat.go` into a
   new same-package file, `internal/provider/codex_responses.go`.
2. Keep `CompleteChat` and `StreamChat` dispatch behavior unchanged:
   - Codex still calls `completeCodexChat` and `streamCodexChat`,
   - DeepSeek and OpenRouter still use the existing chat-completions path.
3. Keep all Codex request shapes, response parsing, stream chunk shapes, error
   classes, timeout behavior, retry-after handling, usage mapping, and marker
   privacy behavior unchanged.
4. Keep storage, credentials, OAuth refresh, server routing, TUI, model
   discovery, OpenRouter behavior, DeepSeek behavior, and smoke harness logic
   unchanged.
5. Do not add request fields, response fields, provider features, migrations,
   persistence, or permanent tests.

## Implementation

1. Create `internal/provider/codex_responses.go`.
2. Move this Codex-specific cluster intact:
   - `MaxCodexAggregateAssistantBytes`
   - `completeCodexChat`
   - `codexResponsesRequest`
   - `codexResponseItem`
   - `codexContentItem`
   - `marshalCodexResponsesRequest`
   - `codexResponsesResult`
   - `readCodexResponses`
   - `codexEventFailure`
   - `codexEventErrorClass`
   - `handleCodexEvent`
   - `codexFinalText`
   - `classifyCodexReadError`
   - `localChatCompletionID`
   - `streamCodexChat`
   - `codexStreamState`
   - `includeStreamUsage`
   - `readCodexStream`
   - `handleCodexStreamEvent`
   - `codexUsagePayload`
   - `codexUsageFromResponse`
   - `codexToolEvent`
   - `writeCodexRoleChunk`
   - `writeCodexContentChunk`
   - `writeCodexFinishChunk`
   - `writeCodexUsageChunk`
   - `marshalCodexStreamChunk`
   - `marshalCodexUsageChunk`
   - `maxCodexAggregateBytes`
3. Leave shared transport helpers in `http_chat.go`, including:
   - `CompleteChat`
   - `StreamChat`
   - `doStreamRequest`
   - `readStream`
   - `joinBasePath`
   - `retryAfterFromHeader`
   - common chat-completions request marshaling
   - model discovery and capability normalization.
4. Run `gofmt` on touched Go files.
5. Manually review the diff before smoke checks. This should be a move plus
   import cleanup, not a logic rewrite.

## Smoke Checks

Run these direct checks before code review:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
tmpbin="$(mktemp -d)"
go build -o "$tmpbin/ilonasin" ./cmd/ilonasin
tmp="$(mktemp -d)"
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" serve --check
ILONASIN_HOME="$tmp" "$tmpbin/ilonasin" manage --check
rm -rf "$tmp" "$tmpbin"
```

`go test ./...` is only a compile/package check. No permanent test files will
be added.

## Review Questions

1. Is moving the whole Codex `/responses` block the right-sized next adapter
   split?
2. Should `maxCodexAggregateBytes` move with the Codex code even though the
   backing field remains on `HTTPChatAdapter`?
3. Are existing compile, vet, build, and CLI smoke checks enough for a
   behavior-preserving provider-file extraction?

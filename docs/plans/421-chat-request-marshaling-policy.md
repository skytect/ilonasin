# 421 Chat Request Marshaling Policy

## Context

`internal/provider/chat_request.go` still drives provider-specific Chat request
marshaling through raw provider-type switches:

- DeepSeek maps local `max_completion_tokens` to upstream `max_tokens`.
- OpenRouter keeps local `max_completion_tokens` as upstream
  `max_completion_tokens`.
- DeepSeek flattens `provider_options.deepseek.thinking`,
  `reasoning_effort`, and `user_id` into top-level upstream fields.
- OpenRouter flattens `provider_options.openrouter.reasoning`, `models`,
  `cache_control`, and `provider` into top-level upstream fields.

`docs/ilonasin-architecture.md` says provider adapters own provider-specific
request translation and explicit namespaced provider escape hatches. Recent
slices made Chat validation, Chat option metadata, and stream parsing
policy-driven. Request marshaling should use the same explicit provider-local
boundary while preserving exact outgoing JSON and error behavior.

## Goal

Introduce an explicit provider-local Chat marshaling policy for
`marshalChatCompletionsRequest`, preserving all existing request translation,
validation dispatch, and error strings.

## Scope

1. Add a small provider-local policy type in
   `internal/provider/chat_request.go`.
   - It should include the provider-options namespace used for
     `validateProviderOptions`.
   - It should encode the upstream token field for local
     `MaxCompletionTokens`.
   - It should encode which provider-options keys are flattened to top-level
     upstream request fields.
   - The zero value should represent unsupported marshaling for optional
     provider-specific fields.
2. Add a helper such as `chatRequestMarshalingPolicyForProviderType`.
   - DeepSeek policy:
     - provider-options namespace `deepseek`;
     - `MaxCompletionTokens` writes upstream `max_tokens`;
     - flattens `thinking`, `reasoning_effort`, and `user_id`.
   - OpenRouter policy:
     - provider-options namespace `openrouter`;
     - `MaxCompletionTokens` writes upstream `max_completion_tokens`;
     - flattens `reasoning`, `models`, `cache_control`, and `provider`.
   - Codex and unknown provider types return the zero/unsupported policy for
     this Chat Completions marshaling path.
3. Rewrite `marshalChatCompletionsRequest` to apply the policy instead of
   switching on raw provider type.
   - Preserve the current fast path when neither `provider_options` nor
     `MaxCompletionTokens` is present.
   - Preserve the current validation call to `validateProviderOptions` before
     flattening provider options.
   - Preserve exact error strings:
     `max_completion_tokens is not supported for %s` and
     `provider_options is not supported for %s`.
4. Keep `validateProviderOptions(providerType string, req ...)` unchanged for
   this slice because both validation and marshaling use it as the provider
   namespace and dispatcher.
5. Keep upstream auth, HTTP transport, streaming, model discovery, storage,
   management, TUI, logging, routing, config, credentials, and public response
   behavior unchanged.
6. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- DeepSeek `MaxCompletionTokens` still writes upstream `max_tokens` and does not
  write `max_completion_tokens`.
- OpenRouter `MaxCompletionTokens` still writes upstream
  `max_completion_tokens`.
- Codex or unknown provider type with `MaxCompletionTokens` still returns
  `max_completion_tokens is not supported for <provider>`.
- DeepSeek `provider_options.deepseek` still flattens `thinking`,
  `reasoning_effort`, and `user_id`, and does not emit the local-only
  `provider_options` key upstream.
- OpenRouter `provider_options.openrouter` still flattens `reasoning`,
  `models`, `cache_control`, and `provider`, and does not emit the local-only
  `provider_options` key upstream.
- Wrong provider-options namespace still returns
  `provider_options must contain only <provider>` for both DeepSeek and
  OpenRouter.
- Codex with `provider_options.codex`, or an unknown provider type with a
  matching provider-options namespace, still returns
  `provider_options is not supported for <provider>`. Wrong namespaces continue
  to fail namespace validation first.
- The no-options/no-`MaxCompletionTokens` fast path still returns the same body
  from `openai.MarshalUpstreamChatRequest`.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/provider
go test ./...
go vet ./...
```

Finally build a temporary `ilonasin` binary and smoke `ilonasin serve` plus
bounded `ilonasin manage` runs at narrow and wide terminal widths against an
isolated temporary `ILONASIN_HOME`, then remove all temporary files.

## Non-Goals

- No new provider-options behavior.
- No change to Chat validation policy.
- No change to provider-options validators.
- No change to Codex Responses or Codex Chat request builders.
- No server, storage, management, TUI, routing, credential, config, logging, or
  model-discovery changes.
- No permanent test files.

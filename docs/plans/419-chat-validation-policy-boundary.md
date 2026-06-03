# 419 Chat Validation Policy Boundary

## Context

`internal/provider/chat_validation.go` owns adapter-side Chat feature
validation, but `ValidateChatRequest` still embeds provider-specific validation
decisions directly in a large `switch instance.Type`:

- DeepSeek-only rejections for OpenRouter-only fields, selected top-level
  fields, and multimodal content;
- OpenRouter support for JSON Schema response format;
- Codex-only rejection lists, top-level service-tier validation, tool-choice
  narrowing, and tool transcript checks;
- raw provider-type strings passed into `validateChatResponseFormat` and
  `validateProviderOptions`.

`docs/ilonasin-architecture.md` requires strict local validation and says
provider adapters own provider-specific behavior and reject unsupported features
clearly. The current code does that behaviorally, but the provider-specific
policy is implicit in one control-flow block, which makes future feature
validation harder to review.

## Goal

Introduce an explicit provider-local Chat validation policy for
`ValidateChatRequest`, preserving all existing accepted/rejected fields,
rejection order, and error strings.

## Scope

1. Add a small provider-local policy type in
   `internal/provider/chat_validation.go`.
   - It should include the provider options namespace used for
     `validateProviderOptions`.
   - It should encode whether JSON Schema response format is allowed.
   - It should encode provider-specific rejected field groups.
   - It should encode DeepSeek multimodal rejection.
   - It should encode Codex top-level service-tier validation.
   - It should encode Codex `tool_choice` narrowing.
   - It should encode Codex tool transcript validation.
2. Add a helper such as `chatValidationPolicyForInstance(instance Instance)`.
   - DeepSeek, OpenRouter, and Codex return policies equivalent to today's
     switch branches.
   - Unknown or empty provider types return unsupported chat validation with the
     same error string as today:
     `provider type %q does not support chat validation`.
3. Rewrite `ValidateChatRequest` to apply the policy in the same order as the
   current branch logic:
   - common rejected fields, currently empty;
   - provider-specific rejected fields;
   - DeepSeek OpenRouter-only field rejection;
   - DeepSeek multimodal rejection;
   - response-format validation;
   - Codex top-level service tier validation;
   - Codex tool-choice narrowing;
   - Codex tool transcript validation;
   - strict tool rejection;
   - provider-options validation.
4. Change `validateChatResponseFormat` to accept an explicit allow-JSON-Schema
   boolean instead of a raw provider-type string.
5. Keep `validateProviderOptions(providerType string, req ...)` unchanged for
   this slice because request marshaling also uses it as the provider-options
   namespace and dispatcher.
6. Keep request marshaling, upstream transport, model discovery, streaming,
   storage, management, TUI, logging, routing, config, credentials, and public
   response behavior unchanged.
7. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- DeepSeek still rejects OpenRouter-only fields with the same field-specific
  first error.
- DeepSeek still rejects multimodal content.
- OpenRouter still allows `response_format.type=json_schema`.
- DeepSeek still rejects `response_format.type=json_schema`.
- Codex still rejects the same first top-level unsupported field from its
  rejection list.
- Codex still accepts top-level `service_tier` values `default`, `priority`,
  and `flex`, and rejects `auto`.
- Codex still rejects non-`auto` `tool_choice`.
- Codex still rejects invalid tool transcript ordering.
- All providers still reject strict tools.
- Provider options validation is still called with the provider namespace:
  DeepSeek, OpenRouter, and Codex each still reject a wrong
  `provider_options` namespace with the same
  `provider_options must contain only <provider>` error.
- Strict tool rejection still happens before provider-options validation: a
  request containing both a strict tool and an invalid or wrong
  `provider_options` namespace returns the strict-tool error first.
- Unknown provider type returns the same unsupported validation error string.

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

- No changes to what any provider accepts or rejects.
- No changes to provider-options validation internals or request marshaling.
- No new provider features.
- No response-format behavior changes.
- No server, storage, management, TUI, routing, credential, config, logging, or
  model-discovery changes.
- No permanent test files.

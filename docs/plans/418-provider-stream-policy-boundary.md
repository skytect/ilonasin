# 418 Provider Stream Policy Boundary

## Context

Whole-codebase review still shows raw provider-type strings threaded through
provider stream parsing:

- `internal/provider/http_stream.go` passes `req.Instance.Type` into
  `readStream`.
- `readStream` passes that raw string into `handleStreamEvent`.
- `handleStreamEvent` uses it for OpenRouter stream cost extraction.
- `streamErrorClass` uses it for OpenRouter credit-error classification.

`docs/ilonasin-architecture.md` says provider adapters own provider-specific
behavior, including stream normalization, provider error normalization, and
token/cost/cache metadata extraction. The provider adapter can still select
provider-specific behavior from the configured provider instance, but internal
stream parsing should use an explicit provider-owned policy instead of passing a
magic provider-type string through multiple layers.

## Goal

Replace the provider stream parser's raw provider-type string parameter with an
explicit provider-owned stream policy, preserving all streaming behavior and
public response shapes.

## Scope

1. Add a small provider-local stream policy type in
   `internal/provider/http_stream.go`.
   - It should encode whether OpenRouter stream cost extraction is enabled.
   - It should encode whether OpenRouter credit-text stream errors map to
     `insufficient_quota`.
   - The zero value must preserve default/non-OpenRouter behavior.
2. Add a helper such as `streamPolicyForInstance(instance Instance)`.
   - OpenRouter enables cost extraction and credit-text quota classification.
   - DeepSeek, Codex, unknown, and empty provider types use the zero policy.
3. Change `readStream`, `handleStreamEvent`, and `streamErrorClass` to accept
   the policy type instead of `providerType string`.
4. Keep request marshaling, upstream auth, logging attributes, stream SSE
   writing, normalized OpenAI chunks, usage extraction, error status mapping,
   IO logging, quota/fallback metadata, server stream error exposure policy,
   config, storage, management, and TUI behavior unchanged.
5. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- Default policy preserves generic stream error classification.
- Default and OpenRouter policies both preserve generic stream error
  classification:
  - `429` code/status and rate-limit text map to `rate_limit_exceeded`;
  - `402`, `payment required`, `insufficient_quota`, and
    `insufficient balance` map to `insufficient_quota`.
- OpenRouter policy still maps stream error text containing `credits` to
  `insufficient_quota`.
- Default policy does not map `credits` text to `insufficient_quota`.
- OpenRouter policy still extracts stream cost microunits from usage chunks.
- Non-OpenRouter policy still records the same usage token counts for the same
  usage chunk while leaving `CostMicrounits` at zero.

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

- No new stream error classes.
- No public stream response changes.
- No server-side stream error exposure changes.
- No request metadata, quota, fallback, health, logging, storage, management,
  TUI, config, credential, or model-discovery changes.
- No cross-package provider capability abstraction.
- No permanent test files.

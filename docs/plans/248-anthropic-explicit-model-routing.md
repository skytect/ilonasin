# Plan 248: Anthropic explicit model routing

## Goal

Remove the Anthropic Messages route's hidden Claude-name fallback to Codex
`gpt-5.5`. `/v1/messages` should keep working for explicitly addressed models
such as `codex/gpt-5.5`, but bare Claude-family names should not silently pick a
Codex provider or a different model.

## Ground Truth

- `docs/ilonasin-architecture.md` says clients address models as
  `<provider_instance_id>/<provider_model_id>`.
- The architecture also says routing is explicit by default, with no hidden
  cross-provider or cross-model fallback.
- `internal/server/anthropic_route.go` currently accepts bare aliases such as
  `sonnet` and `claude-opus-*` when exactly one Codex chat provider exists, then
  routes them to hardcoded `gpt-5.5`.
- Plans 191 and 222 added that behavior for Claude Code compatibility, but it
  now conflicts with the target architecture.

## Scope

1. Remove `anthropicCodexFallbackModel`.
2. Remove `isAnthropicCodexFallbackAlias`.
3. Replace `resolveAnthropicModelAddress` with explicit model-address
   resolution only. It must call `routing.ParseModelAddress` directly, not
   `s.resolveModelAddress`, because the generic resolver can still resolve
   unique bare model names from the model cache.
4. Preserve existing error behavior for invalid or unconfigured addressed
   models.
5. Apply the same explicit behavior to `/v1/messages/count_tokens`, since it
   already calls `resolveAnthropicModelAddress`.
6. Bare Claude Code model invocations such as `--model sonnet` are expected to
   fail with an Anthropic-shaped `400`; callers must use an explicit model such
   as `codex/gpt-5.5`.
7. Do not change Anthropic request decoding, Anthropic response shape,
   authentication, provider adapters, management API, storage, config, TUI, or
   IO logging policy.
8. Do not add permanent tests.

## Verification

- Temporary focused smoke:
  - `codex/gpt-5.5` resolves through `resolveAnthropicModelAddress`.
  - `codex/gpt-5.5` still reaches the `/v1/messages` dispatch path and returns
    an Anthropic-shaped response or upstream-error envelope.
  - bare `sonnet` and `claude-opus-4-8` return Anthropic-shaped `400`
    `invalid_model` errors before provider dispatch.
  - a cached unique bare model such as `gpt-5.5` is still rejected by
    `/v1/messages` and `/v1/messages/count_tokens`.
  - multiple Codex providers do not trigger an ambiguity branch because there is
    no fallback branch.
  - `/v1/messages/count_tokens` rejects the same bare Claude aliases with
    Anthropic-shaped `400` errors.
  - Remove the temporary smoke before commit.
- `find . -name '*_test.go' -type f -print`
- `git diff --check`
- `go test ./...`
- `go vet ./...`
- `go build -o "$tmpbin/ilonasin" ./cmd/ilonasin`
- Start a temporary daemon and smoke:
  - management health over the Unix socket,
  - `ilonasin manage` under a short PTY timeout,
  - `/v1/messages/count_tokens` with bare `sonnet` returns an Anthropic-shaped
    `400`,
  - `/v1/messages/count_tokens` with `codex/gpt-5.5` returns `200` after a
    temporary local client token is created through the management API,
  - `/v1/messages` with explicit `codex/gpt-5.5` reaches route dispatch; if no
    upstream credential exists in the temporary home, the expected result is an
    Anthropic-shaped credential error rather than `invalid_model`.

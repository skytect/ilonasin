# 402 Affinity Signal Provenance

## Context

Credential pooling now has a source-backed signal map in
`docs/ilonasin-architecture.md`: Codex CLI Responses sends
`prompt_cache_key`, Claude Code Anthropic sends session metadata, generic
OpenAI-compatible clients often send only model plus messages or input, and
minimal clients may provide no stable request-level identifier beyond the local
ilonasin API token.

The implementation already follows that policy:

- Chat uses safe optional body fields and selected metadata keys.
- Responses prefers safe `prompt_cache_key`, then selected safe metadata.
- Anthropic conversion uses the Claude Code session metadata shape.
- Server header affinity is body-last fallback only.
- Empty affinity still spreads by local token, provider/model route, pressure,
  and token-scoped cursor.

The next risk is not a missing field. It is future drift from treating request
IDs, window IDs, installation IDs, account IDs, device IDs, token-like values,
or prompt/message/input bodies as credential affinity.

## Goal

Make the client-signal provenance explicit at the code boundaries that extract
or consume credential affinity, without changing runtime routing behavior.

## Scope

1. Add short comments near Chat, Responses, Anthropic, header-fallback, and
   empty-affinity credential-pool code paths.
2. State that `prompt_cache_key` is used because Codex CLI sends it in the
   audited Responses path, not because generic clients are expected to send it.
3. State that generic and minimal clients may have empty affinity, and must
   still spread through the local token plus provider/model pressure cursor.
4. State that request IDs, window IDs, installation IDs, account/device IDs,
   token-like values, prompts, messages, input, raw bodies, and tool payloads
   must not become generic credential-affinity signals.
5. Do not change request parsing, validation, provider adapters, credential
   ordering, pressure tracking, quota behavior, fallback metadata, storage,
   management routes, TUI, config, IO logging, public routes, or metadata
   fields.

## Verification

Use temporary focused checks, then remove them before commit:

- Chat with only `model` and `messages` has empty affinity.
- Responses with only `model` and `input` has empty affinity.
- Anthropic with no supported metadata has empty converted affinity.
- Codex-style safe Responses `prompt_cache_key` is accepted.
- `x-client-request-id`, `x-codex-window-id`, account/device/install/token-like
  values, and unlisted metadata keys do not become affinity.
- Empty affinity still uses the token/provider/model least-pressure cursor
  path.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/openai ./internal/anthropic ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- The implemented affinity policy is easier to audit against the architecture
  signal table.
- No runtime routing behavior changes.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

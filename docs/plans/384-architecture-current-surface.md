# 384 Architecture Current Surface

## Context

`docs/ilonasin-architecture.md` is the target architecture for this goal, but
parts of it are now stale compared with the codebase and supporting docs:

- the OpenAI-compatible surface still says the first API is only `/v1/models`,
  `/v1/chat/completions`, and streaming chat completions;
- the provider adapter section lists only `deepseek` and `openrouter`, despite
  Codex being implemented and documented;
- the MVP section omits Responses and Anthropic-compatible routes that are now
  first-class local surfaces;
- Deferred Research still lists already-decided or already-implemented basics.

Current code exposes:

- `GET /models`;
- `POST /responses`;
- `POST /v1/responses`;
- `POST /v1/messages`;
- `POST /v1/messages/count_tokens`.

Supporting docs also describe Responses and Anthropic compatibility in
`docs/codex-compatibility-audit.md`, `docs/codex-endpoints.md`,
`docs/deepseek-api.md`, and `docs/deepseek-openrouter-comparison.md`.

## Goal

Update `docs/ilonasin-architecture.md` so the target architecture reflects the
current intended local API surface and provider set, without changing runtime
code.

## Scope

1. Rename or split the current OpenAI-compatible surface section into a broader
   local API surface section with distinct groups:
   - OpenAI-compatible: `GET /models` as a root compatibility alias,
     `GET /v1/models`, `POST /v1/chat/completions`, and streaming chat
     completions;
   - Responses-compatible: `POST /responses` and `POST /v1/responses`;
   - Anthropic-compatible: `POST /v1/messages` and
     `POST /v1/messages/count_tokens`.
2. Clarify that unsupported fields still return explicit local errors and that
   provider-specific escape hatches remain namespaced.
3. Update the provider adapter boundary initial provider list to include
   `codex`.
4. Update the conceptual flow or side-plane wording only where needed to mention
   Responses and Anthropic-compatible request conversion as local API surfaces.
5. Update Deferred Research and Open Questions to remove already-settled items
   only when the current code/docs prove them settled, and keep genuinely open
   questions.
6. Update MVP Target to include current Responses and Anthropic-compatible local
   routes.

## Out Of Scope

- No runtime code changes.
- No route, provider adapter, validation, storage, management, TUI, config,
  logging, or smoke behavior changes.
- No broad rewrite of historical plan docs.
- No claims that all Responses or Anthropic provider features are fully
  supported; the architecture should state local compatibility is bounded by
  explicit validation and conversion.

## Verification

Run:

```sh
rg -n "OpenAI-Compatible Surface|/responses|/v1/responses|/v1/messages|count_tokens|Initial provider types|MVP Target|Deferred Research|Open Questions" docs/ilonasin-architecture.md
rg -n "HandleFunc|/responses|/v1/responses|/v1/messages|/v1/messages/count_tokens" internal/server
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- `docs/ilonasin-architecture.md` no longer describes the implemented local API
  surface as chat-only.
- The architecture names Codex as an initial provider type.
- The MVP target includes current Responses and Anthropic-compatible routes.
- The document avoids overstating unsupported feature parity.
- Compile/package checks, vet, and direct serve/manage smokes still pass.

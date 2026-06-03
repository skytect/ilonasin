# 395 Bounded Tool Parity Architecture

## Context

`docs/ilonasin-architecture.md` already says Responses and Anthropic routes are
bounded local compatibility surfaces, not claims of universal upstream feature
parity. It still keeps an open question asking how much tool-family parity is
necessary beyond current conversion paths.

Current code and audit docs already answer the current architecture:

- OpenAI Responses function tools are converted into the strict local Chat model
  when representable.
- Codex provider routing may preserve validated raw Codex-native Responses tool
  declarations instead of forcing them through Chat-tool conversion.
- Non-Codex Chat-adapter provider paths filter or reject tool families that
  cannot be represented safely.
- Unsupported hosted, deferred, namespaced, MCP, shell, tool-search, and broader
  custom tool semantics remain unproven and are deferred research.
- Anthropic Messages tools convert to local Chat function tools only where
  representable, and unsupported fields fail locally.

The architecture should state the current boundary directly instead of leaving a
broad parity policy question open.

## Goal

Clarify that current local Responses and Anthropic compatibility requires only
the implemented, representable conversion paths plus explicit local rejection of
unsupported tool families.

## Scope

1. Expand the Local API Surface wording in `docs/ilonasin-architecture.md`:
   - local compatibility is bounded;
   - representable function-tool paths are supported;
   - validated Codex-native Responses tool declarations may be preserved for
     Codex routing where implemented;
   - unsupported hosted/deferred/namespaced/MCP/shell/tool-search families must
     not be silently flattened or forwarded to Chat-adapter providers;
   - unsupported transcript/output families outside the implemented relay paths
     must be rejected or deferred rather than converted lossy.
2. Keep Deferred Research for provider adapter strategy covering
   hosted/deferred/namespaced tool families.
3. Remove the broad open question about how much parity is necessary beyond
   current paths.

## Out Of Scope

- Runtime behavior changes.
- New tool-family support.
- Provider adapter, route, request validation, TUI, management, config, storage,
  or logging changes.
- Live Codex/OpenRouter smokes.
- Permanent tests.

## Verification

Run:

```sh
rg -n "Responses and Anthropic|tool|hosted|deferred|namespaced|MCP|shell|tool-search|custom_tool|responsesToolsToChatTools|Anthropic" docs/ilonasin-architecture.md docs/codex-compatibility-audit.md internal/openai internal/anthropic internal/provider
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke even though this is
docs-only, to keep the slice discipline consistent.

## Acceptance

- Architecture no longer treats broad tool parity as an active open question.
- Architecture preserves the stricter compatibility boundary: support only
  representable/current paths, preserve validated raw Codex tools only on Codex
  routes where implemented, and reject or defer the rest.
- Deferred research still tracks concrete hosted/deferred/namespaced provider
  adapter strategy.
- No runtime behavior changes.

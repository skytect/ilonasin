# 414 Server Provider Policy Boundary

## Context

Whole-codebase review flagged scattered server-owned provider policy selectors:

- `internal/server/responses_conversion.go` selects Responses conversion policy
  by provider type.
- `internal/server/anthropic_conversion.go` selects Anthropic-to-Chat conversion
  policy by provider type.
- `internal/server/chat_stream.go` selects stream error exposure policy by
  provider type.
- `internal/server/credentials.go` decides whether Codex OAuth refresh is
  available and whether chat/stream/model-credential 401s should retry OAuth
  refresh.
- `internal/server/models.go` decides whether model discovery 401 should retry
  OAuth refresh.

These are explicit and behaviorally correct, but the provider-type selectors are
spread across server execution files. The architecture expects the router core to
keep provider-specific quirks explicit and auditable rather than scattered.

## Goal

Group server-local provider policy selectors into one boundary without changing
runtime behavior.

## Scope

1. Add `internal/server/provider_policy.go` for server-owned provider policy
   helpers.
2. Move these existing helpers into that file unchanged in behavior:
   - `responsesConversionPolicy`.
   - `anthropicConversionPolicy`.
   - `streamErrorExposurePolicy` and `streamErrorExposurePolicyFor`.
   - `canRefreshCodexOAuth`.
   - `shouldRefreshOAuthAfterChat401`.
   - `shouldRefreshOAuthAfterStream401`.
   - `shouldRefreshModelCredentialAfterChat401`.
   - `shouldRefreshModelCredentialAfterStream401`.
   - `shouldRefreshOAuthAfterModel401`.
3. Keep chat/stream/model execution code calling the same helper names.
4. Remove now-empty policy files if they become unnecessary:
   - `internal/server/responses_conversion.go`.
   - `internal/server/anthropic_conversion.go`.
5. Do not change credential resolution, OAuth refresh behavior, routing,
   fallback, quota, metadata, provider adapters, request conversion structs,
   management APIs, TUI, storage schema, config, or logging.
6. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- Codex Responses policy still preserves Codex input/tools and allows Codex
  options.
- OpenRouter Responses policy still allows parallel tool calls only.
- Default Responses policy remains empty.
- Anthropic conversion still includes generation options for non-Codex and not
  for Codex.
- Stream error exposure remains enabled only for Codex.
- Codex OAuth refresh capability remains true only for Codex OAuth instances
  with a refresh controller.
- Chat and stream OAuth refresh policies remain gated by Codex OAuth refresh
  capability, upstream-auth 401 status/class, and current pre-stream constraints.
- Chat and stream model-credential refresh policies remain gated by Codex OAuth
  refresh capability, refreshable credential ID, model-discovery-auth 401
  status/class, and current pre-stream constraints.
- Model discovery 401 refresh policy remains gated by Codex OAuth refresh
  capability.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- Provider-type policy selectors are grouped in one server-local boundary.
- Existing behavior is preserved.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

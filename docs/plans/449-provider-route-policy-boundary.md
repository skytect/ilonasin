# 449 Provider Route Policy Boundary

## Context

Plan 414 grouped provider-policy selectors into
`internal/server/provider_policy.go`. Plan 442 then found that three of those
selectors still embed provider-specific route behavior in the server core:

- Codex and OpenRouter Responses conversion policy;
- Codex Anthropic generation-option omission;
- Codex stream pre-response error exposure.

`docs/ilonasin-architecture.md` says provider adapters own provider-specific
behavior and the router core should not embed provider-specific quirks beyond
selecting an adapter and passing typed route options.

## Goal

Move remaining provider-specific route policy selection out of
`internal/server` into a provider-owned, dependency-neutral policy helper,
preserving exact behavior.

## Scope

1. Add a provider-owned route policy helper, likely in
   `internal/provider/route_policy.go`.
2. Keep the helper dependency-neutral:
   - the new route policy helper must not import `internal/server`,
     `internal/openai`, or `internal/anthropic`;
   - expose provider-owned policy structs with neutral field names.
3. Encode existing behavior exactly:
   - Codex preserves Responses input and tools and allows Codex options;
   - OpenRouter allows Responses parallel tool calls;
   - other provider types use the empty Responses policy;
   - Anthropic generation options are included except for Codex;
   - stream pre-response provider error-class exposure is enabled only for
     Codex.
4. Update `internal/server/provider_policy.go` to map the provider-owned
   neutral policy into `openai.ResponsesConversionPolicy`,
   `anthropic.ChatConversionPolicy`, and the existing stream error exposure
   shape without switching on provider type in server.
5. Keep OAuth refresh policy helpers in `internal/server/provider_policy.go`
   unchanged for this slice because they depend on server refresh-controller
   availability and metadata capability checks.
6. Keep routes, request parsing, conversion behavior, stream error behavior,
   OAuth refresh behavior, provider adapters, storage, management APIs, TUI,
   config, and logging unchanged.
7. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- Codex Responses policy still preserves Codex input/tools and allows Codex
  options.
- OpenRouter Responses policy still allows parallel tool calls only.
- Default Responses policy remains empty.
- Anthropic conversion still includes generation options for non-Codex and not
  for Codex.
- Stream error exposure remains enabled only for Codex.
- Server route policy helpers no longer switch on raw provider type for these
  three route policies.
- OAuth refresh helpers remain behaviorally unchanged. Check the existing truth
  table at least for:
  - Codex OAuth with refresh service enabled;
  - chat 401 `upstream_auth_failed`;
  - stream pre-response 401 `upstream_auth_failed`;
  - model-credential chat 401 `model_discovery_auth_failed` with a
    refreshable credential ID;
  - model-credential stream pre-response 401 `model_discovery_auth_failed` with
    a refreshable credential ID;
  - model-list 401;
  - non-Codex and API-key-only providers skipping refresh.

Then run:

```sh
rg -n 'responsesConversionPolicy|anthropicConversionPolicy|streamErrorExposurePolicyFor|case "codex"|case "openrouter"|instance.Type == "codex"' internal/server/provider_policy.go internal/provider
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/provider ./internal/server
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health and
snapshot over the Unix socket, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- Server route conversion and stream exposure helpers no longer own
  provider-type-specific route policy.
- Provider-owned route policy is explicit and dependency-neutral.
- Existing behavior is preserved.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

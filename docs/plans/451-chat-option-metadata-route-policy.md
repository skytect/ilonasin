# 451 Chat Option Metadata Route Policy

## Context

Plan 450 recorded two independent whole-codebase review findings that server
request metadata still selects Chat option metadata policy from raw provider
type:

```go
provider.ChatOptionMetadataPolicyForProviderType(instance.Type)
```

Plan 449 moved route conversion and stream exposure policy into
`provider.RoutePolicyForInstance`, but Chat option metadata remains a separate
provider-specific selector visible at the server boundary.

`docs/ilonasin-architecture.md` says provider adapters own provider-specific
behavior and the router core should not embed provider-specific quirks beyond
selecting an adapter and passing typed route options.

## Goal

Move Chat option metadata policy selection behind the provider-owned route
policy boundary, preserving exact metadata behavior.

## Scope

1. Update `internal/provider/route_policy.go` and
   `internal/provider/chat_option_metadata.go`.
2. Add Chat option metadata policy to `RoutePolicy`.
   - Codex keeps Codex metadata extraction and suppresses default top-level
     service tier.
   - DeepSeek keeps DeepSeek metadata extraction.
   - OpenRouter keeps OpenRouter metadata extraction.
   - Unknown/default provider types keep the empty metadata policy.
3. Remove or stop using the provider-type selector
   `ChatOptionMetadataPolicyForProviderType`.
4. Update `internal/server/request_metadata_chat.go` to use
   `provider.RoutePolicyForInstance(instance).ChatMetadata` or equivalent,
   without passing `instance.Type` into a provider-specific policy selector.
5. Preserve early request metadata behavior in
   `internal/server/request_metadata_early.go`: it has no provider instance and
   should keep using an empty `provider.ChatOptionMetadataPolicy{}`.
6. Keep request metadata fields, safe option extraction, provider option
   parsing, validation, routing, public APIs, provider adapters, storage,
   management APIs, TUI, config, and logging unchanged.
7. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- Codex metadata still extracts Codex service tier, maps `fast` to `priority`,
  suppresses effective tier for `default`, and extracts reasoning effort and
  summary.
- DeepSeek metadata still extracts `reasoning_effort` and `thinking.type`.
- OpenRouter metadata still extracts reasoning effort, max tokens, enabled, and
  exclude.
- Top-level service tier behavior is unchanged for Codex and non-Codex
  providers.
- Unknown/default provider metadata policy remains empty except common
  top-level service tier behavior allowed by the existing zero policy.
- Early request metadata still uses an empty policy.
- `internal/server/request_metadata_chat.go` no longer calls
  `ChatOptionMetadataPolicyForProviderType` or passes `instance.Type` to a
  Chat metadata policy selector.

Then run:

```sh
rg -n 'ChatOptionMetadataPolicyForProviderType|instance.Type.*ChatOption|ChatMetadata' internal/provider internal/server
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

- Chat option metadata policy is selected through the provider-owned route
  policy boundary.
- Server request metadata no longer calls a raw provider-type metadata selector.
- Existing metadata behavior is preserved.
- No permanent tests are added.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

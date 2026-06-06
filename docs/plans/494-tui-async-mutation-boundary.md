# 494 TUI Async Mutation Boundary

## Context

Plan 490 found that several TUI actions still call daemon management APIs
synchronously from Bubble Tea action/update paths. The TUI already refreshes
snapshots and subscription usage through async `tea.Cmd` messages, so mutation
calls should use the same boundary.

## Goal

Move blocking TUI management mutations into `tea.Cmd` flows while preserving the
existing management API, storage, DTO, keybinding, and rendering behavior.

## Scope

1. Convert local token create and disable actions in
   `internal/tui/api_local_token_actions.go` to return command-backed result
   messages.
2. Convert upstream API key add in
   `internal/tui/provider_api_key_actions.go` to a command-backed result
   message after input validation.
3. Convert upstream credential disable in
   `internal/tui/provider_upstream_actions.go` to a command-backed result
   message.
4. Convert OAuth credential refresh in `internal/tui/oauth_actions.go` to a
   command-backed result message. Preserve device-login behavior, which already
   uses commands.
5. Convert telemetry prune in `internal/tui/usage_log_actions.go` to a
   command-backed result message.
6. Handle all new mutation result messages in `internal/tui/update.go`,
   including existing logging, error text, reveal-token state, prune result
   state, and snapshot refresh after successful or relevant failed mutations.
7. Add a small TUI mutation in-flight guard so repeated keypresses do not start
   duplicate mutable management calls while an async command is still running.
8. Do not change management routes, DTOs, storage schema, keybindings,
   rendered layout, provider behavior, routing, config, or IO logging policy.
9. Do not add permanent tests.

## Verification

Run:

```sh
gofmt -w internal/tui/api_local_token_actions.go internal/tui/provider_api_key_actions.go internal/tui/provider_upstream_actions.go internal/tui/oauth_actions.go internal/tui/usage_log_actions.go internal/tui/mutation_commands.go internal/tui/model.go internal/tui/update.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
rg -n "(CreateLocalToken|DisableLocalToken|AddUpstreamAPIKey|DisableUpstreamCredential|RefreshOAuthCredential|PruneTelemetry)" internal/tui
```

Run direct CLI smokes with a temporary binary:

1. Start `ilonasin serve` with isolated home and a valid config.
2. Verify management health and snapshot over the Unix socket.
3. Run bounded `ilonasin manage` through a PTY at narrow and wide widths.
4. Clean up all temporary files and processes.

## Acceptance

- TUI mutation actions no longer perform management socket or SQLite-backed
  work directly in the update path.
- A TUI mutation in flight suppresses duplicate mutation launches until the
  result message is handled.
- Existing local-token reveal, API-key add, OAuth refresh, telemetry prune,
  logging, error messages, and snapshot refresh behavior are preserved.
- The TUI still uses daemon-owned management APIs for mutable operations.
- Direct compile, vet, serve, and manage smokes pass.
- Source grep shows scoped management mutation calls are only inside command
  closures and not directly in action or update handlers.

## Review Notes

Plan review feedback called out three concrete risks:

- Include upstream credential disable because it was another blocking TUI
  mutation path. This slice includes `internal/tui/provider_upstream_actions.go`.
- Do not keep API key secrets in result messages. The command captures the key
  for the call, but `upstreamAPIKeyAddedMsg` carries only provider ID, created
  credential metadata, and error.
- Do not keep full local token secrets in result messages. The create command
  extracts `created.Metadata` and drops `created.Token` before sending
  `localTokenCreatedMsg`.
- Guard unavailable local-token clients before starting async local-token
  mutation commands, matching other mutation client availability checks.
- Preserve one-time local token reveal and prune result handling in update
  result messages before triggering snapshot refresh.

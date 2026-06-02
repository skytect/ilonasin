# 275 Keepalive Output Cap Guard

## Goal

Restore the subscription keepalive safety invariant: no outbound keepalive
request should run until the output cap is actually enforced on the provider
wire request.

The keepalive feature exists to run a tiny request with one max output token on
each subscription account. Current code has a `max_output_tokens` config value,
but `keepaliveRequest` does not put any output cap into the request. The daemon
therefore can run uncapped keepalive calls while management reports
`enabled_uncapped`. That conflicts with the opt-in feature intent and earlier
plan safety notes.

## Current Evidence

- `internal/config/config.go` defines
  `SubscriptionKeepaliveConfig.MaxOutputTokens` and defaults it to `1`.
- `internal/app/keepalive.go` builds `keepaliveRequest(model string)` without
  using `MaxOutputTokens`.
- `internal/provider/chat_validation.go` rejects `max_tokens` and
  `max_completion_tokens` for Codex Chat Completions.
- `internal/provider/chat_request.go` only maps `MaxCompletionTokens` for
  DeepSeek and OpenRouter, not Codex.
- `docs/plans/104-codex-subscription-usage.md` says not to send keepalive
  requests until there is verified hard source or smoke evidence for a
  Codex-compatible output cap field.
- `internal/management/subscription_usage_keepalive.go` currently reports
  `enabled_uncapped` and `output_cap_verified = false`, which is accurate but
  insufficient because the app still starts the runner.

## Scope

1. Keep `subscription_keepalive.enabled` and schedule/status reporting intact.
2. Add a small config/helper boundary that reports whether the configured
   keepalive output cap is currently wire-verified. For this slice it should
   return false for all configs.
3. Make `startSubscriptionKeepalive` refuse to start the background runner when
   keepalive is enabled but the output cap is not verified. Check this after
   `!enabled` and before resolver/adapter dependency checks or goroutine
   creation.
4. Log a metadata-only warning event when the runner is skipped for
   `unavailable_output_cap_unverified`, without logging model names, prompts,
   request bodies, response bodies, provider payloads, provider instance IDs,
   credential IDs, account data, or tokens.
5. Update management keepalive status to report:
   - disabled config: `status = "disabled"`;
   - enabled but unverified cap: `status = "unavailable_output_cap_unverified"`;
   - `output_cap_verified = false`.
6. Use the same helper for app start and management status so status cannot
   diverge from execution policy.
7. Add a defensive guard in the runner path before outbound work, so future
   internal calls cannot accidentally issue uncapped keepalive requests.
8. Preserve normalized schedule reporting from plan 274.

## Boundaries

- No provider wire-field implementation for output caps in this slice.
- No keepalive prompt, model, schedule, OAuth resolution, subscription usage
  refresh, quota math, credential pooling, storage, schema, public API route,
  Anthropic, OpenAI, or provider adapter changes.
- No direct SQLite or `config.toml` mutation from the TUI.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run temporary focused checks, then remove them before commit:

- assert enabled keepalive with default cap does not start a runner while
  output cap verification is false;
- assert disabled keepalive remains a no-op;
- assert enabled unverified keepalive logs one metadata-only warning before
  returning a no-op stop function;
- assert management status for enabled config is
  `unavailable_output_cap_unverified`;
- assert schedule normalization from plan 274 is preserved in management
  status;
- assert no keepalive request can reach a fake adapter/resolver when the cap is
  unverified.
- assert the defensive runner guard also prevents fake resolver/adapter calls
  if invoked directly.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive enabled, and a shorthand schedule value.
3. Verify the management health endpoint over the management socket.
4. Verify management snapshot reports keepalive enabled,
   `status = "unavailable_output_cap_unverified"`,
   `output_cap_verified = false`, and canonical schedule values.
5. Run `manage` under a short timeout and verify the usage section chrome still
   renders.
6. Remove all temporary artifacts.

## Acceptance

- Enabled keepalive no longer sends uncapped outbound requests.
- Management status clearly explains that the feature is unavailable because
  the output cap is unverified.
- Schedule/status visibility remains intact.
- Compile, vet, focused checks, serve smoke, manage smoke, senior plan review,
  and senior implementation review pass.

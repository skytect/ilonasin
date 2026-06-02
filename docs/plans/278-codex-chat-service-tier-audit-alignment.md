# 278 Codex Chat Service Tier Audit Alignment

## Goal

Align stale compatibility-audit language for direct Codex Chat Completions
service-tier support with the current implementation.

Current code inspection shows plan 246 is implemented:

- `internal/provider/chat_validation.go` no longer rejects top-level
  `service_tier` for Codex and allows `default`, `priority`, and `flex`;
- `internal/provider/codex_responses_request.go` calls `codexRequestOptions`
  when `req.ServiceTier` is present;
- `codexRequestOptions` serializes supported top-level Codex service tiers into
  the upstream Codex Responses request, omits `default`, and lets
  `provider_options.codex.service_tier` win when both are present.

`docs/codex-compatibility-audit.md` still has a current failure row for direct
Codex Chat reasoning/service tier. Reasoning remains a provider-options feature,
but top-level Codex Chat `service_tier` is no longer the code gap described by
that row.

## Scope

1. Add a temporary focused fake-upstream smoke under `internal/provider`, run it,
   and remove it before commit. It must prove:
   - Codex top-level Chat `service_tier: "priority"` is accepted and serialized
     upstream as `service_tier: "priority"`;
   - Codex top-level Chat `service_tier: "flex"` is accepted and serialized
     upstream as `service_tier: "flex"`;
   - Codex top-level `service_tier: "default"` is accepted and omitted upstream;
   - `provider_options.codex.service_tier` wins over top-level `service_tier`
     with a distinguishing case, such as top-level `flex` plus provider-options
     `fast` mapping upstream to `priority`;
   - top-level Codex `service_tier: "auto"` and `service_tier: "scale"` fail
     locally even though the shared OpenAI decoder accepts those values;
   - DeepSeek still rejects top-level `service_tier`.
2. Update `docs/codex-compatibility-audit.md` to:
   - split the stale combined direct Codex Chat reasoning/service-tier row into
     accurate current status;
   - mark direct Codex Chat top-level `service_tier` as fixed in code, with fake
     smoke evidence;
   - preserve remaining switch-gate blockers, including OpenRouter Codex CLI
     compatibility and broader Codex tool-family parity.
3. Do not change production code unless the temporary smoke contradicts current
   code inspection.

## Boundaries

- Documentation-only final diff unless the focused smoke exposes a real code
  regression.
- No management API, storage, TUI, subscription keepalive, model discovery,
  logging policy, route shape, provider option redesign, or live credential
  behavior changes.
- Do not claim all direct Codex Chat reasoning behavior is fixed. Reasoning is
  still documented through `provider_options.codex.reasoning`.
- Do not claim broad Codex switch-gate readiness.
- No permanent tests.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./...
go vet ./...
```

Run a temporary focused fake-upstream smoke, then remove it before commit:

- use a fake Codex models endpoint that advertises `priority` and `flex`
  service tiers;
- exercise direct Chat Completions request validation and Codex Responses body
  marshaling;
- assert `priority`, `flex`, `default`, provider-option precedence, Codex
  `auto` and `scale` rejection, and DeepSeek top-level `service_tier` rejection.

Run disposable daemon smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, keepalive disabled, and at least two provider instances.
3. Verify management health over the management socket.
4. Run `manage` under a short timeout and verify API, providers, usage, and
   logs chrome renders.
5. Remove all temporary artifacts.

## Acceptance

- Temporary fake-upstream smoke proves current Codex Chat top-level
  `service_tier` behavior.
- `docs/codex-compatibility-audit.md` no longer reports direct Codex Chat
  service-tier support as a current failure.
- Remaining compatibility blockers remain intact.
- No permanent test files remain.
- Compile, vet, serve smoke, manage smoke, senior plan review, and senior
  implementation review pass.

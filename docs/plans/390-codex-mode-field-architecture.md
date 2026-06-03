# 390 Codex Mode Field Architecture

## Context

`docs/ilonasin-architecture.md` still says Codex-style fast mode and reasoning
effort field mapping is deferred, and repeats that as an open question. Current
code and audits are more concrete:

- Codex Chat uses `provider_options.codex.reasoning.effort` and
  `provider_options.codex.reasoning.summary` for reasoning controls.
- Codex Chat uses top-level `service_tier` for `default`, `priority`, and
  `flex`, plus `provider_options.codex.service_tier` for provider-specific
  values including local `fast`, which maps upstream to `priority`.
- Codex Chat uses `provider_options.codex.verbosity` for text verbosity.
- Codex Responses accepts top-level `reasoning`, `text.verbosity`, and
  `service_tier`, then translates those into the shared local Chat request
  representation for Codex providers.
- Codex request shaping sends upstream Responses `reasoning`, `text`,
  `service_tier`, and `include: ["reasoning.encrypted_content"]` when
  reasoning is present.

The architecture should describe that settled local field contract without
claiming universal provider parity or broad Codex switch readiness.

## Goal

Replace the stale deferred Codex mode mapping language in
`docs/ilonasin-architecture.md` with the current field contract, and remove the
matching open question.

## Scope

1. Update the Modes and Reasoning Effort section:
   - keep the rule that mode/reasoning behavior must use request fields, not
     model suffixes;
   - document Codex Chat provider-options fields for reasoning, summary,
     verbosity, and provider-specific service tier;
   - document Codex Chat top-level `service_tier` handling for standard tiers;
   - document Codex Responses top-level `reasoning`, `text.verbosity`, and
     `service_tier` translation into the same Codex provider-options contract;
   - state that unsupported fields and invalid mode values should be rejected
     locally rather than silently forwarded, while model-unsupported but locally
     valid Codex reasoning efforts may be normalized to a supported model
     effort.
2. Remove the stale open question about exact Codex fast/reasoning fields.
3. Do not change production code, request validation, provider adapters,
   management API, storage, TUI, logging, config, or smoke/audit claims.

## Verification

Run:

```sh
rg -n "reasoning|service_tier|verbosity|provider_options.codex|text.verbosity|reasoning.encrypted_content|fast mode" docs/ilonasin-architecture.md docs/codex-compatibility-audit.md internal/openai internal/provider internal/server
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke.

## Acceptance

- Architecture no longer treats Codex fast/reasoning request fields as
  unresolved.
- Architecture matches current local Chat, Responses, and Codex provider
  request-shaping behavior.
- Remaining Codex compatibility blockers remain untouched.
- No runtime code changes.

# 481 Supporting Provider Docs Alignment

## Context

Plan 478 recorded supporting-doc drift:

- `docs/deepseek-openrouter-comparison.md` still recommends colon-prefixed model
  namespaces even though the active architecture and router use slash model
  addresses;
- the same comparison doc suggests storing `provider_usage` JSON, which can be
  read as raw provider payload storage;
- `docs/codex-auth.md` calls credits and request IDs first-class rate-limit
  telemetry, while the active architecture forbids querying provider billing,
  balances, credits, plan limits, and full provider request ID persistence by
  default.

## Goal

Align supporting provider docs with the active architecture's model addressing,
metadata-only storage, quota-policy, and request-ID privacy boundaries.

## Scope

1. Update `docs/deepseek-openrouter-comparison.md`.
2. Update `docs/codex-auth.md`.
3. Do not change `docs/ilonasin-architecture.md`.
4. Do not edit historical plan files.
5. Do not change runtime behavior.
6. Do not add permanent tests.

## Implementation

1. Replace colon namespace guidance with current slash model addressing:
   `<provider_instance_id>/<provider_model_id>`.
2. Replace `provider_usage` JSON storage guidance with normalized metadata-only
   fields and optional IO-log-only raw payload debugging language.
3. Replace credits/full request-ID telemetry wording with redacted or normalized
   quota metadata that is allowed by the active architecture.
4. Keep the docs as research/supporting material, not new architecture.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run a direct CLI smoke by building a temporary `ilonasin` binary, starting
`ilonasin serve` with an isolated temporary home and config, checking the
management health and snapshot endpoints over the Unix socket, running bounded
`ilonasin manage` at several terminal widths, then cleaning up all temporary
files and processes.

## Acceptance

- Supporting docs no longer recommend colon model namespaces for ilonasin.
- Supporting docs no longer recommend storing raw provider usage JSON as normal
  metadata.
- Supporting docs no longer recommend credits or full request IDs as default
  persisted telemetry.
- No runtime files are changed.

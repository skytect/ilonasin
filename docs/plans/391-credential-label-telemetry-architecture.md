# 391 Credential Label Telemetry Architecture

## Context

`docs/ilonasin-architecture.md` still asks whether credential records should
use labels visible in telemetry. Current code already has a settled boundary:

- request, health, fallback, and quota metadata rows store credential IDs, not
  duplicated credential labels;
- read-side summary queries join credential labels from `provider_credentials`;
- management DTOs expose `credential_label` for recent requests, health,
  fallbacks, and quota summaries;
- management snapshot sanitization redacts unsafe label values before the TUI
  displays them.

The architecture should describe labels as safe display metadata on summary
surfaces, while keeping durable event rows ID-based.

## Goal

Update `docs/ilonasin-architecture.md` to settle credential-label telemetry
semantics and remove the stale open question.

## Scope

1. Update the Observability and Logging section:
   - raw request, health, fallback, and quota metadata rows should reference
     credentials by local credential ID;
   - management summaries and TUI views may display safe credential labels by
     joining current credential metadata at read time;
   - unsafe labels must be sanitized before management snapshots or TUI output;
   - labels are operator/display metadata, not credential selectors, account
     IDs, bearer tokens, or durable copies of secrets.
2. Remove the stale open question about whether credential records should use
   labels visible in telemetry.
3. Do not change code, storage schema, management DTOs, TUI rendering, provider
   behavior, logging, or pruning.

## Verification

Run:

```sh
rg -n "CredentialLabel|credential_label|provider_credentials|request_metadata|health_events|fallback_events|quota_events|safeSnapshotString" internal/metadata internal/storage/sqlite internal/management internal/tui docs/ilonasin-architecture.md
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run the standard temporary `serve` plus `manage` smoke.

## Acceptance

- Architecture no longer treats credential-label telemetry as unresolved.
- Architecture matches current ID-based durable rows plus read-side safe label
  display behavior.
- No runtime code changes.

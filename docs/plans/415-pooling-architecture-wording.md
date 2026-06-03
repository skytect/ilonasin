# 415 Pooling Architecture Wording

## Context

Whole-codebase review found stale wording in `docs/ilonasin-architecture.md`:

the credential pooling section still says “Fallback-policy rows are
operator/display metadata,” but live code has removed fallback-policy control
surfaces. Current live state exposes credential pool groups as operator display
metadata, while historical fallback-policy artifacts remain only in compatibility
migrations and older plan history.

The stale sentence conflicts with the current architecture and can mislead future
work into preserving a removed fallback-policy model.

## Goal

Update the architecture wording so the credential pooling section describes the
current credential pool-group model.

## Scope

1. Update `docs/ilonasin-architecture.md` in the credential pooling section.
2. Replace the stale fallback-policy-row sentence with wording that says:
   - pooling remains auditable through metadata-only request, fallback, health,
     and quota rows;
   - credential pool-group listing/metadata is operator display metadata;
   - serving eligibility remains the default same-provider, same-model eligible
     credential pool.
3. Keep historical plan files and SQLite migration text unchanged.
4. Do not change code, storage schema, management DTOs, TUI, routing, provider
   behavior, config, logging, or tests.

## Verification

Run:

```sh
rg -n "Fallback-policy rows|fallback-policy rows|credential pool-group|credential pool group" docs/ilonasin-architecture.md
rg -n "credential_fallback_policies|fallback_group|allowed_by_policy" internal/storage/sqlite/migrations.go
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide terminal
widths, and cleaning up all temporary files and processes.

## Acceptance

- The architecture doc no longer describes fallback-policy rows as live
  operator/display metadata.
- The architecture doc describes credential pool groups as the live display
  metadata surface.
- Historical migrations remain intact for compatibility.
- No runtime behavior changes.
- Compile, vet, serve/manage smoke, and three implementation reviews pass.

# 435 Docs Codex Auth Storage Alignment

## Context

Plan 426 found a docs contradiction:

- `docs/ilonasin-architecture.md` says initial Ilonasin SQLite state is
  plaintext and relies on local file permissions, redaction, and clear user
  warnings.
- `docs/codex-auth.md` includes generic router advice recommending encrypted
  local credential storage.

The Codex auth document is useful as source research and generic design advice,
but supporting docs should not read as if they override Ilonasin's current
architecture decision.

## Goal

Clarify `docs/codex-auth.md` so its encrypted-storage recommendation is framed
as generic/future hardening guidance, while Ilonasin's current target remains
plaintext SQLite with permission and redaction controls.

## Scope

1. Update `docs/codex-auth.md`.
2. Keep the underlying Codex source findings unchanged.
3. Reword the credential-storage recommendation to distinguish:
   - generic router guidance or future hardening;
   - Ilonasin's current plaintext SQLite architecture.
4. Do not change runtime code, schemas, config, management API, TUI, logging,
   provider behavior, or request/response behavior.
5. Do not add permanent tests.

## Verification

Run:

```sh
rg -n "encrypted local storage|SQLite is plaintext|database-level encryption|plaintext SQLite|CredentialProvider" docs/codex-auth.md docs/ilonasin-architecture.md
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health and
snapshot over the Unix socket, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- `docs/codex-auth.md` no longer contradicts the current plaintext SQLite
  architecture.
- The document still preserves generic future-hardening advice.
- No runtime behavior changes.

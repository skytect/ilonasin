# 319 TUI Dead Display Helper Cleanup

## Context

Recent TUI rendering slices replaced clipped body-field display paths with
wrapped helpers. That left a few package-private TUI helpers without call sites:

- `credentialDisplay`;
- `healthModelDisplay`;
- `safeWrappedDisplay`.

Keeping stale display helpers makes the TUI sanitizer boundary harder to audit
and works against the architecture goal of no dead code.

This is a TUI-only cleanup slice. It must not change management DTOs, storage,
provider behavior, server routes, config mutation, logging capture policy, or
visible TUI behavior.

## Plan

1. Remove the unused helpers from `internal/tui/display.go` and
   `internal/tui/display_sanitize.go`.
2. Preserve the still-used helpers:
   - `wrappedCredentialDisplay`;
   - `safeWrappedAccountDisplay`;
   - `safeFullWrappedDisplay`;
   - `safeFullWrappedAccountDisplay`;
   - existing clipped chrome/display helpers.
3. Review the diff for accidental sanitizer policy drift, imports that should
   now be removed, and any unintended non-TUI changes.

## Verification

Run:

```sh
rg -n "\\b(credentialDisplay|healthModelDisplay|safeWrappedDisplay)\\b" internal/tui
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/tui
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with a temporary `ILONASIN_HOME`, checking management health over the Unix
socket, running `ilonasin manage` under bounded narrow and wide terminals, and
cleaning up the daemon and temporary directory.

## Acceptance

- The removed helpers have no remaining definitions or call sites.
- TUI sanitizer helpers that are still used remain unchanged.
- No visible TUI behavior or non-TUI behavior changes are introduced.

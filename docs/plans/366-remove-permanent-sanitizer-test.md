# 366 Remove Permanent Sanitizer Test

## Context

AGENTS.md says not to keep permanent test files and to use direct compile,
vet, CLI smoke checks, and temporary focused checks instead. The current tree
still has one permanent test file:

- `internal/credentials/upstream_sanitize_test.go`

It covers two sanitizer invariants:

- `sanitizeOAuthDisplay` must allow normal email-like labels with multiple
  dots;
- `looksLikeJWT` must reject only JWT-shaped strings, not ordinary dotted text.

Those invariants are still useful, but the permanent file conflicts with the
repo execution policy and keeps `find . -name '*_test.go' -type f -print`
non-empty.

## Goal

Remove the permanent sanitizer test file after proving its coverage with a
temporary focused check.

## Scope

1. Create a temporary focused test or equivalent temporary check that verifies
   the same sanitizer invariants.
2. Run the temporary check.
3. Delete the temporary check and the permanent
   `internal/credentials/upstream_sanitize_test.go` file.
4. Do not change sanitizer behavior, OAuth credential behavior, management,
   TUI, storage, provider adapters, routing, logging, or config.

## Out Of Scope

- Rewriting sanitizer helpers.
- Adding new permanent tests.
- Changing privacy policy.
- Removing or weakening standard compile, vet, or CLI smoke checks.

## Verification

Run the temporary focused sanitizer check first, then remove all temporary and
permanent test files.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/credentials
go test ./...
go vet ./...
```

The `find` command must print no files.

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health over the
Unix socket, running bounded `ilonasin manage` at narrow and wide widths, and
cleaning up all temporary files and processes.

## Acceptance

- No permanent `*_test.go` files remain.
- The sanitizer invariants from the deleted file were checked temporarily
  before deletion.
- Compile/package checks, vet, and direct serve/manage smokes pass.

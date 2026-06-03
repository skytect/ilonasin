# 432 Management Local Token Sanitizer

## Context

The whole-codebase review in plan 426 found that management local-token DTOs
still expose raw token labels through the list and create-token management
routes, while full management snapshots sanitize local-token labels and token
fragments.

The architecture says local client-token management surfaces should expose only
safe metadata and fragments. The full generated token is intentionally returned
only once by `CreateLocalTokenResponse.Token`; that one-time secret must remain
unchanged.

## Goal

Apply the existing management snapshot sanitizer policy to local-token metadata
returned by dedicated local-token management operations.

## Scope

1. Update `internal/management/tokens.go`.
2. Sanitize `LocalToken.Label`, `TokenPrefix`, and `TokenLast4` in
   `localTokenFromCredentials`.
3. Preserve `CreateLocalTokenResponse.Token` exactly as the one-time plaintext
   token return value.
4. Preserve DTO field names, route paths, token generation, token hashing,
   storage, auth verification, snapshot sanitization, TUI rendering, config,
   logging, provider behavior, and subscription usage behavior.
5. Do not add permanent tests.

## Verification

Use temporary focused checks, then remove them before commit:

- `ListLocalTokens` redacts unsafe token labels and sanitizes token fragments;
- `CreateLocalToken` redacts unsafe metadata labels and sanitizes metadata
  fragments while preserving the full `Token` response field;
- safe labels still pass through unchanged;
- empty labels and empty fragments remain empty or `none` according to the
  existing sanitizer behavior;
- no raw unsafe label appears in local-token management responses.

Then run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./internal/management
go test ./...
go vet ./...
```

Run direct CLI smokes by building a temporary binary, starting `ilonasin serve`
with an isolated temporary home and config, checking management health and
snapshot over the Unix socket, running bounded `ilonasin manage` at narrow and
wide terminal widths, and cleaning up all temporary files and processes.

## Acceptance

- Dedicated local-token management operations expose the same safe metadata
  boundary as snapshots.
- The full generated local client token is still returned only by
  `CreateLocalTokenResponse.Token`.
- No runtime behavior outside management DTO sanitization changes.

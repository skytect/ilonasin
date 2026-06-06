# 491 Provider Base URL Sanitizer

## Context

Plan 490 found that provider `base_url` validation accepts HTTPS URLs with
userinfo, query, or fragment components, while `auth_issuer` already rejects
those components. Provider configuration should not carry secret-bearing URL
parts or ambiguous request modifiers into provider construction, logs,
management snapshots, or TUI display.

## Goal

Reject unsafe provider `base_url` values at configuration load time by applying
the same component restrictions used for `auth_issuer`.

## Scope

1. Update `internal/provider/provider.go` so configured provider `base_url`
   values must:
   - use `https`;
   - include a host;
   - not include userinfo;
   - not include query;
   - not include fragment.
2. Preserve path support, because built-in defaults use path-bearing base URLs
   such as Codex.
3. Preserve default provider definitions and provider registry behavior for
   valid base URLs.
4. Do not change management DTOs, TUI rendering, provider request URL joining,
   auth issuer validation semantics, local auth, routing, storage, logging, or
   credential handling.
5. Update `docs/ilonasin-architecture.md` only if needed to make the provider
   config invariant explicit.
6. Do not add permanent tests.

## Verification

Run:

```sh
git diff --check
find . -name '*_test.go' -type f -print
go test ./...
go vet ./...
```

Run direct CLI smokes with a temporary binary:

1. Start `ilonasin serve` with an isolated valid config and confirm management
   health and snapshot over the Unix socket.
2. Run bounded `ilonasin manage` through a PTY at narrow and wide widths.
3. Run invalid-config checks for `base_url` values containing userinfo, query,
   and fragment, and confirm `ilonasin serve` exits nonzero before starting the
   daemon.
4. Clean up all temporary files and processes.

## Acceptance

- Unsafe configured provider `base_url` values are rejected with clear errors.
- Valid HTTPS provider base URLs, including path-bearing bases, still work.
- No secrets or query-shaped routing modifiers can enter provider runtime state
  through `base_url`.
- Direct `serve` and `manage` smokes pass.

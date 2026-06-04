# 480 Anthropic Auth Alias Architecture

## Context

Plan 478 recorded an architecture drift finding: the server accepts
`X-Api-Key` as an ilonasin local client token on Anthropic-compatible routes,
while `docs/ilonasin-architecture.md` only says local requests use
`Authorization: Bearer <ilonasin_token>`.

Earlier Anthropic compatibility slices intentionally added this alias for
Claude Code compatibility. The code limits it to Anthropic-compatible routes
and only uses it when no `Authorization` header is present.

## Goal

Make the Anthropic-compatible `X-Api-Key` local auth alias explicit in the
active architecture, with privacy and scope constraints.

## Scope

1. Update `docs/ilonasin-architecture.md` only.
2. Preserve `Authorization: Bearer <ilonasin_token>` as the primary local API
   auth mechanism.
3. Document `X-Api-Key: <ilonasin_token>` as an Anthropic-compatible route alias
   only for `POST /v1/messages` and `POST /v1/messages/count_tokens`.
4. State that the alias still verifies an ilonasin local client token, not an
   upstream provider API key.
5. State that the alias must not be logged or persisted as a full token.
6. Do not change runtime behavior.
7. Do not add permanent tests.

## Implementation

1. Add a short paragraph to the Local API Auth section.
2. Keep the local client token and upstream provider credential separation
   language intact.
3. Do not edit supporting historical plan files.

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

- The architecture names `Authorization: Bearer <ilonasin_token>` as primary
  local API auth.
- The architecture explicitly scopes `X-Api-Key` to Anthropic-compatible local
  routes.
- The architecture says `X-Api-Key` still carries an ilonasin local token, not a
  provider API key.
- No runtime files are changed.

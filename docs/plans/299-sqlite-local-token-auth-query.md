# 299 SQLite Local Token Auth Query

## Context

`docs/ilonasin-architecture.md` keeps local ilonasin client tokens separate from
upstream provider credentials. Local token metadata such as prefix and last4 is
for management display, while the OpenAI-compatible API verifier needs only the
token hash, token ID, label, and disabled state.

`internal/storage/sqlite/local_tokens.go` currently selects `token_prefix` and
`token_last4` in `FindLocalTokenByHash`, then scans them into unused variables.
That leaves dead scan surface in the auth path and makes the local-token storage
boundary less precise than the `credentials.LocalTokenAuthRecord` contract.

This slice is behavior-preserving. It must not change token generation, hashes,
SQLite schema, management token listing, redaction, auth errors, TUI behavior,
provider behavior, or config.

## Plan

1. Narrow `FindLocalTokenByHash` to select only fields required by
   `credentials.LocalTokenAuthRecord`:
   - `id`;
   - `label`;
   - `token_hash`;
   - disabled boolean.
2. Remove the unused prefix/last4 scan variables from the auth lookup.
3. Keep `ListLocalTokens` and `scanLocalTokenMetadata` unchanged so management
   token display still exposes sanitized prefix/last4 metadata through the
   existing route.
4. Keep the `client token not found` error behavior unchanged.
5. Review the diff before checks to confirm SQL/auth semantics are unchanged
   except for the removed unused columns.

## Verification

Run:

```sh
find . -name '*_test.go' -type f -print
git diff --check
go test ./internal/storage/sqlite
go test ./internal/credentials
go test ./...
go vet ./...
```

Run a temporary focused smoke, then remove it before commit:

- create a token through `credentials.Service` backed by a temporary SQLite
  store;
- verify the bearer token succeeds;
- disable the token and verify the same bearer token fails;
- verify listing still returns token prefix/last4 metadata.

Run direct CLI smokes:

1. Build a temporary `ilonasin` binary.
2. Start `serve` with temporary `ILONASIN_HOME`, temporary SQLite, IO capture
   disabled, and at least two provider instances.
3. Create a local token through the management API.
4. Verify authenticated `GET /v1/models` succeeds with that token.
5. Disable the token through the management API and verify authenticated
   `GET /v1/models` returns `401`.
6. Run `manage` under short PTY timeouts at narrow and wide widths and verify
   API, providers, usage, and logs render.
7. Remove all temporary artifacts and stop the daemon.

## Acceptance

- `FindLocalTokenByHash` no longer selects or scans unused prefix/last4 fields.
- Local token auth behavior remains unchanged.
- Local token list/display metadata remains unchanged.
- No schema, management DTO, TUI, server route, provider, or config behavior
  changes are introduced.
- Focused compile, full compile, vet, direct serve/manage smoke, and senior
  implementation reviews pass.
